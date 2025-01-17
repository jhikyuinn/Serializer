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
	"path"
	"regexp"
	"runtime/metrics"
	"strconv"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/golang/protobuf/proto"
	"github.com/herumi/bls-eth-go-binary/bls"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	quic "github.com/quic-go/quic-go"
	"google.golang.org/grpc"
)

type HLFEncodedBlock struct {
	BlockHeader   BlockHeader
	BlockData     BlockData
	BlockMetadata BlockMetadata
	Padding       []int
}
type BlockHeader struct {
	Number       uint64
	PreviousHash []byte
	DataHash     []byte
}
type BlockData struct {
	Data [][]byte
}
type BlockMetadata struct {
	Metadata [][]byte
}

type Fabric struct {
	Num          uint64
	PreviousHash []byte
	DataHash     []byte
	Consensus    bool
}

type WNODE struct {
	wlog *log.Logger

	address   string
	selfId    string
	roundIdx  uint32
	members   []*pb.Wnode
	isLeader  chan bool
	leader    bool
	channelID string
	done      chan bool

	blockMsg  *pb.TransactionMsg
	memberMsg *pb.MemberMsg
	auditMsg  *pb.AuditMsg
}

var (
	sAddr    = "localhost:16220" // [S-Node] IP:Port for gRPC
	sAddrNAT = "localhost:11730" // [S-Node] IP:Port for AWS NAT

	LPORT = ":5252" // [W-Node] Port Number for consensus listening
	GPORT = ":4242" // [W-Node] Gossip Msg Listening Port Number
	HPORT = ":4243"
	MPORT = ":12345" // [Prometheus]

	addrL = "117.16.244.33" // [Default Leader Node] SAMSUNG IP

	brokers = ""

	leaderBro  = false
	leaderChan = ""
)

func Kafka() *kafka.Consumer {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "myGroup",
		"auto.offset.reset": "earliest",
	})

	if err != nil {
		// When a connection error occurs, a panic occurs and the system is shut down
		panic(err)
	}

	return c
}

var NUM_MSG int

// My private and public key
var sec bls.SecretKey
var pub *bls.PublicKey

// All nodes' private & public key
var sigVec []bls.Sign
var pubVec []bls.PublicKey

// METRICS for PROMETHEUS MONITORING
var counter = prometheus.NewCounter(
	prometheus.CounterOpts{
		Namespace: "WDN_WNODE",
		Name:      "WDN_counter",
		Help:      "This is WDN counter",
	})

var gauge = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "WDN_WNODE",
		Name:      "WDN_gauge",
		Help:      "This is WDN gauge",
	})

var histogram = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Namespace: "WDN_WNODE",
		Name:      "WDN_histogram",
		Help:      "This is WDN histogram",
	})

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
		// export the value want
		for {
			counter.Add(rd.Float64() * 5)
			gauge.Add(rd.Float64()*15 - 5)
			histogram.Observe(rd.Float64() * 10)

			metrics.Read(getMetric)
			time.Sleep(2 * time.Second)
		}
	}()
	fmt.Println(http.ListenAndServe(MPORT, nil))
}

func main() {
	snode := flag.String("snode", "117.16.244.33", "")
	kafka := flag.String("broker", "117.16.244.33", "")
	libp2pAddr := flag.String("libp2p", "/ip4/117.16.244.33/tcp/4041/p2p/QmWRVJEifrHHyc1DEDTNfuAeDGtvkT6cSdPAMUGc12KxX7", "")
	ami := flag.Bool("ami", false, "")
	chann := flag.String("channel", "mychannel", "")

	flag.Parse()

	sAddr = *snode + ":16220"
	sAddrNAT = *snode + ":11730"

	brokers = *kafka + ":9091, " + *kafka + ":9092, " + *kafka + ":9093"
	leaderBro = *ami
	leaderChan = *chann

	var wNode WNODE

	LOGFILE := path.Join("./wnode.log")
	f, err := os.OpenFile(LOGFILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	wNode = WNODE{
		wlog:      log.New(f, "WLog ", log.LstdFlags),
		isLeader:  make(chan bool),
		leader:    false,
		done:      make(chan bool),
		blockMsg:  &pb.TransactionMsg{},
		auditMsg:  &pb.AuditMsg{},
		memberMsg: &pb.MemberMsg{},
	}
	wNode.wlog.Println("Start")

	// GossipSub Channel Join within 3 second
	go wp2p.Start(true, *libp2pAddr, 4010)
	time.Sleep(3 * time.Second)
	// WDG ID is calculated by hashing IP address.
	wNode.selfId = wp2p.Host
	fmt.Println("[HOST ID]", wNode.selfId)

	/*  Initialize BLS multi-signature
	    Thus, w-node get his  secret key and public key
	*/
	bls.Init(bls.BLS12_381)
	bls.SetETHmode(bls.EthModeDraft07)
	sec.SetByCSPRNG()
	pub = sec.GetPublicKey()

	//  Export PROMETHEUS Metric for health check
	http.Handle("/metrics", promhttp.Handler())
	go monitoring()

	myChannel := leaderChan
	wp2p.JoinShard(myChannel)

	//  Let's start to communicate with s-node
	wNode.start()
}

// func (w *WNODE) GetAddress() string {
// 	// ifaces, _ := net.Interfaces()
// 	// for _, i := range ifaces {
// 	// 	// "eth0" or "eno1"
// 	// 	if i.Name == "eth0" {
// 	// 		addrs, _ := i.Addrs()
// 	// 		for _, addr := range addrs {
// 	// 			var ip net.IP
// 	// 			switch v := addr.(type) {
// 	// 			case *net.IPNet:
// 	// 				ip = v.IP
// 	// 			case *net.IPAddr:
// 	// 				ip = v.IP
// 	// 			}
// 	// 			w.address = ip.String()
// 	// 			return true
// 	// 		}
// 	// 	}
// 	// }
// }

func (w *WNODE) start() {
	go w.setLeaderChan(false, "")
	go w.setDone(false)
	w.leader = false

	// [GossipSub: Tx Proposal Listening]
	// 1. All w-nodes receive every round of Gissip round messages from s-nodes
	// 2. All w-nodes receive every round of Gissip block messages from the leader
	// 3. In other words, there are two message types (tpye-1) round msg (type-2) Block Msg
	// go w.BlockListening()

	go w.CommitteeListening()

	// [QUIC: Consensus Message]
	// 1. The leader node forwards the consensus message to all w-nodes
	// 2. Each w-node is signed and delivered to the leader node
	// 3. Wait for consensus message if you are not a leader
	go w.ConsensusListening()

	// [GRPC: Membership Message]
	//	1. Report his network information
	//	2. Get the information WDN Members
	w.Reporting()

	// To solve AWS NAT Problem (외부 IP 주소를 알려주기 위해)
	time.Sleep(time.Second * 3)
	ReportExternal(w.selfId)
	for {
		time.Sleep(time.Second * 10)
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
func (w *WNODE) KafkaListener(rekey string) {

	c := Kafka()
	c.SubscribeTopics([]string{rekey}, nil)

	var userIDs []int

	for {
		select {
		case done := <-w.done:
			if done {
				c.Close()
				return
			}
		default:
			msg, err := c.ReadMessage(-1)
			if err == nil {
				var index = 0
				var Abortdata []*common.Envelope
				err := json.Unmarshal(msg.Value, &Abortdata)
				if err != nil {
					fmt.Println(err)
				}
				for i := range Abortdata {
					payload, _ := protoutil.UnmarshalTransaction(Abortdata[i].Payload)
					fmt.Println("③", payload)
					re := regexp.MustCompile(`User(\d+)`)
					match := re.FindStringSubmatch(payload.String())
					if len(match) > 1 {
						idx := match[1]
						index = idxToInt(idx)
						userIDs = append(userIDs, index)
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
}

func (w *WNODE) bftConsensus(userCount map[string]int) {
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
	for _, each := range w.members {
		wnodeIP := each.Addr
		go w.SubmitPrepare(ch1, wnodeIP)
	}

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
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Millisecond):
	}

	ch2 := make(chan bool, len(w.members))
	for _, each := range w.members {
		wnodeIP := each.Addr
		go w.SubmitCommit(ch2, wnodeIP)
	}

	// Final block commitment only if 2f+1 message is successful
	// Send sensus message to each w-node
	// to sync goroutines
	votes = 0
	for {
		select {
		case <-ch2:
			votes += 1
		}
		if votes == len(w.members) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Millisecond):
		// go lg.BlkInsert(w.auditMsg)
		go lg.UserAbortInfoInsert(w.auditMsg)
	}

	// when the consensus was successfully done, disseminate
	// the block to the w-nodes.
	gossibMsg := &pb.GossipMsg{}
	gossibMsg.Type = 2
	gossibMsg.Rndblk = w.auditMsg
	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossibMsg)
}

func (w *WNODE) setLeaderChan(b bool, channel string) {
	if w.channelID != channel {
		w.channelID = channel
	}
	w.isLeader <- b
}

func (w *WNODE) setDone(b bool) {
	w.done <- b
}

// decide whether the peer is elected as a leader or not
func (w *WNODE) BlockListening() {
	l, err := net.Listen("tcp", "localhost"+GPORT)
	if nil != err {
		log.Println(err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if nil != err {
			log.Println(err)
			continue
		}
		go w.ConnHandler(conn)
	}
}

func (w *WNODE) ConnHandler(conn net.Conn) {
	defer conn.Close()
	recvBuf := make([]byte, 8192)
	n, err := conn.Read(recvBuf)
	data := recvBuf[:n]
	if nil != err {
		if io.EOF == err {
			log.Printf("connection is closed from client; %v", conn.RemoteAddr().String())
			return
		}
		log.Printf("fail to receive data; err: %v", err)
		return
	}

	MsgRecv := &pb.GossipMsg{}
	json.Unmarshal(data, MsgRecv)

	w.roundIdx = MsgRecv.RndMsg.RoundNum

	// Start to make a consensus if I'm a leader

	fmt.Println(MsgRecv.RndMsg.LeaderID)

	if MsgRecv.GetType() == 1 && MsgRecv.RndMsg.LeaderID == w.selfId {
		if !w.leader {
			w.leader = true
			w.channelID = MsgRecv.Channel

			go w.setDone(false)
			go w.setLeaderChan(true, MsgRecv.Channel)

			go w.KafkaListener(MsgRecv.Channel + "-abort")
		}
	} else if MsgRecv.GetType() == 1 && MsgRecv.RndMsg.LeaderID != w.selfId {
		w.leader = false
		go w.setDone(true)
		go w.setLeaderChan(false, "")
	}

	// Commit the block in their own ledger
	if MsgRecv.GetType() == 2 {
		// go lg.BlkInsert(MsgRecv.Rndblk)
		go lg.UserAbortInfoInsert(w.auditMsg)
	}
}

// decide whether the peer is elected as a leader or not
func (w *WNODE) CommitteeListening() {
	l, _ := net.Listen("tcp", "localhost"+GPORT)
	defer l.Close()
	for {
		conn, err := l.Accept()
		if nil != err {
			log.Println(err)
			continue
		}

		go w.CommConnHandler(conn)
	}
}

func (w *WNODE) CommConnHandler(conn net.Conn) {
	defer conn.Close()
	recvBuf := make([]byte, 8192)
	n, err := conn.Read(recvBuf)
	data := recvBuf[:n]
	if nil != err {
		if io.EOF == err {
			log.Printf("connection is closed from client; %v", conn.RemoteAddr().String())
			return
		}
		log.Printf("fail to receive data; err: %v", err)
		return
	}
	recvMsg := &pb.CommitteeMsg{}
	json.Unmarshal(data, recvMsg)

	w.roundIdx = recvMsg.RoundNum

	var community pb.Shard

	for _, shard := range recvMsg.Shards {
		for _, mem := range shard.Member {
			if mem.NodeID == w.selfId {
				w.members = shard.Member
				community = *shard
				if recvMsg.GetType() == 1 && community.LeaderID == w.selfId {

					if !w.leader {
						w.leader = true
						w.channelID = community.ID
						w.channelID = leaderChan

						go w.setDone(false)
						go w.setLeaderChan(true, w.channelID)

						go w.KafkaListener(w.channelID + "-abort")
					}
				} else {
					w.leader = false
					w.channelID = community.ID
					w.channelID = leaderChan

					go w.setDone(true)
					go w.setLeaderChan(false, "")
				}

			}
		}
	}
}

/*I'm a leader, send a message to a w-node*/
func (w *WNODE) SubmitPrepare(ch chan<- bool, ip string) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	session, err := quic.DialAddr(context.Background(), addrL+LPORT, tlsConf, nil)
	if err != nil {
		panic(err)
	}
	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		panic(err)
	}
	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		panic(err)
	}
	_, err = stream.Write(sndBuf)
	if err != nil {
		panic(err)
	}

	/* Wait for consensus response */
	cMsgRecv := &pb.AuditMsg{}
	rcvBuf := make([]byte, 8192)

	_, err = stream.Read(rcvBuf)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(rcvBuf, cMsgRecv)

	err = stream.Close()
	if err != nil {
		panic(err)
	}
	ch <- true
}

/* I'm a leader, send a message to a w-node*/
func (w *WNODE) SubmitCommit(ch chan<- bool, ip string) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	// addrN: Audit Leader IP/PORT
	session, err := quic.DialAddr(context.Background(), "localhost"+LPORT, tlsConf, nil) // addrN
	if err != nil {
		panic(err)
	}
	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		panic(err)
	}
	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		panic(err)
	}
	_, err = stream.Write(sndBuf)
	if err != nil {
		panic(err)
	}

	cMsgRecv := &pb.AuditMsg{}
	rcvBuf := make([]byte, 8192)

	_, err = stream.Read(rcvBuf)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(rcvBuf, cMsgRecv)
	err = stream.Close()
	if err != nil {
		panic(err)
	}
	ch <- true
}

/*Consensus three-phases: Announce(Completed State), Prepare, Commit] I'm not a leader*/
func (w *WNODE) ConsensusListening() {
	listener, err := quic.ListenAddr(w.address+LPORT, generateTLSConfig(), nil)
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
func (w *WNODE) StreamHandler(stream quic.Stream) {
	rcvBuf := make([]byte, 8192)
	cMsgRecv := &pb.AuditMsg{}
	_, err := stream.Read(rcvBuf)
	if err != nil {
		if io.EOF == err {
			return
		}
		log.Printf("fail to receive data; err: %v", err)
	}
	json.Unmarshal(rcvBuf, cMsgRecv)

	if cMsgRecv.PhaseNum == pb.AuditMsg_PREPARE {
		hash := []byte(cMsgRecv.PrevHash)
		signing := sec.SignByte(hash)
		byteSig := signing.Serialize()
		w.auditMsg.Signature = byteSig
		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE
	}

	if cMsgRecv.PhaseNum == pb.AuditMsg_COMMIT {
		hash := []byte(cMsgRecv.PrevHash)
		signing := sec.SignByte(hash)
		byteSig := signing.Serialize()
		w.auditMsg.Signature = byteSig
		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
	}

	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		panic(err)
	}
	_, err = stream.Write(sndBuf)
	if err != nil {
		panic(err)
	}
}

func (w *WNODE) SendAggregatedMsg(stream quic.Stream) {
	sndBuf, err := proto.Marshal(w.auditMsg)
	if err != nil {
		panic(err)
	}
	_, err = stream.Write(sndBuf)
	if err != nil {
		panic(err)
	}
}

func (w *WNODE) Multisinging(cMsgs *pb.AuditMsg) (bool, int) {
	if cMsgs.PhaseNum == pb.AuditMsg_PREPARE {
		fmt.Println("............................... L-Node Phase 1")
		// Multi-Signature Signing
		var aggeSign bls.Sign
		aggeSign.Aggregate(sigVec)
		sigVec = sigVec[:0]
		byteSig := aggeSign.Serialize()

		w.auditMsg.BlkNum = cMsgs.BlkNum
		w.auditMsg.PrevHash = cMsgs.PrevHash
		w.auditMsg.CurHash = cMsgs.CurHash
		w.auditMsg.Signature = byteSig
		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE

		return true, len(w.auditMsg.HonestAuditors)
	}
	if cMsgs.PhaseNum == pb.AuditMsg_COMMIT {
		fmt.Println("............................... L-Node Phase 2")
		// Multi-Signature Signing
		var aggeSign bls.Sign
		aggeSign.Aggregate(sigVec)
		sigVec = sigVec[:0]
		byteSig := aggeSign.Serialize()

		w.auditMsg.BlkNum = cMsgs.BlkNum
		w.auditMsg.PrevHash = cMsgs.PrevHash
		w.auditMsg.CurHash = cMsgs.CurHash

		// Next LeaderID
		fmt.Println("L-Node ID:", w.memberMsg.LeaderID)

		w.auditMsg.Signature = byteSig
		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
		return true, len(w.auditMsg.HonestAuditors)
	}
	return false, 0
}

/*GRPC SERVICE Membershhip Exchanging Procedure*/
func (w *WNODE) Reporting() {
	conn, err := grpc.Dial(sAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewMembershipServiceClient(conn)
	Membership := &pb.MemberMsg{}
	node := new(pb.Node)
	node.NodeID = w.selfId
	node.Port = "11730" // listening port when the leader is elected
	bytePub := pub.Serialize()
	node.Publickey = bytePub
	node.Alive = true
	// Send my info to S-node
	Membership.Nodes = append(Membership.Nodes, node)

	// Send your membership information, receive all w-nodes membership
	w.memberMsg, err = c.GetMembership(context.Background(), Membership)
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
}

func MerkleHash(s []uint64) string {
	ss := int64(s[0])
	xx := strconv.FormatInt(ss, 16)
	return xx
}

func ReportExternal(id string) {
	sndBuf, _ := json.Marshal(id)
	conn, err := net.Dial("tcp", sAddrNAT)
	if err != nil {
		fmt.Println("[NAT] Faield to Dial : ", err)
	}
	defer conn.Close()
	_, err = conn.Write(sndBuf)

	if err != nil {
		fmt.Println("[NAT] Failed to write data : ", err)
	}
	_ = conn.Close()
}

/* Setup a bare-bones TLS config for the server --------------------------------------------------------------------- */
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
