package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	lg "auditchain/ledger"
	pb "auditchain/msg"
	wp2p "auditchain/wp2p"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/herumi/bls-eth-go-binary/bls"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go"
	"google.golang.org/grpc"
)

// ======================= Structs =======================
type CONSENSUSNODE struct {
	address          string
	selfId           string
	roundIdx         uint32
	members          []*pb.Wnode
	isLeader         atomic.Bool
	leader           bool
	channelID        string
	done             chan bool
	pendingCommittee *pb.CommitteeMsg // 새 커미티 대기용

	kafkaConsumer *kafka.Consumer
	blockMsg      *pb.TransactionMsg
	memberMsg     *pb.MemberMsg
	auditMsg      *pb.AuditMsg
	cancel        context.CancelFunc
	isRunning     atomic.Bool
}

// ======================= Constants =======================
const (
	validatorIP       = "117.16.244.33"
	validatorGrpcPort = "16220"
	validatorNatPort  = "11730"
	gossipMsgPort     = ":4242"
	grpcPort          = ":5252"
	gossipListenPort  = ":6262"
	prometheusPort    = ":12345"
)

var (
	validatorGrpcAddr = validatorIP + ":" + validatorGrpcPort
	validatorNatAddr  = validatorIP + ":" + validatorNatPort
	brokers           = ""
	kafkaGroupID      = "MYGROUP1"
	leaderChan        = ""
	sec               bls.SecretKey
	pub               *bls.PublicKey
	sigVec            []bls.Sign
	pubVec            []bls.PublicKey

	mu sync.RWMutex

	ctx, cancel = context.WithCancel(context.Background())
)
var consensusStatusMap = make(map[string]bool)

// ======================= Main =======================
func main() {
	defaultValidatorAddr := flag.String("snode", "117.16.244.33", "Validator node IP address")
	kafkaProcessorAddr := flag.String("broker", "117.16.244.33", "Kafka broker IP address")
	libp2pAddr := flag.String("libp2p", "/ip4/117.16.244.33/tcp/4001/p2p/QmTZPpSK44fZpmQn5orB2zjMF4qVGrRm5H7MwNhwJJtLNP", "Libp2p multiaddress")
	channel := flag.String("channel", "mychannel", "Channel name")
	consensusProtocol := flag.Bool("networktype", false, "true = MP2BTP, false = QUIC")

	flag.Parse()

	validatorGrpcAddr = *defaultValidatorAddr + ":16220"
	validatorNatAddr = *defaultValidatorAddr + ":11730"
	brokers = *kafkaProcessorAddr + ":9091," + *kafkaProcessorAddr + ":9092," + *kafkaProcessorAddr + ":9093"
	leaderChan = *channel

	var consensusNode CONSENSUSNODE
	consensusNode = CONSENSUSNODE{
		leader:    false,
		done:      make(chan bool, 1),
		blockMsg:  &pb.TransactionMsg{},
		auditMsg:  &pb.AuditMsg{},
		memberMsg: &pb.MemberMsg{},
	}

	go wp2p.Start(true, *libp2pAddr, 4010)
	time.Sleep(3 * time.Second)

	consensusNode.selfId = wp2p.Host
	fmt.Println("[HOST ID]", consensusNode.selfId)

	bls.Init(bls.BLS12_381)
	bls.SetETHmode(bls.EthModeDraft07)
	sec.SetByCSPRNG()
	pub = sec.GetPublicKey()

	http.Handle("/metrics", promhttp.Handler())
	// go monitoring()

	wp2p.JoinShard(leaderChan)

	consensusNode.start(*consensusProtocol)
}
func NewKafkaConsumer() *kafka.Consumer {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":    brokers,
		"group.id":             kafkaGroupID,
		"auto.offset.reset":    "earliest",
		"session.timeout.ms":   30000,
		"max.poll.interval.ms": 300000,
		"fetch.wait.max.ms":    10,
	})
	if err != nil {
		log.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	return c
}

func (w *CONSENSUSNODE) InitKafka() error {
	c := NewKafkaConsumer()
	err := c.SubscribeTopics([]string{"mychannel-abort"}, nil)
	if err != nil {
		return err
	}
	w.kafkaConsumer = c
	return nil
}

func (w *CONSENSUSNODE) setLeaderChan(isLeader bool, channel string) {
	if w.channelID != channel {
		w.channelID = channel
	}
	w.isLeader.Store(isLeader)
}

func (w *CONSENSUSNODE) setDone(b bool) {
	w.done <- b
}

// ======================= Node Methods =======================
func (w *CONSENSUSNODE) start(consensusProtocol bool) {
	go w.setLeaderChan(false, "")
	go w.setDone(false)
	w.leader = false

	// [GossipSub: Tx Proposal Listening]
	go w.InitKafka()
	go w.CommitteeListening()
	go w.BlockListening()
	go w.ConsensusListening()
	w.isRunning.Store(false)
	// go w.MP2BTPConsensusListening(pu) // 주석 처리: 필요 시 사용

	// [GRPC: Membership Message]
	w.Reporting()
	time.Sleep(3 * time.Second)
	ReportExternal(w.selfId)

	for {
		time.Sleep(10 * time.Second)
		w.Reporting()
	}
}

// ======================= Committee Listening =======================
func (w *CONSENSUSNODE) CommitteeListening() {
	w.GetAddress()
	listener, err := net.Listen("tcp", w.address+gossipListenPort)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", w.address+gossipListenPort, err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("⚠️ Accept error:", err)
			continue
		}
		go w.CommConnHandler(conn)
	}
}

func (w *CONSENSUSNODE) CommConnHandler(conn net.Conn) {
	defer conn.Close()

	recvBuf := make([]byte, 8192)
	n, err := conn.Read(recvBuf)
	if err != nil {
		if err == io.EOF {
			log.Printf("🔌 Connection closed by client: %v", conn.RemoteAddr())
		} else {
			log.Printf("❌ Failed to read data: %v", err)
		}
		return
	}

	recvMsg := &pb.CommitteeMsg{}
	if err := json.Unmarshal(recvBuf[:n], recvMsg); err != nil {
		log.Printf("❌ Failed to unmarshal CommitteeMsg: %v", err)
		return
	}

	// ======================= 합의 중 처리 =======================
	if w.isRunning.Load() {
		w.pendingCommittee = recvMsg
		log.Printf("⚠️ Still processing previous consensus round, storing new CommitteeMsg (round %d) as pending", recvMsg.RoundNum)
		return
	}

	w.applyCommittee(recvMsg)
}

// // Check if this peer is the leader; if so, it should receive messages from Kafka.
// // BlockListening listens for block insertion.
func (w *CONSENSUSNODE) BlockListening() {
	w.GetAddress()
	listener, err := net.Listen("tcp", w.address+grpcPort)
	if err != nil {
		log.Fatalf("❌ Failed to listen on %s: %v", w.address+grpcPort, err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("⚠️ Accept error:", err)
			continue
		}
		go w.BloConnHandler(conn)
	}
}

func (w *CONSENSUSNODE) BloConnHandler(conn net.Conn) {
	defer conn.Close()

	recvBuf := make([]byte, 8192)
	n, err := conn.Read(recvBuf)
	if err != nil {
		if err == io.EOF {
			log.Printf("🔌 Connection closed by client: %v", conn.RemoteAddr())
		} else {
			log.Printf("❌ Failed to read data: %v", err)
		}
		return
	}

	MsgRecv := &pb.GossipMsg{}
	if err := json.Unmarshal(recvBuf[:n], MsgRecv); err != nil {
		log.Printf("❌ Failed to unmarshal GossipMsg: %v", err)
		return
	}

	fmt.Println("📦 BLOCKINSERT: Committing block to ledger")
	go lg.UserAbortInfoInsert(MsgRecv.Rndblk)

	if w.pendingCommittee != nil {
		w.applyCommittee(w.pendingCommittee)
		w.pendingCommittee = nil
	}
}

func (w *CONSENSUSNODE) StartListener(topic string) {
	if w.isRunning.Load() {
		fmt.Println("이미 실행 중")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.isRunning.Store(true)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("💥 KafkaListener panic 발생: %v\n", r)
				debug.PrintStack()
			}
		}()
		defer w.isRunning.Store(false)

		w.KafkaListener(ctx, topic) // <- context 받아서 내부 loop 제어
	}()
}

func (w *CONSENSUSNODE) StopListener() {
	if w.cancel != nil {
		w.cancel()
		fmt.Println("Listener 중단 요청")
	}
}

// ======================= Pending Committee 적용 =======================
func (w *CONSENSUSNODE) applyCommittee(msg *pb.CommitteeMsg) {
	w.done = make(chan bool, 1)
	w.roundIdx = msg.RoundNum

	for _, shard := range msg.Shards {
		if len(shard.Member) != 1 {
			for _, mem := range shard.Member {
				if mem.NodeID == w.selfId {

					fmt.Println(msg.RoundNum)

					// if !verifyVRF(mem.VRFProof, mem.PublicKey, msg.RoundNum) {
					// 	fmt.Println("❌ VRF 검증 실패, 메시지 무시")
					// 	return
					// }

					w.members = shard.Member

					isLeader := (msg.GetType() == 1 && shard.LeaderID == w.selfId)
					w.leader = isLeader
					w.channelID = leaderChan

					if isLeader {
						fmt.Println("🎉Leader consensus node")
						go func() {
							defer func() {
								if r := recover(); r != nil {
									fmt.Printf("💥 KafkaListener 내부 goroutine에서 panic 발생: %v\n", r)
									debug.PrintStack()
								}
							}()
							defer w.setDone(false)
							defer w.isRunning.Store(false)
							w.StartListener(w.channelID + "-abort")
						}()
					} else {
						fmt.Println("🧩 Follower consensus node")
						w.StopListener()
					}
					return
				}
			}
		}
	}
}

// ======================= Kafka Listener =======================
func (w *CONSENSUSNODE) KafkaListener(ctx context.Context, rekey string) {
	re := regexp.MustCompile(`User(\d+)`)
	for {
		msg, err := w.kafkaConsumer.ReadMessage(-1)
		if err != nil {
			log.Printf("Kafka read error: %v", err)
			continue
		}
		_, err = w.kafkaConsumer.CommitMessage(msg)
		if err != nil {
			log.Printf("Failed to commit message: %v", err)
		}

		var Abortdata []*pb.TransactionMsg
		err = json.Unmarshal(msg.Value, &Abortdata)
		if err != nil {
			log.Printf("Failed to unmarshal Kafka message: %v", err)
			continue
		}

		userCount := make(map[string]int32)
		for _, data := range Abortdata {
			match := re.FindStringSubmatch(data.String())
			if len(match) > 1 {
				idx := idxToInt(match[1])
				userKey := "User" + strconv.Itoa(idx)
				userCount[userKey]++
			}
		}

		w.auditMsg = &pb.AuditMsg{}
		w.bftConsensus(userCount)

	}
}

func (w *CONSENSUSNODE) bftConsensus(userCount map[string]int32) {
	setConsensusStart(w.selfId)
	defer cancel()

	w.auditMsg = &pb.AuditMsg{
		BlkNum:              w.roundIdx,
		LeaderID:            w.selfId,
		PhaseNum:            pb.AuditMsg_PREPARE,
		MerkleRootHash:      "abcdefghijklmnopqrstuvwxyz",
		Aborttransactionnum: userCount,
		HonestAuditors: []*pb.HonestAuditor{
			{Id: w.selfId},
		},
	}

	hashBytes := lg.BlockHashCalculator(w.auditMsg)
	hash := sha512.Sum512(hashBytes)
	w.auditMsg.CurHash = hex.EncodeToString(hash[:63])

	signing := sec.SignByte([]byte(w.auditMsg.CurHash))
	w.auditMsg.Signature = signing.Serialize()

	// ==== PREPARE & COMMIT Broadcasting ====
	fmt.Println("🚀 Broadcasting PREPARE")
	chPrepare := make(chan *pb.AuditMsg, len(w.members))
	for _, each := range w.members {
		go w.Submit(chPrepare, each.Addr, pb.AuditMsg_AGGREGATED_PREPARE)
	}
	quorum := len(w.members) // TODO: 2f+1 로 계산
	w.waitForVotes(chPrepare, quorum, pb.AuditMsg_AGGREGATED_PREPARE)

	fmt.Println("🚀 Broadcasting COMMIT")
	chCommit := make(chan *pb.AuditMsg, len(w.members))
	for _, each := range w.members {
		go w.Submit(chCommit, each.Addr, pb.AuditMsg_AGGREGATED_COMMIT)
	}
	w.waitForVotes(chCommit, quorum, pb.AuditMsg_AGGREGATED_COMMIT)

	fmt.Println("📦 Disseminating block to peers")
	gossipMsg := &pb.GossipMsg{
		Type:   2,
		Rndblk: w.auditMsg,
	}
	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossipMsg)
	setConsensusDone(w.selfId)
}

/*I'm a leader, send a message to a w-node*/
func (w *CONSENSUSNODE) Submit(ch chan<- *pb.AuditMsg, ip string, targetPhase pb.AuditMsg_Phases) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	session, err := quic.DialAddr(context.Background(), ip+gossipMsgPort, tlsConf, nil)
	if err != nil {
		log.Printf("❌ Failed to connect to %s: %v", ip+gossipMsgPort, err)
		ch <- nil
		return
	}

	defer session.CloseWithError(0, "")

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Printf("❌ Failed to open stream: %v", err)
		ch <- nil
		return
	}

	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		log.Printf("❌ Marshal error: %v", err)
		ch <- nil
		return
	}
	_, err = stream.Write(sndBuf)
	if err != nil {
		log.Printf("❌ Write error: %v", err)
		ch <- nil
		return
	}

	rcvBuf := make([]byte, 8192)
	n, err := stream.Read(rcvBuf)
	if err != nil || n == 0 {
		ch <- nil
		return
	}

	msg := &pb.AuditMsg{}
	if err := json.Unmarshal(rcvBuf[:n], msg); err != nil {
		log.Println("❌ JSON unmarshal error:", err)
		ch <- nil
		return
	}

	if msg.PhaseNum == targetPhase {
		ch <- msg
	} else {
		ch <- nil
	}
}

/*Consensus three-phases: Announce(Completed State), Prepare, Commit] I'm not a leader*/
func (w *CONSENSUSNODE) ConsensusListening() {
	w.GetAddress()
	listener, err := quic.ListenAddr(w.address+gossipMsgPort, generateTLSConfig(), nil)
	if err != nil {
		panic(err)
	}
	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			panic(err)
		}
		stream, err := sess.AcceptStream(context.Background())
		if err != nil {
			panic(err)
		}
		go w.StreamHandler(stream)
	}
}

func (w *CONSENSUSNODE) StreamHandler(stream quic.Stream) {

	for {
		rcvBuf := make([]byte, 8192)
		n, err := stream.Read(rcvBuf)

		if err != nil {
			break
		}

		if n == 0 {
			continue
		}

		rcvBuf = rcvBuf[:n]
		cMsgRecv := &pb.AuditMsg{}
		if err = json.Unmarshal(rcvBuf, cMsgRecv); err != nil {
			fmt.Println(err)
			continue
		}

		switch cMsgRecv.PhaseNum {
		case pb.AuditMsg_PREPARE:
			w.preparePhase(cMsgRecv)
			w.sendResponse(stream)
			return
		case pb.AuditMsg_COMMIT:
			if w.verifying(cMsgRecv) {
				w.commitPhase(cMsgRecv)
				w.sendResponse(stream)
				return
			}
		default:
			continue
		}
	}
}

func (w *CONSENSUSNODE) preparePhase(cMsg *pb.AuditMsg) {
	w.auditMsg = cMsg
	hash := []byte(cMsg.CurHash)
	signing := sec.SignByte(hash)
	w.auditMsg.Signature = signing.Serialize()
	w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE
	w.updateAuditFields(cMsg)
}

func (w *CONSENSUSNODE) commitPhase(cMsg *pb.AuditMsg) {
	w.auditMsg = cMsg
	hash := []byte(cMsg.CurHash)
	signing := sec.SignByte(hash)
	w.auditMsg.Signature = signing.Serialize()
	w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
	w.updateAuditFields(cMsg)
}

func (w *CONSENSUSNODE) updateAuditFields(cMsg *pb.AuditMsg) {

	alreadyExists := false
	for _, id := range cMsg.HonestAuditors {
		if id.Id == w.selfId {
			alreadyExists = true
			break
		}
	}
	if !alreadyExists {
		w.auditMsg.HonestAuditors = append(cMsg.HonestAuditors, &pb.HonestAuditor{Id: w.selfId})
	} else {
		w.auditMsg.HonestAuditors = cMsg.HonestAuditors
	}
}

func (w *CONSENSUSNODE) sendResponse(stream quic.Stream) {
	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		panic(err)
	}
	if _, err = stream.Write(sndBuf); err != nil {
		panic(err)
	}
}

func (w *CONSENSUSNODE) Multisinging(cMsgs *pb.AuditMsg, sigVec []bls.Sign) (bool, int) {
	switch cMsgs.PhaseNum {
	case pb.AuditMsg_AGGREGATED_PREPARE:
		return w.aggregateAndPrepare(cMsgs, sigVec), len(w.auditMsg.HonestAuditors)

	case pb.AuditMsg_AGGREGATED_COMMIT:
		return w.aggregateAndCommit(cMsgs, sigVec), len(w.auditMsg.HonestAuditors)

	default:
		return false, 0
	}
}

func (w *CONSENSUSNODE) aggregateAndPrepare(cMsgs *pb.AuditMsg, sigVec []bls.Sign) bool {
	return w.aggregateAndSet(cMsgs, sigVec, pb.AuditMsg_COMMIT)
}

func (w *CONSENSUSNODE) aggregateAndCommit(cMsgs *pb.AuditMsg, sigVec []bls.Sign) bool {
	return w.aggregateAndSet(cMsgs, sigVec, pb.AuditMsg_AGGREGATED_COMMIT)
}

func (w *CONSENSUSNODE) aggregateAndSet(cMsgs *pb.AuditMsg, sigVec []bls.Sign, phase pb.AuditMsg_Phases) bool {
	var aggeSign bls.Sign
	aggeSign.Aggregate(sigVec)
	byteSig := aggeSign.Serialize()

	w.auditMsg.BlkNum = cMsgs.BlkNum
	w.auditMsg.PrevHash = cMsgs.PrevHash
	w.auditMsg.CurHash = cMsgs.CurHash
	w.auditMsg.Signature = byteSig
	w.auditMsg.PhaseNum = phase
	return true
}

func (w *CONSENSUSNODE) verifying(cMsg *pb.AuditMsg) bool {
	pubVec = pubVec[:0]

	for _, honestID := range cMsg.HonestAuditors {
		if pk := w.getPublicKeyByNodeID(honestID.Id); pk != nil {
			pubVec = append(pubVec, *pk)
		} else {
			fmt.Println("Missing public key for node: %s\n", honestID)
		}
	}

	var decSign bls.Sign
	if err := decSign.Deserialize(cMsg.Signature); err != nil {
		fmt.Println("Signature deserialization error:", err)
		return false
	}

	switch cMsg.PhaseNum {
	case pb.AuditMsg_COMMIT:
		if decSign.FastAggregateVerify(pubVec, []byte(cMsg.CurHash)) {
			fmt.Println("✅ AGGREGATED_COMMIT: Verification SUCCESS")
			return true
		}
		fmt.Println("❌ AGGREGATED_COMMIT: Verification ERROR")
	}
	return false
}

func (w *CONSENSUSNODE) getPublicKeyByNodeID(id string) *bls.PublicKey {
	for _, node := range w.memberMsg.Nodes {
		if node.NodeID == id {
			dec := &bls.PublicKey{}
			if err := dec.Deserialize(node.Publickey); err != nil {
				fmt.Printf("Failed to deserialize public key for node %s: %v\n", id, err)
				return nil
			}
			return dec
		}
	}
	return nil
}

// ======================= Helper =======================

func (w *CONSENSUSNODE) GetAddress() string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		// loopback 제외
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				w.address = ip.String()
				return w.address
			}
		}
	}
	return ""
}

type VRFNode struct {
	NodeID string
	PubKey *bls.PublicKey
	Seed   string
	Proof  []byte
	Value  *big.Int
}

func GenerateVRF(nodeID string, seed string) (node *VRFNode) {

	nodeinit := &VRFNode{
		NodeID: nodeID,
		PubKey: pub,
	}

	hash := sha256.Sum256([]byte(seed))
	sign := sec.SignByte(hash[:]) // 기존 sec 사용
	nodeinit.Proof = sign.Serialize()

	vrfHash := sha256.Sum256(sign.Serialize())
	nodeinit.Value = new(big.Int).SetBytes(vrfHash[:])

	return nodeinit
}

/*GRPC SERVICE Membershhip Exchanging Procedure*/
func (w *CONSENSUSNODE) Reporting() {
	conn, err := grpc.Dial(validatorGrpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect to validator gRPC server: %v", err)
	}
	defer conn.Close()

	c := pb.NewMembershipServiceClient(conn)
	nodeinfo := GenerateVRF(w.address, "hello")
	req := createSelfMembership(w.selfId, w.address, pub.Serialize(), "11730", nodeinfo)

	w.memberMsg, err = c.GetMembership(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to get membership: %v", err)
	}
}

func createSelfMembership(id string, addr string, pubKey []byte, port string, nodeinfo *VRFNode) *pb.MemberMsg {
	value, _ := json.Marshal(nodeinfo.Value.Bytes())

	node := &pb.Node{
		NodeID:    id,
		Addr:      addr,
		Port:      port,
		Publickey: pubKey,
		Seed:      "hello",
		Proof:     nodeinfo.Proof,
		Value:     value,
	}

	return &pb.MemberMsg{Nodes: []*pb.Node{node}}
}

func MerkleHash(s []uint64) string {
	return strconv.FormatUint(s[0], 16)
}

func ReportExternal(id string) {
	conn, err := net.Dial("tcp", validatorNatAddr)
	if err != nil {
		fmt.Println("[NAT] Failed to dial:", err)
		return
	}
	defer conn.Close()

	sndBuf, err := json.Marshal(id)
	if err != nil {
		fmt.Println("[NAT] Failed to marshal ID:", err)
		return
	}

	if _, err := conn.Write(sndBuf); err != nil {
		fmt.Println("[NAT] Failed to write data:", err)
	}
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(fmt.Errorf("failed to generate RSA key: %w", err))
	}

	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(fmt.Errorf("failed to create certificate: %w", err))
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(fmt.Errorf("failed to load TLS key pair: %w", err))
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}

func idxToInt(s string) int {
	var num int
	fmt.Sscanf(s, "%d", &num)
	return num
}

func setConsensusStart(id string) {
	consensusStatusMap[id] = false
}

func setConsensusDone(id string) {
	consensusStatusMap[id] = true
}

func (w *CONSENSUSNODE) waitForVotes(ch chan *pb.AuditMsg, quorum int, targetPhase pb.AuditMsg_Phases) bool {
	votes := 0
	var sigVec []bls.Sign

	honestAuditorsSet := make(map[string]*pb.HonestAuditor)

	for {
		msg := <-ch // blocking read
		if msg == nil {
			continue
		}

		sig := bls.Sign{}
		if err := sig.Deserialize(msg.Signature); err != nil {
			log.Println("❌ Signature deserialize error:", err)
			continue
		}

		sigVec = append(sigVec, sig)
		votes++
		fmt.Printf("🗳️ Votes: %d/%d\n", votes, quorum)

		for _, auditor := range msg.HonestAuditors {
			if auditor != nil {
				honestAuditorsSet[auditor.Id] = auditor
			}
		}

		// map → slice로 변환
		w.auditMsg.HonestAuditors = make([]*pb.HonestAuditor, 0, len(honestAuditorsSet))
		for _, auditor := range honestAuditorsSet {
			w.auditMsg.HonestAuditors = append(w.auditMsg.HonestAuditors, auditor)
		}

		if votes >= quorum {
			fmt.Printf("✅ Phase %v Aggregation Complete\n", targetPhase)
			w.Multisinging(msg, sigVec)
			return true
		}
	}
}

// ======================= Monitoring =======================
// func monitoring() {
// 	counter := prometheus.NewCounter(prometheus.CounterOpts{Namespace: "WDN", Name: "counter_total"})
// 	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "WDN", Name: "gauge_value"})
// 	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
// 		Namespace: "WDN", Name: "histogram_value", Buckets: prometheus.LinearBuckets(0, 5, 10),
// 	})

// 	prometheus.MustRegister(counter, gauge, histogram)

// 	go func() {
// 		for {
// 			counter.Add(rand.Float64() * 5)
// 			gauge.Add(rand.Float64()*15 - 5)
// 			histogram.Observe(rand.Float64() * 10)
// 			time.Sleep(2 * time.Second)
// 		}
// 	}()
// 	fmt.Println(http.ListenAndServe(prometheusPort, nil))
// }
