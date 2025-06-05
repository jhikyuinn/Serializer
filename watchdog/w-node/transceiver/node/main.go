package main

import (
	lg "auditchain/ledger"
	pb "auditchain/msg"
	wp2p "auditchain/wp2p"
	"context"
	"crypto/rand"
	"crypto/rsa"
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
	rd "math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime/metrics"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/herumi/bls-eth-go-binary/bls"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	quic "github.com/quic-go/quic-go"
	"google.golang.org/grpc"
)

type Fabric struct {
	Num          uint64
	PreviousHash []byte
	DataHash     []byte
	Consensus    bool
}

type CONSENSUSNODE struct {
	wlog *log.Logger

	address   string
	selfId    string
	roundIdx  uint32
	members   []*pb.Wnode
	isLeader  atomic.Bool
	leader    bool
	channelID string
	done      chan bool

	blockMsg  *pb.TransactionMsg
	memberMsg *pb.MemberMsg
	auditMsg  *pb.AuditMsg
}

const (
	validatorIP          = "117.16.244.33"
	validatorGrpcPort    = "16220"
	validatorNatPort     = "11730"
	gossipMsgPort        = ":4242"
	consensusPort        = ":5252"
	gossipListenPort     = ":6262"
	prometheusListenPort = ":12345"
)

var (
	validatorGrpcAddr = validatorIP + ":" + validatorGrpcPort
	validatorNatAddr  = validatorIP + ":" + validatorNatPort

	brokers    = ""
	leaderBro  = false
	leaderChan = ""

	sec    bls.SecretKey
	pub    *bls.PublicKey
	sigVec []bls.Sign
	pubVec []bls.PublicKey
)

var MP2BTPsession *PuCtrl.PuCtrl

// func getKafkaGroupID() string {
// 	hostname, _ := os.Hostname()
// 	groupID := "myGroup-" + hostname
// 	if groupID == "" {
// 		// 환경변수가 없으면 임시 로컬용 UUID 생성
// 		groupID = "local-" + uuid.New().String()
// 		fmt.Println("KAFKA_GROUP_ID not set, using local generated ID:", groupID)
// 	}
// 	return groupID
// }

func NewKafkaConsumer() *kafka.Consumer {
	// kafkaGroupID := getKafkaGroupID()
	kafkaGroupID := "MYGROUP"
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          kafkaGroupID,
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		log.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	return c
}

var (
	counter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "WDN_WNODE",
		Name:      "wdn_counter_total",
		Help:      "Total count of WDN events",
	})

	gauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "WDN_WNODE",
		Name:      "wdn_gauge_value",
		Help:      "Current gauge value of WDN",
	})

	histogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "WDN_WNODE",
		Name:      "wdn_histogram_value",
		Help:      "Histogram of WDN measurements",
		Buckets:   prometheus.LinearBuckets(0, 5, 10), // 0~50까지 10버킷
	})
)

func monitoring() {
	const nGo = "/sched/goroutines:goroutines"
	const nMem = "/memory/classes/heap/objects:bytes"

	prometheus.MustRegister(counter)
	prometheus.MustRegister(gauge)
	prometheus.MustRegister(histogram)

	getMetric := make([]metrics.Sample, 2)
	getMetric[0].Name = nGo
	getMetric[1].Name = nMem

	go func() {
		for {
			counter.Add(rd.Float64() * 5)
			gauge.Add(rd.Float64()*15 - 5)
			histogram.Observe(rd.Float64() * 10)

			metrics.Read(getMetric)
			time.Sleep(2 * time.Second)
		}
	}()
	fmt.Println(http.ListenAndServe(prometheusListenPort, nil))
}

func main() {
	defaultValidatorAddr := flag.String("snode", "117.16.244.33", "Validator node IP address")
	kafkaProcessorAddr := flag.String("broker", "117.16.244.33", "Kafka broker IP address")
	libp2pAddr := flag.String("libp2p", "/ip4/117.16.244.33/tcp/4001/p2p/QmeD4iQQJAjoAvaTwT3UbFqSZ5zMWJne7dk7519H1A4ML5", "Libp2p multiaddress")
	ami := flag.Bool("ami", false, "AMI mode (leader)")
	channel := flag.String("channel", "mychannel", "Channel name")

	flag.Parse()

	validatorGrpcAddr = *defaultValidatorAddr + ":16220"
	validatorNatAddr = *defaultValidatorAddr + ":11730"

	brokers = *kafkaProcessorAddr + ":9091," + *kafkaProcessorAddr + ":9092," + *kafkaProcessorAddr + ":9093"
	leaderBro = *ami
	leaderChan = *channel

	var consensusNode CONSENSUSNODE

	logFile := "./wnode.log"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		return
	}
	defer f.Close()

	consensusNode = CONSENSUSNODE{
		wlog:      log.New(f, "WLog ", log.LstdFlags),
		leader:    false,
		done:      make(chan bool, 1),
		blockMsg:  &pb.TransactionMsg{},
		auditMsg:  &pb.AuditMsg{},
		memberMsg: &pb.MemberMsg{},
	}

	consensusNode.wlog.Println("Start")

	go wp2p.Start(true, *libp2pAddr, 4010)
	time.Sleep(3 * time.Second)

	consensusNode.selfId = wp2p.Host
	fmt.Println("[HOST ID]", consensusNode.selfId)

	bls.Init(bls.BLS12_381)
	bls.SetETHmode(bls.EthModeDraft07)
	sec.SetByCSPRNG()
	pub = sec.GetPublicKey()

	http.Handle("/metrics", promhttp.Handler())
	go monitoring()

	wp2p.JoinShard(leaderChan)

	consensusNode.start()
}

func (w *CONSENSUSNODE) GetAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		w.wlog.Printf("Failed to get network interfaces: %v", err)
		return ""
	}

	for _, iface := range ifaces {
		// 도커 컨테이너 환경일 경우
		if iface.Name == "eth0" {
			// 로컬 환경일 경우
			// if iface.Name == "eno1" {
			addrs, err := iface.Addrs()
			if err != nil {
				w.wlog.Printf("Failed to get addresses for interface %s: %v", iface.Name, err)
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
				// IPv4 주소만 반환
				if ip != nil && ip.To4() != nil {
					w.address = ip.String()
					return w.address
				}
			}
		}
	}

	w.wlog.Println("No suitable network interface found")
	return ""
}

func (w *CONSENSUSNODE) start() {
	go w.setLeaderChan(false, "")
	go w.setDone(false)
	w.leader = false

	// [GossipSub: Tx Proposal Listening]
	// 1. All w-nodes receive every round of Gossip round messages from s-nodes (committee)
	// 2. If leader, run block listening
	// 3. Two message types: type-1 round msg, type-2 block msg (consensus)

	go w.CommitteeListening()
	go w.BlockListening()
	go w.ConsensusListening()

	// [GRPC: Membership Message]
	// 1. Report network info
	// 2. Get WDN Members info
	w.Reporting()

	// To solve AWS NAT Problem (외부 IP 주소를 알려주기 위해)
	time.Sleep(3 * time.Second)
	ReportExternal(w.selfId)

	for {
		time.Sleep(10 * time.Second)
		w.Reporting()
	}
}

func idxToInt(s string) int {
	var num int
	fmt.Sscanf(s, "%d", &num)
	return num
}

// kafkaListener pulls reliable blocks from the Weave Kafka partition when
// the peer is a leader.

func (w *CONSENSUSNODE) KafkaListener(rekey string) {
	c := NewKafkaConsumer()
	defer c.Close()
	err := c.SubscribeTopics([]string{rekey}, nil)
	if err != nil {
		log.Printf("Failed to subscribe to topic %s: %v", rekey, err)
		return
	}

	re := regexp.MustCompile(`User(\d+)`)

	for {
		select {
		case done := <-w.done:
			fmt.Println(done, "❌", w.isLeader.Load())
			if done && !w.isLeader.Load() {
				c.Close()
				return
			}
		default:
			msg, err := c.ReadMessage(-1)
			if err != nil {
				log.Printf("Kafka read error: %v", err)
				continue
			}

			var Abortdata []*common.Envelope
			err = json.Unmarshal(msg.Value, &Abortdata)
			if err != nil {
				log.Printf("Failed to unmarshal Kafka message: %v", err)
				continue
			}

			userIDs := []int{}
			for _, env := range Abortdata {
				payload, err := protoutil.UnmarshalTransaction(env.Payload)
				if err != nil {
					log.Printf("Failed to unmarshal transaction payload: %v", err)
					continue
				}

				match := re.FindStringSubmatch(payload.String())
				if len(match) > 1 {
					idx := idxToInt(match[1])
					userIDs = append(userIDs, idx)
				}
			}

			userCount := make(map[string]int)
			for _, id := range userIDs {
				userKey := "User" + strconv.Itoa(id)
				userCount[userKey]++
			}

			fmt.Println("User Count Map:", userCount)

			w.auditMsg = &pb.AuditMsg{}
			go w.bftConsensus(userCount)

		}
	}
}

func (w *CONSENSUSNODE) bftConsensus(userCount map[string]int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Construct initial consensus message
	w.auditMsg = &pb.AuditMsg{
		BlkNum:              w.roundIdx,
		LeaderID:            w.selfId,
		PhaseNum:            pb.AuditMsg_PREPARE,
		MerkleRootHash:      "abcdefghijklmnopqrstuvwxyz",
		AbortTransactionNum: userCount,
		HonestAuditors:      []string{w.selfId},
	}

	// Hashing
	if w.auditMsg.BlkNum == 1 {
		w.auditMsg.CurHash = "ajknadajsnamajkndqwaakdmkaiwq"
	} else {
		hashBytes := lg.BlockHashCalculator(w.auditMsg)
		hash := sha512.Sum512(hashBytes)
		w.auditMsg.CurHash = hex.EncodeToString(hash[:63])
	}

	// Signature
	signing := sec.SignByte([]byte(w.auditMsg.CurHash))
	w.auditMsg.Signature = signing.Serialize()

	// === 1st Phase: PREPARE ===
	fmt.Println("🚀 Broadcasting PREPARE")
	chPrepare := make(chan bool, len(w.members))
	for _, each := range w.members {
		go w.SubmitPrepare(chPrepare, each.Addr)
	}

	if waitForVotes(chPrepare, len(w.members)) {
		fmt.Println("✅ PREPARE Quorum achieved")
	} else {
		fmt.Println("⚠️ PREPARE Quorum not achieved")
	}
	<-ctx.Done()

	// === 2nd Phase: COMMIT ===
	fmt.Println("🚀 Broadcasting COMMIT")
	chCommit := make(chan bool, len(w.members))
	for _, each := range w.members {
		go w.SubmitCommit(chCommit, each.Addr)
	}

	if waitForVotes(chCommit, len(w.members)) {
		fmt.Println("✅ COMMIT Quorum achieved")
	} else {
		fmt.Println("⚠️ COMMIT Quorum not achieved")
	}
	<-ctx.Done()

	// === Final: Disseminate block ===
	fmt.Println("📦 Disseminating block to peers")
	gossipMsg := &pb.GossipMsg{
		Type:   2,
		Rndblk: w.auditMsg,
	}
	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossipMsg)
}

func waitForVotes(ch chan bool, total int) bool {
	votes := 0
	quorum := 1 // TODO: replace with 2f+1 logic if needed

	for {
		select {
		case <-ch:
			votes++
			fmt.Printf("🗳️ Votes: %d/%d\n", votes, total)
			if votes >= quorum {
				return true
			}
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
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

// Check if this peer is the leader; if so, it should receive messages from Kafka.
// BlockListening listens for consensus-related block messages.
func (w *CONSENSUSNODE) BlockListening() {
	w.GetAddress()
	listener, err := net.Listen("tcp", w.address+consensusPort)
	if err != nil {
		log.Fatalf("❌ Failed to listen on %s: %v", w.address+consensusPort, err)
	}
	defer listener.Close()

	fmt.Println("📡 BlockListenIP:", w.address+consensusPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("⚠️ Accept error:", err)
			continue
		}
		go w.BloConnHandler(conn)
	}
}

// BloConnHandler handles incoming consensus messages (leader/follower/block insertion).
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

	fmt.Println("🏦 Received GossipMsg:", MsgRecv)

	switch MsgRecv.GetType() {
	case 1:
		w.roundIdx = MsgRecv.RndMsg.RoundNum
		isLeader := (MsgRecv.RndMsg.LeaderID == w.selfId)

		w.leader = isLeader
		w.channelID = MsgRecv.Channel

		if isLeader {
			fmt.Println("👑 Elected as leader")
			go w.setDone(false)
			go w.isLeader.Store(true)
			go w.KafkaListener(w.channelID + "-abort")
			fmt.Println(w.isLeader.Load())
		} else {
			fmt.Println("🧩 Follower consensus node")
			go w.setDone(true)
			go w.isLeader.Store(false)
			fmt.Println(w.isLeader.Load())
		}

	case 2:
		fmt.Println("📦 BLOCKINSERT: Committing block to ledger")
		go lg.UserAbortInfoInsert(w.auditMsg)
	}
}

// decide whether the peer is elected as a leader or not
func (w *CONSENSUSNODE) CommitteeListening() {
	w.GetAddress()
	listener, err := net.Listen("tcp", w.address+gossipListenPort)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", w.address+gossipListenPort, err)
	}
	defer listener.Close()

	fmt.Println("📡 CommitteeListenIP:", w.address+gossipListenPort)

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

	w.roundIdx = recvMsg.RoundNum

	for _, shard := range recvMsg.Shards {
		for _, mem := range shard.Member {
			if mem.NodeID == w.selfId {
				w.members = shard.Member

				isLeader := (recvMsg.GetType() == 1 && shard.LeaderID == w.selfId)

				w.leader = isLeader
				w.channelID = leaderChan // use fixed ID if that is intended

				if isLeader {
					fmt.Println("👑 Elected as leader")
					go w.setDone(false)
					go w.isLeader.Store(true)
					go w.KafkaListener(w.channelID + "-abort")
					fmt.Println(w.isLeader.Load())
				} else {
					fmt.Println("🧩 Follower consensus node")
					go w.setDone(true)
					go w.isLeader.Store(false)
					fmt.Println(w.isLeader.Load())
				}

				return // exit after setting role

				// push := w.MP2BTPChildOnce()
				// w.MP2BTPConsensusListening(push)
			}
		}
	}
}

/*I'm a leader, send a message to a w-node*/
func (w *CONSENSUSNODE) SubmitPrepare(ch chan<- bool, ip string) {
	w.submitPhase(ch, ip, pb.AuditMsg_AGGREGATED_PREPARE)
}

func (w *CONSENSUSNODE) SubmitCommit(ch chan<- bool, ip string) {
	w.submitPhase(ch, ip, pb.AuditMsg_AGGREGATED_COMMIT)
}

func (w *CONSENSUSNODE) submitPhase(ch chan<- bool, ip string, targetPhase pb.AuditMsg_Phases) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	session, err := quic.DialAddr(context.Background(), ip+gossipMsgPort, tlsConf, nil)
	if err != nil {
		log.Printf("❌ Failed to connect to %s: %v", ip, err)
		ch <- false
		return
	}
	defer session.CloseWithError(0, "")

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Printf("❌ Failed to open stream: %v", err)
		ch <- false
		return
	}

	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		log.Printf("❌ Marshal error: %v", err)
		ch <- false
		return
	}
	_, err = stream.Write(sndBuf)
	if err != nil {
		log.Printf("❌ Write error: %v", err)
		ch <- false
		return
	}

	remoteIP := session.RemoteAddr().String()
	fmt.Println("연결된 상대 IP:", remoteIP)

	for {
		rcvBuf := make([]byte, 8192)
		n, err := stream.Read(rcvBuf)
		if err != nil {
			// log.Printf("❌ Stream read error: %v", err)
			ch <- false
			break
		}
		if n == 0 {
			continue
		}

		cMsgRecv := &pb.AuditMsg{}
		if err := json.Unmarshal(rcvBuf[:n], cMsgRecv); err != nil {
			log.Println("❌ JSON unmarshal error:", err)
			continue
		}

		if cMsgRecv.PhaseNum != targetPhase {
			continue
		}

		sig := bls.Sign{}
		if err := sig.Deserialize(cMsgRecv.Signature); err != nil {
			log.Println("❌ Signature deserialize error:", err)
			continue
		}
		sigVec = append(sigVec, sig)
		if len(sigVec) == len(w.memberMsg.Nodes) {
			log.Printf("✅ Phase %v Aggregation Complete", targetPhase)
			w.Multisinging(cMsgRecv, sigVec)
			sigVec = sigVec[:0]
			if targetPhase == pb.AuditMsg_AGGREGATED_PREPARE {
				w.auditMsg.HonestAuditors = cMsgRecv.HonestAuditors
			}
			break
		} else {
			log.Printf("⏳ Waiting for more signatures: %d/%d", len(sigVec), len(w.memberMsg.Nodes))
			continue
		}
	}

	stream.Close()
	ch <- true
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
			// log.Printf("❌ Stream read error: %v", err)
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
	hash := []byte(cMsg.CurHash)
	signing := sec.SignByte(hash)
	w.auditMsg.Signature = signing.Serialize()
	w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE
	w.updateAuditFields(cMsg)
}

func (w *CONSENSUSNODE) commitPhase(cMsg *pb.AuditMsg) {
	hash := []byte(cMsg.CurHash)
	signing := sec.SignByte(hash)
	w.auditMsg.Signature = signing.Serialize()
	w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
	w.updateAuditFields(cMsg)
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

func (w *CONSENSUSNODE) updateAuditFields(cMsg *pb.AuditMsg) {
	w.auditMsg.CurHash = cMsg.CurHash
	w.auditMsg.MerkleRootHash = cMsg.MerkleRootHash

	alreadyExists := false
	for _, id := range cMsg.HonestAuditors {
		if id == w.selfId {
			alreadyExists = true
			break
		}
	}
	if !alreadyExists {
		w.auditMsg.HonestAuditors = append(cMsg.HonestAuditors, w.selfId)
	} else {
		w.auditMsg.HonestAuditors = cMsg.HonestAuditors
	}
}

func (w *CONSENSUSNODE) verifying(cMsg *pb.AuditMsg) bool {
	pubVec = pubVec[:0]

	for _, honestID := range cMsg.HonestAuditors {
		if pk := w.getPublicKeyByNodeID(honestID); pk != nil {
			pubVec = append(pubVec, *pk)
		} else {
			fmt.Printf("Missing public key for node: %s\n", honestID)
		}
	}

	fmt.Printf("👑 Phase: %v\n👑 Public Keys Count: %d\n👑 Public Keys: %v\n", cMsg.PhaseNum, len(pubVec), pubVec)

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

/*GRPC SERVICE Membershhip Exchanging Procedure*/
func (w *CONSENSUSNODE) Reporting() {
	conn, err := grpc.Dial(validatorGrpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect to validator gRPC server: %v", err)
	}
	defer conn.Close()

	c := pb.NewMembershipServiceClient(conn)
	req := createSelfMembership(w.selfId, pub.Serialize(), "11730")

	w.memberMsg, err = c.GetMembership(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to get membership: %v", err)
	}
}

func createSelfMembership(id string, pubKey []byte, port string) *pb.MemberMsg {
	node := &pb.Node{
		NodeID:    id,
		Port:      port,
		Publickey: pubKey,
		Alive:     true,
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

// MP2BTP + WDN

// -------------------------------------------------------------------------------------- //
var mu sync.Mutex

func (w *CONSENSUSNODE) MP2BTPRootOnce(ch chan<- bool, addrs []string) *PuCtrl.PuCtrl {
	mu.Lock()
	defer mu.Unlock()
	w.GetAddress()

	peerId := rd.Intn(10) + 1

	configFile := "./push_root.toml"
	fmt.Println("MP2BTPSubmit IP:", addrs)

	err := WriteTomlFile(configFile, w.address, addrs)
	if err != nil {
		fmt.Println("❌ TOML 파일 생성 실패:", err)
		ch <- false
		return nil
	}

	fmt.Println("✅ TOML 파일 생성 완료:", configFile)

	var config PuCtrl.Config
	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		panic(err)
	}

	pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

	fmt.Println("🖥️", config.PEER_ADDRS, "🖥️")

	nodeInfo := pu.GetNodeInfo(config.PEER_ADDRS)

	pu.Listen()

	time.Sleep(1 * time.Second)

	pu.Connect(nodeInfo, byte(PuCtrl.PUSH_SESSION))

	time.Sleep(1 * time.Second)
	return pu
}
func (w *CONSENSUSNODE) MP2BTPSubmit(ch chan<- bool, pu *PuCtrl.PuCtrl) {

	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		fmt.Println("❌ JSON 직렬화 실패:", err)
		ch <- false
		return
	}
	pu.SendAuditMsg(sndBuf)
	fmt.Println("💛FINISH", sndBuf, "💛FINISH")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan recvResult)
	go receiveWorker(pu, resultCh)

	for {
		select {
		case result := <-resultCh:
			if result.err != nil {
				fmt.Println("❌ Error:", result.err)
				ch <- false
				return
			}
			if result.buf == nil || len(result.buf) == 0 {
				continue
			}
			datalength := int(result.buf[0])
			if datalength > len(result.buf)-1 {
				fmt.Println("⚠️ 데이터 길이 이상:", datalength)
				ch <- false
				return
			}
			resultData := make([]byte, datalength)
			copy(resultData, result.buf[1:datalength+1])

			fmt.Printf("📦 수신 (%d bytes): %v\n", datalength, resultData)
			ch <- true
			break

		case <-ctx.Done():
			fmt.Println("🛑 수신 시간 초과 또는 취소")
			ch <- false
			return
		}
		break
	}

}

type recvResult struct {
	n   int
	err error
	buf []byte
}

func receiveWorker(pu *PuCtrl.PuCtrl, resultCh chan<- recvResult) {
	for {
		buf := make([]byte, 8192)
		buf, err := pu.ReceiveRaw(buf)
		if err != nil {
			resultCh <- recvResult{buf: nil, err: err}
			return
		}
		resultCh <- recvResult{buf: buf, err: nil}
	}
}
func (w *CONSENSUSNODE) MP2BTPChildOnce() *PuCtrl.PuCtrl {
	fmt.Println("😬 Starting Consensus Child")

	configFile := "./push_child.toml"
	peerId := rd.Intn(10) + 1

	if _, err := toml.DecodeFile(configFile, new(PuCtrl.Config)); err != nil {
		fmt.Println("❌ Config 파일 파싱 실패:", err)
		return nil
	}

	pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

	time.Sleep(1 * time.Second)

	pu.Listen()

	time.Sleep(1 * time.Second)
	return pu
}

func (w *CONSENSUSNODE) MP2BTPConsensusListening(pu *PuCtrl.PuCtrl) {

	s, _ := os.Create(fmt.Sprintf("log.txt"))
	defer s.Close()

	startTime := time.Now()
	recvBytes := 0
	datalength := 0
	resultData := make([]byte, datalength)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan recvResult)
	go receiveWorker(pu, resultCh)

	for {
		select {
		case result := <-resultCh:
			if result.err != nil {
				fmt.Println("❌ Error:", result.err)
				return
			}
			if result.buf == nil || len(result.buf) == 0 {
				continue
			}
			fmt.Println("⭐️")
			datalength = int(result.buf[0])
			if datalength > len(result.buf)-1 {
				fmt.Println("⚠️ 데이터 길이 이상:", datalength)
				return
			}

			copy(resultData, result.buf[1:datalength+1])
			fmt.Printf("📦 수신 (%d bytes): %v\n", datalength, resultData)
			break

		case <-ctx.Done():
			fmt.Println("🛑 수신 시간 초과 또는 취소")
			return
		}
		break
	}

	elapsedTime := time.Since(startTime)
	throughput := (float64(recvBytes) * 8.0) / elapsedTime.Seconds() / (1000 * 1000)
	logStr := fmt.Sprintf("Seconds=%f, Throughput=%f, ReceivedSize=%d\n", elapsedTime.Seconds(), throughput, recvBytes)
	s.Write([]byte(logStr))

	var auditMsg pb.AuditMsg
	fmt.Println(datalength, resultData[1:datalength+1])
	if err := json.Unmarshal(resultData[1:datalength+1], &auditMsg); err != nil {
		fmt.Println("Unmarshal error:", err)
	}

	if auditMsg.PhaseNum == pb.AuditMsg_PREPARE {
		hash := []byte(auditMsg.PrevHash)
		signing := sec.SignByte(hash)
		byteSig := signing.Serialize()
		w.auditMsg.Signature = byteSig
		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE
	}

	if auditMsg.PhaseNum == pb.AuditMsg_COMMIT {
		hash := []byte(auditMsg.PrevHash)
		signing := sec.SignByte(hash)
		byteSig := signing.Serialize()
		w.auditMsg.Signature = byteSig
		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
	}

	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		panic(err)
	}
	err = pu.SendAuditMsg(sndBuf)
	if err != nil {
		panic(err)
	}
}

// -------------------------------------------------------------------------------------- //

func WriteTomlFile(fileName string, myIP string, addrs []string) error {
	_ = os.Remove(fileName)
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	fixedConfig := fmt.Sprintf(`# MP2BTP Configurations
VERBOSE_MODE        = true
NUM_MULTIPATH       = 1
MY_IP_ADDRS         = ["%s", "127.0.0.1"]
LISTEN_PORT         = 4000
BIND_PORT           = 5000
EQUIC_ENABLE        = true
EQUIC_PATH          = "./fectun"
EQUIC_APP_SRC_PORT  = 6000
EQUIC_APP_DST_PORT  = 5000
EQUIC_TUN_SRC_PORT  = 7000
EQUIC_TUN_DST_PORT  = 7000
EQUIC_FEC_MODE      = false
SEGMENT_SIZE        = 2097152  
FEC_SEGMENT_SIZE    = 2466816  
THROUGHPUT_PERIOD = 0.1
THROUGHPUT_WEIGHT = 0.1
MULTIPATH_THRESHOLD_THROUGHPUT = 100.0
MULTIPATH_THRESHOLD_SEND_COMPLETE_TIME = 0.0000000001
CLOSE_TIMEOUT_PERIOD = 200
`, myIP)
	_, err = f.WriteString(fixedConfig)
	if err != nil {
		return err
	}

	// [[peer_addr]]
	selfBlock := fmt.Sprintf(
		"\n[[peer_addr]]  # Offset=0\nAddr = \"%s\"\nNumChild = %d\nChildOffset = 1\n",
		myIP,
		len(addrs),
	)
	_, err = f.WriteString(selfBlock)
	if err != nil {
		return err
	}

	// 2. 나머지 peer addr들 작성 (Offset=1부터 시작)
	for i, addr := range addrs {
		block := fmt.Sprintf(
			"\n[[peer_addr]]  # Offset=%d\nAddr = \"%s\"\nNumChild = 0\nChildOffset = 0\n",
			i+1, addr,
		)
		_, err := f.WriteString(block)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *CONSENSUSNODE) MP2BTPbftConsensus(userCount map[string]int) {
	// Consensus Timer Setup
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(10)*time.Second)
	defer cancel()

	// Make a consensus message
	w.auditMsg.BlkNum = w.roundIdx
	w.auditMsg.LeaderID = w.selfId
	w.auditMsg.PhaseNum = pb.AuditMsg_PREPARE
	w.auditMsg.MerkleRootHash = "abcdefghijklmnopqrstuvwxyz"
	w.auditMsg.AbortTransactionNum = userCount

	// Calculate Block Hash Value, Current Hash Value is determined by the following 6 field values
	// BlkNum, LeaderID, PhaseNum, MerkleRootHash, TxListm, PrevHash
	if w.auditMsg.BlkNum == 1 {
		w.auditMsg.CurHash = "ajknadajsnamajkndqwaakdmkaiwq"
	} else {
		b := lg.BlockHashCalculator(w.auditMsg)
		h := sha512.Sum512(b)
		w.auditMsg.CurHash = hex.EncodeToString(h[:63])
	}

	// Signing
	hash := []byte(w.auditMsg.CurHash)
	signing := sec.SignByte(hash)
	byteSig := signing.Serialize()

	w.auditMsg.Signature = byteSig

	//  Who signed
	w.auditMsg.HonestAuditors = append(w.auditMsg.HonestAuditors, w.selfId)

	// [Send the first consensus message] Send sensus message to each w-node
	// When the settlement time expires, the message transfer goroutin ends
	ch1 := make(chan bool, len(w.members))

	go w.MP2BTPSubmit(ch1, MP2BTPsession)

	// 2f+1 messages must be processed to send a second consensus message
	// [Second consensual message] Send consensus message to each w-node
	// When the settlement time expires, the message transfer goroutin ends

	votes := 0
	for {
		select {
		case <-ch1:
			votes += 1
		}
		if votes == len(w.members) {
			break
		}
	}
	<-ctx.Done()
	fmt.Println("✅ 응답 받음1, 다음 작업 진행")

	ch2 := make(chan bool, len(w.members))

	go w.MP2BTPSubmit(ch2, MP2BTPsession)

	// Final block commitment only if 2f+1 message is successful
	// Send sensus message to each w-node
	// to sync goroutines
	votes = 0
	for {
		select {
		case <-ch2:
			votes += 1
			fmt.Println("⭐️", votes)
		}
		if votes == len(w.members) {
			fmt.Println(votes)
			break
		}
		fmt.Println(votes)
	}
	<-ctx.Done()
	fmt.Println("✅ 응답 받음2, 다음 작업 진행")

	go w.MP2BTPSubmit(ch2, MP2BTPsession)

	// when the consensus was successfully done, disseminate
	// the block to the w-nodes.
	gossibMsg := &pb.GossipMsg{}
	gossibMsg.Type = 2
	gossibMsg.Rndblk = w.auditMsg
	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossibMsg)
}
