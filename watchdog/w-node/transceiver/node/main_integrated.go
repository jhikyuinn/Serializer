package main

// import (
// 	lg "auditchain/ledger"
// 	pb "auditchain/msg"
// 	wp2p "auditchain/wp2p"
// 	"context"
// 	"crypto/rand"
// 	"crypto/rsa"
// 	"crypto/sha512"
// 	"crypto/tls"
// 	"crypto/x509"
// 	"encoding/hex"
// 	"encoding/json"
// 	"encoding/pem"
// 	"flag"
// 	"fmt"
// 	"io"
// 	"log"
// 	"math/big"
// 	rd "math/rand"
// 	"net"
// 	"net/http"
// 	"os"
// 	"regexp"
// 	"runtime/debug"
// 	"runtime/metrics"
// 	"strconv"
// 	"strings"
// 	"sync"
// 	"sync/atomic"
// 	"time"

// 	"github.com/BurntSushi/toml"
// 	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
// 	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
// 	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
// 	"github.com/herumi/bls-eth-go-binary/bls"
// 	"github.com/hyperledger/fabric-protos-go/common"
// 	"github.com/hyperledger/fabric/protoutil"
// 	"github.com/prometheus/client_golang/prometheus"
// 	"github.com/prometheus/client_golang/prometheus/promhttp"
// 	quic "github.com/quic-go/quic-go"
// 	"google.golang.org/grpc"
// )

// type Fabric struct {
// 	Num          uint64
// 	PreviousHash []byte
// 	DataHash     []byte
// 	Consensus    bool
// }

// type recvResult struct {
// 	n   int
// 	err error
// 	buf []byte
// }

// type CONSENSUSNODE struct {
// 	address   string
// 	selfId    string
// 	roundIdx  uint32
// 	members   []*pb.Wnode
// 	isLeader  atomic.Bool
// 	leader    bool
// 	channelID string
// 	done      chan bool

// 	kafkaConsumer *kafka.Consumer
// 	blockMsg      *pb.TransactionMsg
// 	memberMsg     *pb.MemberMsg
// 	auditMsg      *pb.AuditMsg
// 	isRunning     atomic.Bool
// }

// const baseleaderTomlConfig = `LISTEN_PORT         = 4000
// BIND_PORT           = 5000
// EQUIC_ENABLE        = true
// EQUIC_PATH          = "./fectun"
// EQUIC_APP_SRC_PORT  = 6000
// EQUIC_APP_DST_PORT  = 5000
// EQUIC_TUN_SRC_PORT  = 7000
// EQUIC_TUN_DST_PORT  = 7000
// EQUIC_FEC_MODE      = false
// SEGMENT_SIZE        = 2097152
// FEC_SEGMENT_SIZE    = 2466816
// THROUGHPUT_PERIOD   = 0.1
// THROUGHPUT_WEIGHT   = 0.1
// MULTIPATH_THRESHOLD_THROUGHPUT = 100.0
// MULTIPATH_THRESHOLD_SEND_COMPLETE_TIME = 0.0000000001
// CLOSE_TIMEOUT_PERIOD = 200
// `

// const (
// 	validatorIP          = "117.16.244.33"
// 	validatorGrpcPort    = "16220"
// 	validatorNatPort     = "11730"
// 	gossipMsgPort        = ":4242"
// 	grpcPort             = ":5252"
// 	gossipListenPort     = ":6262"
// 	prometheusListenPort = ":12345"
// )

// var (
// 	validatorGrpcAddr = validatorIP + ":" + validatorGrpcPort
// 	validatorNatAddr  = validatorIP + ":" + validatorNatPort

// 	brokers      = ""
// 	kafkaGroupID = "MYGROUP1"
// 	leaderBro    = false
// 	leaderChan   = ""

// 	sec    bls.SecretKey
// 	pub    *bls.PublicKey
// 	sigVec []bls.Sign
// 	pubVec []bls.PublicKey
// )

// var consensusStatusMap = make(map[string]bool)
// var mu sync.RWMutex

// var ctx, cancel = context.WithCancel(context.Background())

// var consensusprotocol *bool

// var MP2BTPsession *PuCtrl.PuCtrl

// func NewKafkaConsumer() *kafka.Consumer {
// 	c, err := kafka.NewConsumer(&kafka.ConfigMap{
// 		"bootstrap.servers":    brokers,
// 		"group.id":             kafkaGroupID,
// 		"auto.offset.reset":    "earliest",
// 		"session.timeout.ms":   30000,
// 		"max.poll.interval.ms": 300000,
// 		// "enable.auto.commit": false,
// 	})
// 	if err != nil {
// 		log.Fatalf("Failed to create Kafka consumer: %v", err)
// 	}
// 	return c
// }

// var (
// 	counter = prometheus.NewCounter(prometheus.CounterOpts{
// 		Namespace: "WDN_WNODE",
// 		Name:      "wdn_counter_total",
// 		Help:      "Total count of WDN events",
// 	})

// 	gauge = prometheus.NewGauge(prometheus.GaugeOpts{
// 		Namespace: "WDN_WNODE",
// 		Name:      "wdn_gauge_value",
// 		Help:      "Current gauge value of WDN",
// 	})

// 	histogram = prometheus.NewHistogram(prometheus.HistogramOpts{
// 		Namespace: "WDN_WNODE",
// 		Name:      "wdn_histogram_value",
// 		Help:      "Histogram of WDN measurements",
// 		Buckets:   prometheus.LinearBuckets(0, 5, 10), // 0~50까지 10버킷
// 	})
// )

// func monitoring() {
// 	const nGo = "/sched/goroutines:goroutines"
// 	const nMem = "/memory/classes/heap/objects:bytes"

// 	prometheus.MustRegister(counter)
// 	prometheus.MustRegister(gauge)
// 	prometheus.MustRegister(histogram)

// 	getMetric := make([]metrics.Sample, 2)
// 	getMetric[0].Name = nGo
// 	getMetric[1].Name = nMem

// 	go func() {
// 		for {
// 			counter.Add(rd.Float64() * 5)
// 			gauge.Add(rd.Float64()*15 - 5)
// 			histogram.Observe(rd.Float64() * 10)

// 			metrics.Read(getMetric)
// 			time.Sleep(2 * time.Second)
// 		}
// 	}()
// 	fmt.Println(http.ListenAndServe(prometheusListenPort, nil))
// }

// func main() {
// 	defaultValidatorAddr := flag.String("snode", "117.16.244.33", "Validator node IP address")
// 	kafkaProcessorAddr := flag.String("broker", "117.16.244.33", "Kafka broker IP address")
// 	libp2pAddr := flag.String("libp2p", "/ip4/117.16.244.33/tcp/4001/p2p/QmeD4iQQJAjoAvaTwT3UbFqSZ5zMWJne7dk7519H1A4ML5", "Libp2p multiaddress")
// 	channel := flag.String("channel", "mychannel", "Channel name")
// 	consensusprotocol = flag.Bool("networktype", true, "true = MP2BTP, false = QUIC")

// 	flag.Parse()

// 	validatorGrpcAddr = *defaultValidatorAddr + ":16220"
// 	validatorNatAddr = *defaultValidatorAddr + ":11730"

// 	brokers = *kafkaProcessorAddr + ":9091," + *kafkaProcessorAddr + ":9092," + *kafkaProcessorAddr + ":9093"
// 	leaderChan = *channel

// 	var consensusNode CONSENSUSNODE

// 	consensusNode = CONSENSUSNODE{
// 		leader:    false,
// 		done:      make(chan bool, 1),
// 		blockMsg:  &pb.TransactionMsg{},
// 		auditMsg:  &pb.AuditMsg{},
// 		memberMsg: &pb.MemberMsg{},
// 	}

// 	go wp2p.Start(true, *libp2pAddr, 4010)
// 	time.Sleep(3 * time.Second)

// 	consensusNode.selfId = wp2p.Host
// 	fmt.Println("[HOST ID]", consensusNode.selfId)

// 	bls.Init(bls.BLS12_381)
// 	bls.SetETHmode(bls.EthModeDraft07)
// 	sec.SetByCSPRNG()
// 	pub = sec.GetPublicKey()

// 	http.Handle("/metrics", promhttp.Handler())
// 	go monitoring()

// 	wp2p.JoinShard(leaderChan)

// 	consensusNode.start()
// }

// func (w *CONSENSUSNODE) GetAddress() string {
// 	ifaces, _ := net.Interfaces()

// 	for _, iface := range ifaces {
// 		// // 도커 컨테이너 환경일 경우
// 		// if iface.Name == "eth0" {
// 		// 로컬 환경일 경우
// 		if iface.Name == "eno1" {
// 			addrs, err := iface.Addrs()
// 			if err != nil {
// 				continue
// 			}

// 			for _, addr := range addrs {
// 				var ip net.IP
// 				switch v := addr.(type) {
// 				case *net.IPNet:
// 					ip = v.IP
// 				case *net.IPAddr:
// 					ip = v.IP
// 				}
// 				// IPv4 주소만 반환
// 				if ip != nil && ip.To4() != nil {
// 					w.address = ip.String()
// 					return w.address
// 				}
// 			}
// 		}
// 	}
// 	return ""
// }
// func (w *CONSENSUSNODE) start() {
// 	go w.setLeaderChan(false, "")
// 	go w.setDone(false)
// 	w.leader = false

// 	// [GossipSub: Tx Proposal Listening]
// 	// 1. All w-nodes receive every round of Gossip round messages from s-nodes (committee)
// 	// 2. If leader, run block listening
// 	// 3. Two message types: type-1 round msg, type-2 block msg (consensus)

// 	go w.InitKafka()
// 	go w.CommitteeListening()
// 	go w.BlockListening()
// 	go w.ConsensusListening()
// 	// pu := w.MP2BTPChildOnce()
// 	// go w.MP2BTPConsensusListening(pu)

// 	// [GRPC: Membership Message]
// 	// 1. Report network info
// 	// 2. Get WDN Members info
// 	w.Reporting()

// 	// To solve AWS NAT Problem
// 	time.Sleep(3 * time.Second)
// 	ReportExternal(w.selfId)

// 	for {
// 		time.Sleep(10 * time.Second)
// 		w.Reporting()
// 	}
// }

// func (w *CONSENSUSNODE) InitKafka() error {
// 	c := NewKafkaConsumer()
// 	err := c.SubscribeTopics([]string{"mychannel-abort"}, nil)
// 	if err != nil {
// 		return err
// 	}
// 	w.kafkaConsumer = c
// 	return nil
// }

// // kafkaListener pulls reliable blocks from the Weave Kafka partition when the peer is a leader.
// func (w *CONSENSUSNODE) KafkaListener(rekey string) {

// 	re := regexp.MustCompile(`User(\d+)`)
// 	for {
// 		select {
// 		case done := <-w.done:
// 			fmt.Println(done, "❌", w.isRunning.Load())
// 			if !w.isRunning.Load() {
// 				w.kafkaConsumer.Close()
// 				return
// 			}
// 			continue
// 		default:
// 			msg, err := w.kafkaConsumer.ReadMessage(-1)
// 			if err != nil {
// 				log.Printf("Kafka read error: %v", err)
// 				continue
// 			}

// 			_, err = w.kafkaConsumer.CommitMessage(msg)
// 			if err != nil {
// 				log.Printf("Failed to commit message: %v", err)
// 			}

// 			var Abortdata []*common.Envelope
// 			err = json.Unmarshal(msg.Value, &Abortdata)
// 			if err != nil {
// 				log.Printf("Failed to unmarshal Kafka message: %v", err)
// 				continue
// 			}

// 			userIDs := []int{}
// 			for _, env := range Abortdata {
// 				payload, err := protoutil.UnmarshalTransaction(env.Payload)
// 				if err != nil {
// 					log.Printf("Failed to unmarshal transaction payload: %v", err)
// 					continue
// 				}

// 				match := re.FindStringSubmatch(payload.String())
// 				if len(match) > 1 {
// 					idx := idxToInt(match[1])
// 					userIDs = append(userIDs, idx)
// 				}
// 			}

// 			userCount := make(map[string]int)
// 			for _, id := range userIDs {
// 				userKey := "User" + strconv.Itoa(id)
// 				userCount[userKey]++
// 			}

// 			fmt.Println("📝[NEW CONSENSUS DATA]", userCount)

// 			w.auditMsg = &pb.AuditMsg{}
// 			w.bftConsensus(userCount)
// 		}
// 	}
// }

// func idxToInt(s string) int {
// 	var num int
// 	fmt.Sscanf(s, "%d", &num)
// 	return num
// }

// func (w *CONSENSUSNODE) bftConsensus(userCount map[string]int) {
// 	setConsensusStart(w.selfId)
// 	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	w.auditMsg = &pb.AuditMsg{
// 		BlkNum:              w.roundIdx,
// 		LeaderID:            w.selfId,
// 		PhaseNum:            pb.AuditMsg_PREPARE,
// 		MerkleRootHash:      "abcdefghijklmnopqrstuvwxyz",
// 		AbortTransactionNum: userCount,
// 		HonestAuditors:      []string{w.selfId},
// 	}

// 	if w.auditMsg.BlkNum == 1 {
// 		w.auditMsg.CurHash = "ajknadajsnamajkndqwaakdmkaiwq"
// 	} else {
// 		hashBytes := lg.BlockHashCalculator(w.auditMsg)
// 		hash := sha512.Sum512(hashBytes)
// 		w.auditMsg.CurHash = hex.EncodeToString(hash[:63])
// 	}

// 	signing := sec.SignByte([]byte(w.auditMsg.CurHash))
// 	w.auditMsg.Signature = signing.Serialize()

// 	fmt.Println("🚀 Broadcasting PREPARE")
// 	chPrepare := make(chan bool, len(w.members))
// 	var addrs []string
// 	if *consensusprotocol {
// 		//==== ✅ 통합 MP2BTP+WDN 방식 ====
// 		for _, each := range w.members {
// 			addrs = append(addrs, each.Addr)
// 		}
// 		MP2BTPsession = w.MP2BTPRootOnce(chPrepare, addrs)
// 		go w.MP2BTPSubmit(chPrepare, MP2BTPsession, pb.AuditMsg_PREPARE)
// 	} else {
// 		// ==== ✅ 기존 WDN 방식 ====
// 		for _, each := range w.members {
// 			go w.Submit(chPrepare, each.Addr, pb.AuditMsg_PREPARE)
// 		}
// 	}

// 	if waitForVotes(chPrepare, len(w.members)) {
// 		fmt.Println("✅ PREPARE Quorum achieved")
// 	} else {
// 		fmt.Println("⚠️ PREPARE Quorum not achieved")
// 	}
// 	// <-ctx.Done()

// 	fmt.Println("🚀 Broadcasting COMMIT")
// 	chCommit := make(chan bool, len(w.members))
// 	if *consensusprotocol {
// 		//==== ✅ 통합 MP2BTP+WDN 방식 ====
// 		go w.MP2BTPSubmit(chCommit, MP2BTPsession, pb.AuditMsg_COMMIT)
// 	} else {
// 		// ==== ✅ 기존 WDN 방식 ====
// 		for _, each := range w.members {
// 			go w.Submit(chCommit, each.Addr, pb.AuditMsg_COMMIT)
// 		}
// 	}

// 	if waitForVotes(chCommit, len(w.members)) {
// 		fmt.Println("✅ COMMIT Quorum achieved")
// 	} else {
// 		fmt.Println("⚠️ COMMIT Quorum not achieved")
// 	}
// 	// <-ctx.Done()

// 	fmt.Println("📦 Disseminating block to peers")
// 	gossipMsg := &pb.GossipMsg{
// 		Type:   2,
// 		Rndblk: w.auditMsg,
// 	}
// 	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossipMsg)
// 	setConsensusDone(w.selfId)
// }

// func setConsensusStart(id string) {
// 	consensusStatusMap[id] = false
// }

// func setConsensusDone(id string) {
// 	consensusStatusMap[id] = true
// }

// func waitForVotes(ch chan bool, total int) bool {
// 	votes := 0
// 	quorum := 1 // TODO: replace with 2f+1 logic if needed

// 	for {
// 		select {
// 		case <-ch:
// 			votes++
// 			fmt.Printf("🗳️ Votes: %d/%d\n", votes, total)
// 			if votes >= quorum {
// 				return true
// 			}
// 		default:
// 			time.Sleep(3 * time.Second)
// 		}
// 	}
// }

// func (w *CONSENSUSNODE) setLeaderChan(isLeader bool, channel string) {
// 	if w.channelID != channel {
// 		w.channelID = channel
// 	}
// 	w.isLeader.Store(isLeader)
// }

// func (w *CONSENSUSNODE) setDone(b bool) {
// 	w.done <- b
// }

// // Check if this peer is the leader; if so, it should receive messages from Kafka.
// // BlockListening listens for block insertion.
// func (w *CONSENSUSNODE) BlockListening() {
// 	w.GetAddress()
// 	listener, err := net.Listen("tcp", w.address+grpcPort)
// 	if err != nil {
// 		log.Fatalf("❌ Failed to listen on %s: %v", w.address+grpcPort, err)
// 	}
// 	defer listener.Close()

// 	for {
// 		conn, err := listener.Accept()
// 		if err != nil {
// 			log.Println("⚠️ Accept error:", err)
// 			continue
// 		}
// 		go w.BloConnHandler(conn)
// 	}
// }

// func (w *CONSENSUSNODE) BloConnHandler(conn net.Conn) {
// 	defer conn.Close()

// 	recvBuf := make([]byte, 8192)
// 	n, err := conn.Read(recvBuf)
// 	if err != nil {
// 		if err == io.EOF {
// 			log.Printf("🔌 Connection closed by client: %v", conn.RemoteAddr())
// 		} else {
// 			log.Printf("❌ Failed to read data: %v", err)
// 		}
// 		return
// 	}

// 	MsgRecv := &pb.GossipMsg{}
// 	if err := json.Unmarshal(recvBuf[:n], MsgRecv); err != nil {
// 		log.Printf("❌ Failed to unmarshal GossipMsg: %v", err)
// 		return
// 	}

// 	fmt.Println("📦 BLOCKINSERT: Committing block to ledger")
// 	go lg.UserAbortInfoInsert(MsgRecv.Rndblk)

// }

// // Check if this peer is the leader; if so, it should receive messages from Kafka.
// // CommitteeListening listens for committe information.
// func (w *CONSENSUSNODE) CommitteeListening() {
// 	w.GetAddress()
// 	listener, err := net.Listen("tcp", w.address+gossipListenPort)
// 	if err != nil {
// 		log.Fatalf("Failed to listen on %s: %v", w.address+gossipListenPort, err)
// 	}
// 	defer listener.Close()

// 	for {
// 		conn, err := listener.Accept()
// 		if err != nil {
// 			log.Println("⚠️ Accept error:", err)
// 			continue
// 		}
// 		go w.CommConnHandler(conn)
// 	}
// }

// func (w *CONSENSUSNODE) CommConnHandler(conn net.Conn) {
// 	defer conn.Close()

// 	recvBuf := make([]byte, 8192)
// 	n, err := conn.Read(recvBuf)
// 	if err != nil {
// 		if err == io.EOF {
// 			log.Printf("🔌 Connection closed by client: %v", conn.RemoteAddr())
// 		} else {
// 			log.Printf("❌ Failed to read data: %v", err)
// 		}
// 		return
// 	}

// 	recvMsg := &pb.CommitteeMsg{}
// 	if err := json.Unmarshal(recvBuf[:n], recvMsg); err != nil {
// 		log.Printf("❌ Failed to unmarshal CommitteeMsg: %v", err)
// 		return
// 	}

// 	if w.isRunning.Load() {
// 		log.Printf("⚠️ Still processing previous round, skipping new CommitteeMsg (round %d)", recvMsg.RoundNum)
// 		return
// 	}

// 	w.done = make(chan bool, 1)
// 	w.roundIdx = recvMsg.RoundNum

// 	for _, shard := range recvMsg.Shards {
// 		for _, mem := range shard.Member {
// 			if mem.NodeID == w.selfId {
// 				w.members = shard.Member

// 				isLeader := (recvMsg.GetType() == 1 && shard.LeaderID == w.selfId)

// 				w.leader = isLeader
// 				w.channelID = leaderChan // use fixed ID if that is intended

// 				if isLeader {
// 					w.isRunning.Store(true)

// 					go func() {
// 						defer func() {
// 							if r := recover(); r != nil {
// 								fmt.Printf("💥 KafkaListener 내부 goroutine에서 panic 발생: %v\n", r)
// 								debug.PrintStack() // runtime/debug import 필요
// 							}
// 						}()
// 						defer w.setDone(false)
// 						defer w.isRunning.Store(false)
// 						w.KafkaListener(w.channelID + "-abort")
// 					}()
// 				} else {
// 					fmt.Println("🧩 Follower consensus node")
// 				}
// 				return
// 			}
// 		}
// 	}
// }

// /*I'm a leader, send a message to a w-node*/
// func (w *CONSENSUSNODE) Submit(ch chan<- bool, ip string, targetPhase pb.AuditMsg_Phases) {
// 	tlsConf := &tls.Config{
// 		InsecureSkipVerify: true,
// 		NextProtos:         []string{"quic-echo-example"},
// 	}
// 	session, err := quic.DialAddr(context.Background(), ip+gossipMsgPort, tlsConf, nil)
// 	if err != nil {
// 		log.Printf("❌ Failed to connect to %s: %v", ip, err)
// 		ch <- false
// 		return
// 	}
// 	defer session.CloseWithError(0, "")

// 	stream, err := session.OpenStreamSync(context.Background())
// 	if err != nil {
// 		log.Printf("❌ Failed to open stream: %v", err)
// 		ch <- false
// 		return
// 	}

// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		log.Printf("❌ Marshal error: %v", err)
// 		ch <- false
// 		return
// 	}
// 	_, err = stream.Write(sndBuf)
// 	if err != nil {
// 		log.Printf("❌ Write error: %v", err)
// 		ch <- false
// 		return
// 	}

// 	for {
// 		rcvBuf := make([]byte, 8192)
// 		n, err := stream.Read(rcvBuf)
// 		if err != nil {
// 			ch <- false
// 			break
// 		}
// 		if n == 0 {
// 			continue
// 		}

// 		cMsgRecv := &pb.AuditMsg{}
// 		if err := json.Unmarshal(rcvBuf[:n], cMsgRecv); err != nil {
// 			log.Println("❌ JSON unmarshal error:", err)
// 			continue
// 		}

// 		if cMsgRecv.PhaseNum != targetPhase {
// 			continue
// 		}

// 		sig := bls.Sign{}
// 		if err := sig.Deserialize(cMsgRecv.Signature); err != nil {
// 			log.Println("❌ Signature deserialize error:", err)
// 			continue
// 		}
// 		sigVec = append(sigVec, sig)
// 		if len(sigVec) == len(w.memberMsg.Nodes) {
// 			fmt.Printf("✅ Phase %v Aggregation Complete", targetPhase)
// 			w.Multisinging(cMsgRecv, sigVec)
// 			sigVec = sigVec[:0]
// 			if targetPhase == pb.AuditMsg_AGGREGATED_PREPARE {
// 				w.auditMsg.HonestAuditors = cMsgRecv.HonestAuditors
// 			}
// 			break
// 		} else {
// 			fmt.Printf("⏳ Waiting for more signatures: %d/%d", len(sigVec), len(w.memberMsg.Nodes))
// 			continue
// 		}
// 	}

// 	stream.Close()
// 	ch <- true
// }

// /*Consensus three-phases: Announce(Completed State), Prepare, Commit] I'm not a leader*/
// func (w *CONSENSUSNODE) ConsensusListening() {
// 	w.GetAddress()
// 	listener, err := quic.ListenAddr(w.address+gossipMsgPort, generateTLSConfig(), nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	for {
// 		sess, err := listener.Accept(context.Background())
// 		if err != nil {
// 			panic(err)
// 		}
// 		stream, err := sess.AcceptStream(context.Background())
// 		if err != nil {
// 			panic(err)
// 		}
// 		go w.StreamHandler(stream)
// 	}
// }

// func (w *CONSENSUSNODE) StreamHandler(stream quic.Stream) {

// 	for {
// 		rcvBuf := make([]byte, 8192)
// 		n, err := stream.Read(rcvBuf)
// 		if err != nil {
// 			break
// 		}

// 		if n == 0 {
// 			continue
// 		}

// 		rcvBuf = rcvBuf[:n]
// 		cMsgRecv := &pb.AuditMsg{}
// 		if err = json.Unmarshal(rcvBuf, cMsgRecv); err != nil {
// 			fmt.Println(err)
// 			continue
// 		}

// 		switch cMsgRecv.PhaseNum {
// 		case pb.AuditMsg_PREPARE:
// 			w.preparePhase(cMsgRecv)
// 			w.sendResponse(stream)
// 			return
// 		case pb.AuditMsg_COMMIT:
// 			if w.verifying(cMsgRecv) {
// 				w.commitPhase(cMsgRecv)
// 				w.sendResponse(stream)
// 				return
// 			}
// 		default:
// 			continue
// 		}
// 	}
// }

// func (w *CONSENSUSNODE) preparePhase(cMsg *pb.AuditMsg) {
// 	hash := []byte(cMsg.CurHash)
// 	signing := sec.SignByte(hash)
// 	w.auditMsg.Signature = signing.Serialize()
// 	w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE
// 	w.updateAuditFields(cMsg)
// }

// func (w *CONSENSUSNODE) commitPhase(cMsg *pb.AuditMsg) {
// 	hash := []byte(cMsg.CurHash)
// 	signing := sec.SignByte(hash)
// 	w.auditMsg.Signature = signing.Serialize()
// 	w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
// 	w.updateAuditFields(cMsg)
// }

// func (w *CONSENSUSNODE) sendResponse(stream quic.Stream) {
// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	if _, err = stream.Write(sndBuf); err != nil {
// 		panic(err)
// 	}
// }
// func (w *CONSENSUSNODE) sendMP2BTPResponse(pu *PuCtrl.PuCtrl) {
// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	pu.SendAuditAckMsg(sndBuf)
// }

// func (w *CONSENSUSNODE) updateAuditFields(cMsg *pb.AuditMsg) {
// 	w.auditMsg.CurHash = cMsg.CurHash
// 	w.auditMsg.MerkleRootHash = cMsg.MerkleRootHash

// 	alreadyExists := false
// 	for _, id := range cMsg.HonestAuditors {
// 		if id == w.selfId {
// 			alreadyExists = true
// 			break
// 		}
// 	}
// 	if !alreadyExists {
// 		w.auditMsg.HonestAuditors = append(cMsg.HonestAuditors, w.selfId)
// 	} else {
// 		w.auditMsg.HonestAuditors = cMsg.HonestAuditors
// 	}
// }

// func (w *CONSENSUSNODE) verifying(cMsg *pb.AuditMsg) bool {
// 	pubVec = pubVec[:0]

// 	for _, honestID := range cMsg.HonestAuditors {
// 		if pk := w.getPublicKeyByNodeID(honestID); pk != nil {
// 			pubVec = append(pubVec, *pk)
// 		} else {
// 			fmt.Printf("Missing public key for node: %s\n", honestID)
// 		}
// 	}

// 	fmt.Printf("👑 Phase: %v\n👑 Public Keys Count: %d\n👑 Public Keys: %v\n", cMsg.PhaseNum, len(pubVec), pubVec)

// 	var decSign bls.Sign
// 	if err := decSign.Deserialize(cMsg.Signature); err != nil {
// 		fmt.Println("Signature deserialization error:", err)
// 		return false
// 	}

// 	switch cMsg.PhaseNum {
// 	case pb.AuditMsg_COMMIT:
// 		if decSign.FastAggregateVerify(pubVec, []byte(cMsg.CurHash)) {
// 			fmt.Println("✅ AGGREGATED_COMMIT: Verification SUCCESS")
// 			return true
// 		}
// 		fmt.Println("❌ AGGREGATED_COMMIT: Verification ERROR")
// 	}
// 	return false
// }

// func (w *CONSENSUSNODE) getPublicKeyByNodeID(id string) *bls.PublicKey {
// 	for _, node := range w.memberMsg.Nodes {
// 		if node.NodeID == id {
// 			dec := &bls.PublicKey{}
// 			if err := dec.Deserialize(node.Publickey); err != nil {
// 				fmt.Printf("Failed to deserialize public key for node %s: %v\n", id, err)
// 				return nil
// 			}
// 			return dec
// 		}
// 	}
// 	return nil
// }
// func (w *CONSENSUSNODE) Multisinging(cMsgs *pb.AuditMsg, sigVec []bls.Sign) (bool, int) {
// 	switch cMsgs.PhaseNum {
// 	case pb.AuditMsg_AGGREGATED_PREPARE:
// 		return w.aggregateAndPrepare(cMsgs, sigVec), len(w.auditMsg.HonestAuditors)

// 	case pb.AuditMsg_AGGREGATED_COMMIT:
// 		return w.aggregateAndCommit(cMsgs, sigVec), len(w.auditMsg.HonestAuditors)

// 	default:
// 		return false, 0
// 	}
// }

// func (w *CONSENSUSNODE) aggregateAndPrepare(cMsgs *pb.AuditMsg, sigVec []bls.Sign) bool {
// 	return w.aggregateAndSet(cMsgs, sigVec, pb.AuditMsg_COMMIT)
// }

// func (w *CONSENSUSNODE) aggregateAndCommit(cMsgs *pb.AuditMsg, sigVec []bls.Sign) bool {
// 	return w.aggregateAndSet(cMsgs, sigVec, pb.AuditMsg_AGGREGATED_COMMIT)
// }

// func (w *CONSENSUSNODE) aggregateAndSet(cMsgs *pb.AuditMsg, sigVec []bls.Sign, phase pb.AuditMsg_Phases) bool {
// 	var aggeSign bls.Sign
// 	aggeSign.Aggregate(sigVec)
// 	byteSig := aggeSign.Serialize()

// 	w.auditMsg.BlkNum = cMsgs.BlkNum
// 	w.auditMsg.PrevHash = cMsgs.PrevHash
// 	w.auditMsg.CurHash = cMsgs.CurHash
// 	w.auditMsg.Signature = byteSig
// 	w.auditMsg.PhaseNum = phase
// 	return true
// }

// /*GRPC SERVICE Membershhip Exchanging Procedure*/
// func (w *CONSENSUSNODE) Reporting() {
// 	conn, err := grpc.Dial(validatorGrpcAddr, grpc.WithInsecure(), grpc.WithBlock())
// 	if err != nil {
// 		log.Fatalf("Failed to connect to validator gRPC server: %v", err)
// 	}
// 	defer conn.Close()

// 	c := pb.NewMembershipServiceClient(conn)
// 	req := createSelfMembership(w.selfId, w.address, pub.Serialize(), "11730", w.channelID)

// 	w.memberMsg, err = c.GetMembership(context.Background(), req)
// 	if err != nil {
// 		log.Fatalf("Failed to get membership: %v", err)
// 	}
// }

// func getConsensusStatus(id string) bool {
// 	return !consensusStatusMap[id]
// }

// func createSelfMembership(id string, addr string, pubKey []byte, port string, channelid string) *pb.MemberMsg {
// 	node := &pb.Node{
// 		NodeID:          id,
// 		Addr:            addr,
// 		Port:            port,
// 		Publickey:       pubKey,
// 		Consensusstatus: getConsensusStatus(id),
// 		Channel:         channelid,
// 	}
// 	fmt.Println("👑 NODE", node, "👑")
// 	return &pb.MemberMsg{Nodes: []*pb.Node{node}}
// }

// func MerkleHash(s []uint64) string {
// 	return strconv.FormatUint(s[0], 16)
// }

// func ReportExternal(id string) {
// 	conn, err := net.Dial("tcp", validatorNatAddr)
// 	if err != nil {
// 		fmt.Println("[NAT] Failed to dial:", err)
// 		return
// 	}
// 	defer conn.Close()

// 	sndBuf, err := json.Marshal(id)
// 	if err != nil {
// 		fmt.Println("[NAT] Failed to marshal ID:", err)
// 		return
// 	}

// 	if _, err := conn.Write(sndBuf); err != nil {
// 		fmt.Println("[NAT] Failed to write data:", err)
// 	}
// }

// func generateTLSConfig() *tls.Config {
// 	key, err := rsa.GenerateKey(rand.Reader, 1024)
// 	if err != nil {
// 		panic(fmt.Errorf("failed to generate RSA key: %w", err))
// 	}

// 	template := x509.Certificate{SerialNumber: big.NewInt(1)}
// 	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
// 	if err != nil {
// 		panic(fmt.Errorf("failed to create certificate: %w", err))
// 	}

// 	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
// 	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

// 	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
// 	if err != nil {
// 		panic(fmt.Errorf("failed to load TLS key pair: %w", err))
// 	}

// 	return &tls.Config{
// 		Certificates: []tls.Certificate{tlsCert},
// 		NextProtos:   []string{"quic-echo-example"},
// 	}
// }

// func WriteTomlFile(fileName string, numMultipath int, myAddrs []string, childAddrs []string) error {
// 	_ = os.Remove(fileName)
// 	f, err := os.Create(fileName)
// 	if err != nil {
// 		return err
// 	}
// 	defer f.Close()

// 	var sb strings.Builder

// 	sb.WriteString("# MP2BTP Configurations\n")
// 	sb.WriteString(fmt.Sprintf("VERBOSE_MODE = true\n"))
// 	sb.WriteString(fmt.Sprintf("NUM_MULTIPATH = %d\n", numMultipath))

// 	formattedIPs := make([]string, len(myAddrs))
// 	fmt.Println(myAddrs)
// 	for i, ip := range myAddrs {
// 		formattedIPs[i] = fmt.Sprintf("\"%s\"", ip)
// 	}
// 	sb.WriteString(fmt.Sprintf("MY_IP_ADDRS = [%s]\n", strings.Join(formattedIPs, ", ")))

// 	sb.WriteString(baseleaderTomlConfig)

// 	sb.WriteString(fmt.Sprintf(
// 		"\n[[peer_addr]]  # Offset=0\nAddr = \"%s\"\nNumChild = %d\nChildOffset = 1\n",
// 		myAddrs[0], len(childAddrs),
// 	))

// 	for i, addr := range childAddrs {
// 		sb.WriteString(fmt.Sprintf(
// 			"\n[[peer_addr]]  # Offset=%d\nAddr = \"%s\"\nNumChild = 0\nChildOffset = 0\n",
// 			i+1, addr,
// 		))
// 	}

// 	_, err = f.WriteString(sb.String())
// 	return err
// }

// func (w *CONSENSUSNODE) MP2BTPRootOnce(ch chan<- bool, addrs []string) *PuCtrl.PuCtrl {
// 	mu.Lock()
// 	defer mu.Unlock()
// 	w.GetAddress()

// 	peerId := rd.Intn(10) + 1

// 	configFile := "./push_root.toml"

// 	myaddrs := make([]string, 1)
// 	myaddrs[0] = w.address

// 	err := WriteTomlFile(configFile, 1, myaddrs, addrs)
// 	if err != nil {
// 		fmt.Println("❌ TOML 파일 생성 실패:", err)
// 		ch <- false
// 		return nil
// 	}

// 	fmt.Println("🗂️ TOML 파일 생성 완료:", configFile)

// 	var config PuCtrl.Config
// 	if _, err := toml.DecodeFile(configFile, &config); err != nil {
// 		panic(err)
// 	}

// 	pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

// 	nodeInfo := pu.GetNodeInfo(config.PEER_ADDRS)

// 	pu.Listen()

// 	pu.Connect(nodeInfo, byte(PuCtrl.PUSH_SESSION))

// 	return pu
// }

// func (w *CONSENSUSNODE) MP2BTPSubmit(ch chan<- bool, pu *PuCtrl.PuCtrl, targetPhase pb.AuditMsg_Phases) {

// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		fmt.Println("❌ JSON 직렬화 실패:", err)
// 		ch <- false
// 		return
// 	}
// 	pu.SendAuditMsg(sndBuf)

// 	go func() {
// 		for {
// 			select {
// 			case <-ctx.Done():
// 				fmt.Println("🛑 leader종료 신호 수신, 고루틴 종료")
// 				return
// 			case msg := <-pu.AuditMsgAckChan:
// 				fmt.Println("🌟 child에서 받은 auditMsg:", msg)

// 				auditMsg := &pb.AuditMsg{}
// 				if err := json.Unmarshal(msg.Data, &auditMsg); err != nil {
// 					fmt.Println(err)
// 					continue
// 				}
// 				fmt.Println("🌟🌟🌟🌟", auditMsg.PhaseNum)
// 				ch <- true

// 				sig := bls.Sign{}
// 				if err := sig.Deserialize(auditMsg.Signature); err != nil {
// 					log.Println("❌ Signature deserialize error:", err)
// 					continue
// 				}
// 				sigVec = append(sigVec, sig)
// 				if len(sigVec) == len(w.memberMsg.Nodes) {
// 					log.Printf("✅ Phase %v Aggregation Complete", targetPhase)
// 					w.Multisinging(auditMsg, sigVec)
// 					sigVec = sigVec[:0]
// 					if targetPhase == pb.AuditMsg_AGGREGATED_PREPARE {
// 						w.auditMsg.HonestAuditors = auditMsg.HonestAuditors
// 					}
// 					return
// 				} else {
// 					log.Printf("⏳ Waiting for more signatures: %d/%d", len(sigVec), len(w.memberMsg.Nodes))
// 					continue
// 				}

// 			}
// 			// <-ctx.Done()
// 		}

// 	}()
// }

// func (w *CONSENSUSNODE) MP2BTPChildOnce() *PuCtrl.PuCtrl {
// 	fmt.Println("😬 Starting MP2BTP listening")

// 	configFile := "./push_child.toml"
// 	peerId := rd.Intn(10) + 1

// 	if _, err := toml.DecodeFile(configFile, new(PuCtrl.Config)); err != nil {
// 		fmt.Println("❌ Config 파일 파싱 실패:", err)
// 		return nil
// 	}

// 	pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

// 	pu.Listen()

// 	return pu
// }

// func (w *CONSENSUSNODE) MP2BTPConsensusListening(pu *PuCtrl.PuCtrl) {
// 	startTime := time.Now()
// 	recvBytes := 0
// 	var msg *packet.AuditDataPacket

// 	auditMsg := &pb.AuditMsg{}
// 	// auditChan := pu.AuditBroadcaster.Register()

// 	go func() {
// 		for {
// 			// msg = <-auditChan

// 			auditMsg = &pb.AuditMsg{}
// 			if err := json.Unmarshal(msg.Data, &auditMsg); err != nil {
// 				fmt.Println(err)
// 				continue
// 			}

// 			elapsedTime := time.Since(startTime)
// 			throughput := (float64(recvBytes) * 8.0) / elapsedTime.Seconds() / (1000 * 1000)
// 			fmt.Println("Seconds=", elapsedTime.Seconds())
// 			fmt.Println("Throughput=", throughput)
// 			fmt.Println("ReceivedSize=", recvBytes)

// 			switch auditMsg.PhaseNum {
// 			case pb.AuditMsg_PREPARE:
// 				fmt.Println("111111111111111111111111111")
// 				w.preparePhase(auditMsg)
// 				w.sendMP2BTPResponse(pu)
// 			case pb.AuditMsg_COMMIT:
// 				if w.verifying(auditMsg) {
// 					fmt.Println("22222222222222222222")
// 					w.commitPhase(auditMsg)
// 					w.sendMP2BTPResponse(pu)
// 				}
// 			default:
// 				// 필요에 따라 다른 PhaseNum 처리 또는 무시
// 			}
// 		}
// 	}()
// }
