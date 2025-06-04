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
// 	"path"
// 	"regexp"
// 	"runtime/metrics"
// 	"strconv"
// 	"sync"
// 	"time"

// 	"github.com/BurntSushi/toml"
// 	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
// 	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
// 	"github.com/golang/protobuf/proto"
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

// type CONSENSUSNODE struct {
// 	wlog *log.Logger

// 	address   string
// 	selfId    string
// 	roundIdx  uint32
// 	members   []*pb.Wnode
// 	isLeader  chan bool
// 	leader    bool
// 	channelID string
// 	done      chan bool

// 	blockMsg  *pb.TransactionMsg
// 	memberMsg *pb.MemberMsg
// 	auditMsg  *pb.AuditMsg
// }

// var (
// 	validatorGrpcAddr = "117.16.244.33:16220" // [S-Node] default IP : Port for gRPC
// 	validatorNatAddr  = "117.16.244.33:11730" // [S-Node] default IP : Port for AWS NAT

// 	ConCPORT = ":5252"  // [W-Node] Port for consensus listening
// 	ConGPORT = ":4242"  // [W-Node] Port for gossip Msg Listening
// 	ConLPORT = ":6262"  // [W-Node] Port for gossip Msg Listening
// 	ConPPORT = ":12345" // [W-Node] Port for prometheus Listening

// 	brokers    = ""
// 	leaderBro  = false
// 	leaderChan = ""
// )
// var MP2BTPsession *PuCtrl.PuCtrl

// func Kafka() *kafka.Consumer {
// 	c, err := kafka.NewConsumer(&kafka.ConfigMap{
// 		"bootstrap.servers": brokers,
// 		"group.id":          "myGroup",
// 		"auto.offset.reset": "earliest",
// 	})

// 	if err != nil {
// 		// When a connection error occurs, a panic occurs and the system is shut down
// 		panic(err)
// 	}

// 	return c
// }

// // My private and public key
// var sec bls.SecretKey
// var pub *bls.PublicKey

// // All nodes' private & public key
// var sigVec []bls.Sign
// var pubVec []bls.PublicKey

// // METRICS for PROMETHEUS MONITORING
// var counter = prometheus.NewCounter(
// 	prometheus.CounterOpts{
// 		Namespace: "WDN_WNODE",
// 		Name:      "WDN_counter",
// 		Help:      "This is WDN counter",
// 	})

// var gauge = prometheus.NewGauge(
// 	prometheus.GaugeOpts{
// 		Namespace: "WDN_WNODE",
// 		Name:      "WDN_gauge",
// 		Help:      "This is WDN gauge",
// 	})

// var histogram = prometheus.NewHistogram(
// 	prometheus.HistogramOpts{
// 		Namespace: "WDN_WNODE",
// 		Name:      "WDN_histogram",
// 		Help:      "This is WDN histogram",
// 	})

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
// 		// export the value want
// 		for {
// 			counter.Add(rd.Float64() * 5)
// 			gauge.Add(rd.Float64()*15 - 5)
// 			histogram.Observe(rd.Float64() * 10)

// 			metrics.Read(getMetric)
// 			time.Sleep(2 * time.Second)
// 		}
// 	}()
// 	fmt.Println(http.ListenAndServe(ConPPORT, nil))
// }

// func main() {
// 	// 도커 컨테이너화를 진행했을떄, 포트나 ip가 제대로 동작하도록 설정하고 도커 컨테이너화 진행할 수 있도록.
// 	defaultvalidatorAddr := flag.String("snode", "117.16.244.33", "")
// 	kafkaprocessorAddr := flag.String("broker", "117.16.244.33", "")
// 	libp2pAddr := flag.String("libp2p", "/ip4/117.16.244.33/tcp/4001/p2p/QmeD4iQQJAjoAvaTwT3UbFqSZ5zMWJne7dk7519H1A4ML5", "")
// 	ami := flag.Bool("ami", false, "")
// 	chann := flag.String("channel", "mychannel", "")

// 	flag.Parse()

// 	validatorGrpcAddr = *defaultvalidatorAddr + ":16220"
// 	validatorNatAddr = *defaultvalidatorAddr + ":11730"

// 	brokers = *kafkaprocessorAddr + ":9091, " + *kafkaprocessorAddr + ":9092, " + *kafkaprocessorAddr + ":9093"
// 	leaderBro = *ami
// 	leaderChan = *chann

// 	var consensusNode CONSENSUSNODE

// 	LOGFILE := path.Join("./wnode.log")
// 	f, err := os.OpenFile(LOGFILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	if err != nil {
// 		fmt.Println(err)
// 		return
// 	}
// 	defer f.Close()
// 	consensusNode = CONSENSUSNODE{
// 		wlog:      log.New(f, "WLog ", log.LstdFlags),
// 		isLeader:  make(chan bool),
// 		leader:    false,
// 		done:      make(chan bool),
// 		blockMsg:  &pb.TransactionMsg{},
// 		auditMsg:  &pb.AuditMsg{},
// 		memberMsg: &pb.MemberMsg{},
// 	}
// 	consensusNode.wlog.Println("Start")

// 	// GossipSub Channel Join within 3 second
// 	go wp2p.Start(true, *libp2pAddr, 4010)
// 	time.Sleep(3 * time.Second)
// 	// WDG ID is calculated by hashing IP address.
// 	consensusNode.selfId = wp2p.Host
// 	fmt.Println("[HOST ID]", consensusNode.selfId)

// 	/*  Initialize BLS multi-signature
// 	    Thus, w-node get his  secret key and public key
// 	*/
// 	bls.Init(bls.BLS12_381)
// 	bls.SetETHmode(bls.EthModeDraft07)
// 	sec.SetByCSPRNG()
// 	pub = sec.GetPublicKey()

// 	//  Export PROMETHEUS Metric for health check
// 	http.Handle("/metrics", promhttp.Handler())
// 	go monitoring()

// 	myChannel := leaderChan
// 	wp2p.JoinShard(myChannel)

// 	//  Let's start to communicate with s-node
// 	consensusNode.start()
// }

// func (w *CONSENSUSNODE) GetAddress() string {
// 	ifaces, _ := net.Interfaces()
// 	for _, i := range ifaces {
// 		// //DOCKER
// 		// if i.Name == "eth0" {
// 		//LOCAL
// 		if i.Name == "eno1" {
// 			// if i.Name == "enp2s0" {
// 			addrs, _ := i.Addrs()
// 			for _, addr := range addrs {
// 				var ip net.IP
// 				switch v := addr.(type) {
// 				case *net.IPNet:
// 					ip = v.IP
// 				case *net.IPAddr:
// 					ip = v.IP
// 				}
// 				w.address = ip.String()
// 				return w.address
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
// 	// 1. All w-nodes receive every round of Gossip round messages from s-nodes 이게 committee
// 	// 그 중에서 leader라면 blocklistening
// 	// 2. All w-nodes receive every round of Gossip block messages from the leader 이게 consemsus.
// 	// 3. In other words, there are two message types (type-1) round msg (type-2) Block Msg

// 	go w.CommitteeListening()

// 	go w.BlockListening()

// 	go w.ConsensusListening()

// 	// [GRPC: Membership Message]
// 	//	1. Report his network information
// 	//	2. Get the information WDN Members
// 	w.Reporting()

// 	// To solve AWS NAT Problem (외부 IP 주소를 알려주기 위해)
// 	time.Sleep(time.Second * 3)
// 	ReportExternal(w.selfId)
// 	for {
// 		time.Sleep(time.Second * 10)
// 		w.Reporting()
// 	}
// }

// func idxToInt(s string) int {
// 	var num int
// 	fmt.Sscanf(s, "%d", &num)
// 	return num
// }

// // kafkaListener pulls reliable blocks from the Weave Kafka partition when
// // the peer is a leader.
// func (w *CONSENSUSNODE) KafkaListener(rekey string) {

// 	c := Kafka()
// 	c.SubscribeTopics([]string{rekey}, nil)

// 	var userIDs []int

// 	for {
// 		select {
// 		case done := <-w.done:
// 			if done {
// 				c.Close()
// 				return
// 			}
// 		default:
// 			msg, err := c.ReadMessage(-1)
// 			if err == nil {
// 				var index = 0
// 				var Abortdata []*common.Envelope
// 				err := json.Unmarshal(msg.Value, &Abortdata)
// 				if err != nil {
// 					fmt.Println(err)
// 				}
// 				for i := range Abortdata {
// 					payload, _ := protoutil.UnmarshalTransaction(Abortdata[i].Payload)
// 					re := regexp.MustCompile(`User(\d+)`)
// 					match := re.FindStringSubmatch(payload.String())
// 					if len(match) > 1 {
// 						idx := match[1]
// 						index = idxToInt(idx)
// 						userIDs = append(userIDs, index)
// 					}
// 				}

// 				userCount := make(map[string]int)

// 				for _, id := range userIDs {
// 					userKey := "User" + strconv.Itoa(id)
// 					userCount[userKey]++
// 				}

// 				fmt.Println("User Count Map:", userCount)
// 				w.auditMsg = &pb.AuditMsg{}
// 				go w.bftConsensus(userCount)
// 				// go w.MP2BTPbftConsensus(userCount)
// 			}
// 		}
// 	}
// }

// func (w *CONSENSUSNODE) bftConsensus(userCount map[string]int) {
// 	// Consensus Timer Setup
// 	ctx := context.Background()
// 	ctx, cancel := context.WithTimeout(ctx, time.Duration(10)*time.Second)
// 	defer cancel()

// 	// Make a consensus message
// 	w.auditMsg.BlkNum = w.roundIdx
// 	w.auditMsg.LeaderID = w.selfId
// 	w.auditMsg.PhaseNum = pb.AuditMsg_PREPARE
// 	w.auditMsg.MerkleRootHash = "abcdefghijklmnopqrstuvwxyz"
// 	w.auditMsg.AbortTransactionNum = userCount

// 	// Calculate Block Hash Value, Current Hash Value is determined by the following 6 field values
// 	// BlkNum, LeaderID, PhaseNum, MerkleRootHash, TxListm, PrevHash
// 	if w.auditMsg.BlkNum == 1 {
// 		w.auditMsg.CurHash = "ajknadajsnamajkndqwaakdmkaiwq"
// 	} else {
// 		b := lg.BlockHashCalculator(w.auditMsg)
// 		h := sha512.Sum512(b)
// 		w.auditMsg.CurHash = hex.EncodeToString(h[:63])
// 	}

// 	// Signing
// 	hash := []byte(w.auditMsg.CurHash)
// 	signing := sec.SignByte(hash)
// 	byteSig := signing.Serialize()

// 	w.auditMsg.Signature = byteSig

// 	//  Who signed
// 	w.auditMsg.HonestAuditors = append(w.auditMsg.HonestAuditors, w.selfId)

// 	// [Send the first consensus message] Send sensus message to each w-node
// 	// When the settlement time expires, the message transfer goroutin ends
// 	ch1 := make(chan bool, len(w.members))
// 	for _, each := range w.members {
// 		wnodeIP := each.Addr

// 		fmt.Println("Sending to:", wnodeIP)
// 		go w.SubmitPrepare(ch1, wnodeIP)
// 	}

// 	// 2f+1 messages must be processed to send a second consensus message
// 	// [Second consensual message] Send consensus message to each w-node
// 	// When the settlement time expires, the message transfer goroutin ends

// 	votes := 0
// 	for {
// 		select {
// 		case <-ch1:
// 			votes += 1
// 			fmt.Println("🗳️PREPARE", votes, len(w.members))
// 		}
// 		// if votes >= (2*len(w.members))/3+1 {
// 		// 	fmt.Println("✅ Quorum achieved")
// 		// 	break
// 		// }
// 		if votes >= 1 {
// 			fmt.Println("✅ Quorum achieved")
// 			break
// 		}
// 	}
// 	<-ctx.Done()
// 	fmt.Println("✅ FINISH ANNOUNCE, START PREPARE")

// 	ch2 := make(chan bool, len(w.members))
// 	for _, each := range w.members {
// 		wnodeIP := each.Addr
// 		fmt.Println("Sending to:", wnodeIP)
// 		go w.SubmitCommit(ch2, wnodeIP)
// 	}

// 	// Final block commitment only if 2f+1 message is successful
// 	// Send sensus message to each w-node
// 	// to sync goroutines
// 	votes = 0
// 	for {
// 		select {
// 		case <-ch2:
// 			votes += 1
// 			fmt.Println("🗳️COMMIT", votes, len(w.members))
// 		}
// 		// if votes >= (2*len(w.members))/3+1 {
// 		// 	fmt.Println("✅ Quorum achieved")
// 		// 	break
// 		// }
// 		if votes >= 1 {
// 			fmt.Println("✅ Quorum achieved")
// 			break
// 		}
// 	}
// 	<-ctx.Done()
// 	fmt.Println("✅ FINISH PREPARE, START COMMIT")

// 	// when the consensus was successfully done, disseminate
// 	// the block to the w-nodes.(GOSSIP으로 뿌리기)
// 	gossibMsg := &pb.GossipMsg{}
// 	gossibMsg.Type = 2
// 	gossibMsg.Rndblk = w.auditMsg
// 	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossibMsg)
// }

// func (w *CONSENSUSNODE) MP2BTPbftConsensus(userCount map[string]int) {
// 	// Consensus Timer Setup
// 	ctx := context.Background()
// 	ctx, cancel := context.WithTimeout(ctx, time.Duration(10)*time.Second)
// 	defer cancel()

// 	// Make a consensus message
// 	w.auditMsg.BlkNum = w.roundIdx
// 	w.auditMsg.LeaderID = w.selfId
// 	w.auditMsg.PhaseNum = pb.AuditMsg_PREPARE
// 	w.auditMsg.MerkleRootHash = "abcdefghijklmnopqrstuvwxyz"
// 	w.auditMsg.AbortTransactionNum = userCount

// 	// Calculate Block Hash Value, Current Hash Value is determined by the following 6 field values
// 	// BlkNum, LeaderID, PhaseNum, MerkleRootHash, TxListm, PrevHash
// 	if w.auditMsg.BlkNum == 1 {
// 		w.auditMsg.CurHash = "ajknadajsnamajkndqwaakdmkaiwq"
// 	} else {
// 		b := lg.BlockHashCalculator(w.auditMsg)
// 		h := sha512.Sum512(b)
// 		w.auditMsg.CurHash = hex.EncodeToString(h[:63])
// 	}

// 	// Signing
// 	hash := []byte(w.auditMsg.CurHash)
// 	signing := sec.SignByte(hash)
// 	byteSig := signing.Serialize()

// 	w.auditMsg.Signature = byteSig

// 	//  Who signed
// 	w.auditMsg.HonestAuditors = append(w.auditMsg.HonestAuditors, w.selfId)

// 	// [Send the first consensus message] Send sensus message to each w-node
// 	// When the settlement time expires, the message transfer goroutin ends
// 	ch1 := make(chan bool, len(w.members))

// 	go w.MP2BTPSubmit(ch1, MP2BTPsession)

// 	// 2f+1 messages must be processed to send a second consensus message
// 	// [Second consensual message] Send consensus message to each w-node
// 	// When the settlement time expires, the message transfer goroutin ends

// 	votes := 0
// 	for {
// 		select {
// 		case <-ch1:
// 			votes += 1
// 		}
// 		if votes == len(w.members) {
// 			break
// 		}
// 	}
// 	<-ctx.Done()
// 	fmt.Println("✅ 응답 받음1, 다음 작업 진행")

// 	ch2 := make(chan bool, len(w.members))

// 	go w.MP2BTPSubmit(ch2, MP2BTPsession)

// 	// Final block commitment only if 2f+1 message is successful
// 	// Send sensus message to each w-node
// 	// to sync goroutines
// 	votes = 0
// 	for {
// 		select {
// 		case <-ch2:
// 			votes += 1
// 			fmt.Println("⭐️", votes)
// 		}
// 		if votes == len(w.members) {
// 			fmt.Println(votes)
// 			break
// 		}
// 		fmt.Println(votes)
// 	}
// 	<-ctx.Done()
// 	fmt.Println("✅ 응답 받음2, 다음 작업 진행")

// 	go w.MP2BTPSubmit(ch2, MP2BTPsession)

// 	// when the consensus was successfully done, disseminate
// 	// the block to the w-nodes.
// 	gossibMsg := &pb.GossipMsg{}
// 	gossibMsg.Type = 2
// 	gossibMsg.Rndblk = w.auditMsg
// 	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossibMsg)
// }

// func (w *CONSENSUSNODE) setLeaderChan(b bool, channel string) {
// 	if w.channelID != channel {
// 		w.channelID = channel
// 	}
// 	w.isLeader <- b
// }

// func (w *CONSENSUSNODE) setDone(b bool) {
// 	w.done <- b
// }

// // Check if this peer is the leader; if so, it should receive messages from Kafka.
// func (w *CONSENSUSNODE) BlockListening() {
// 	w.GetAddress()
// 	l, err := net.Listen("tcp", w.address+ConCPORT)
// 	if nil != err {
// 		log.Println(err)
// 	}
// 	defer l.Close()

// 	for {
// 		conn, err := l.Accept()
// 		if nil != err {
// 			log.Println(err)
// 			continue
// 		}
// 		go w.BloConnHandler(conn)
// 	}
// }

// func (w *CONSENSUSNODE) BloConnHandler(conn net.Conn) {
// 	defer conn.Close()
// 	recvBuf := make([]byte, 8192)
// 	n, err := conn.Read(recvBuf)
// 	data := recvBuf[:n]
// 	if nil != err {
// 		if io.EOF == err {
// 			log.Printf("connection is closed from client; %v", conn.RemoteAddr().String())
// 			return
// 		}
// 		log.Printf("fail to receive data; err: %v", err)
// 		return
// 	}

// 	MsgRecv := &pb.GossipMsg{}
// 	json.Unmarshal(data, MsgRecv)

// 	// Start to make a consensus if I'm a leader
// 	fmt.Println("🏦", MsgRecv)

// 	if MsgRecv.GetType() == 1 && MsgRecv.RndMsg.LeaderID == w.selfId {
// 		w.roundIdx = MsgRecv.RndMsg.RoundNum
// 		if !w.leader {
// 			w.leader = true
// 			w.channelID = MsgRecv.Channel
// 			fmt.Println("👑Leaderconsensus")

// 			go w.setDone(false)
// 			go w.setLeaderChan(true, MsgRecv.Channel)

// 			go w.KafkaListener(MsgRecv.Channel + "-abort")

// 			// var preparetargets []string
// 			// fmt.Println("📸", w.members, "📸")
// 			// ch1 := make(chan bool, len(w.members))
// 			// for _, each := range w.members {
// 			// 	wnodeIP := each.Addr
// 			// 	if wnodeIP == w.selfId || wnodeIP == "" {
// 			// 		fmt.Println("Skipping self:", wnodeIP)
// 			// 		continue
// 			// 	}
// 			// 	// IP만 추출하고 포트 통일
// 			// 	fmt.Println(wnodeIP)
// 			// 	hostOnly := strings.Split(wnodeIP, ":")[0]
// 			// 	target := fmt.Sprintf("%s:5000", hostOnly)
// 			// 	preparetargets = append(preparetargets, target)
// 			// }
// 			// fmt.Println(preparetargets)
// 			// MP2BTPsession = w.MP2BTPRootOnce(ch1, preparetargets)
// 		}
// 	} else if MsgRecv.GetType() == 1 && MsgRecv.RndMsg.LeaderID != w.selfId {
// 		w.leader = false
// 		go w.setDone(true)
// 		go w.setLeaderChan(false, "")
// 		fmt.Println("CONSENSUSNODE")
// 	}

// 	// Commit the block in their own ledger
// 	if MsgRecv.GetType() == 2 {
// 		fmt.Println("BLOCKINSERT")
// 		go lg.UserAbortInfoInsert(w.auditMsg)
// 	}
// }

// // decide whether the peer is elected as a leader or not
// func (w *CONSENSUSNODE) CommitteeListening() {
// 	w.GetAddress()
// 	l, _ := net.Listen("tcp", w.address+ConLPORT)
// 	fmt.Println("CommitteeListenIP", w.address+ConLPORT)
// 	defer l.Close()
// 	for {
// 		conn, err := l.Accept()
// 		if nil != err {
// 			log.Println(err)
// 			continue
// 		}

// 		go w.CommConnHandler(conn)
// 	}
// }

// func (w *CONSENSUSNODE) CommConnHandler(conn net.Conn) {

// 	defer conn.Close()
// 	recvBuf := make([]byte, 8192)
// 	n, err := conn.Read(recvBuf)
// 	data := recvBuf[:n]
// 	if nil != err {
// 		if io.EOF == err {
// 			log.Printf("connection is closed from client; %v", conn.RemoteAddr().String())
// 			return
// 		}
// 		log.Printf("fail to receive data; err: %v", err)
// 		return
// 	}
// 	recvMsg := &pb.CommitteeMsg{}
// 	json.Unmarshal(data, recvMsg)

// 	w.roundIdx = recvMsg.RoundNum

// 	var community pb.Shard

// 	for _, shard := range recvMsg.Shards {
// 		for _, mem := range shard.Member {
// 			if mem.NodeID == w.selfId {
// 				w.members = shard.Member
// 				community = *shard
// 				if recvMsg.GetType() == 1 && community.LeaderID == w.selfId {

// 					if !w.leader {
// 						w.leader = true
// 						w.channelID = community.ID
// 						w.channelID = leaderChan
// 						fmt.Println("👑Leaderconsensus")

// 						go w.setDone(false)
// 						go w.setLeaderChan(true, w.channelID)

// 						go w.KafkaListener(w.channelID + "-abort")

// 						// var preparetargets []string
// 						// fmt.Println("📸", w.members, "📸")
// 						// ch1 := make(chan bool, len(w.members))
// 						// for _, each := range w.members {
// 						// 	wnodeIP := each.Addr
// 						// 	if wnodeIP == w.selfId || wnodeIP == "" {
// 						// 		fmt.Println("Skipping self:", wnodeIP)
// 						// 		continue
// 						// 	}

// 						// 	// IP만 추출하고 포트 통일
// 						// 	fmt.Println(wnodeIP)
// 						// 	hostOnly := strings.Split(wnodeIP, ":")[0]
// 						// 	target := fmt.Sprintf("%s:5000", hostOnly)
// 						// 	preparetargets = append(preparetargets, target)
// 						// }
// 						// fmt.Println(preparetargets)
// 						// MP2BTPsession = w.MP2BTPRootOnce(ch1, preparetargets)

// 					}
// 				} else {
// 					w.leader = false
// 					w.channelID = community.ID
// 					w.channelID = leaderChan

// 					fmt.Println("CONSENSUSNODE")

// 					go w.setDone(true)
// 					go w.setLeaderChan(false, "")

// 					// push := w.MP2BTPChildOnce()
// 					// w.MP2BTPConsensusListening(push)

// 				}

// 			}
// 		}
// 	}
// }

// /*I'm a leader, send a message to a w-node*/
// func (w *CONSENSUSNODE) SubmitPrepare(ch chan<- bool, ip string) {
// 	tlsConf := &tls.Config{
// 		InsecureSkipVerify: true,
// 		NextProtos:         []string{"quic-echo-example"},
// 	}
// 	session, err := quic.DialAddr(context.Background(), ip+ConGPORT, tlsConf, nil)

// 	if err != nil {
// 		panic(err)
// 	}
// 	stream, err := session.OpenStreamSync(context.Background())
// 	if err != nil {
// 		panic(err)
// 	}
// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	_, err = stream.Write(sndBuf)
// 	if err != nil {
// 		panic(err)
// 	}

// 	remoteIP := session.RemoteAddr().String()
// 	fmt.Println("연결된 상대 IP:", remoteIP)

// 	for {
// 		rcvBuf := make([]byte, 8192)
// 		n, err := stream.Read(rcvBuf)
// 		if err != nil {
// 			panic(err)
// 		}

// 		if n == 0 {
// 			continue
// 		}

// 		cMsgRecv := &pb.AuditMsg{}
// 		err = json.Unmarshal(rcvBuf[:n], cMsgRecv)
// 		if err != nil {
// 			fmt.Println(err)
// 			continue
// 		}
// 		if cMsgRecv.PhaseNum == pb.AuditMsg_AGGREGATED_PREPARE {
// 			fmt.Println("[CMSG]", cMsgRecv)
// 			sig := bls.Sign{}
// 			_ = sig.Deserialize(cMsgRecv.Signature)
// 			sigVec = append(sigVec, sig)

// 			fmt.Println(len(sigVec) == len(w.memberMsg.Nodes))
// 			fmt.Println(len(cMsgRecv.HonestAuditors), len(w.memberMsg.Nodes))

// 			if len(sigVec) == len(w.memberMsg.Nodes) && len(cMsgRecv.HonestAuditors) == len(w.memberMsg.Nodes) {
// 				fmt.Println(len(w.memberMsg.Nodes), "[LEADER]SIGNNODES", sigVec)
// 				fmt.Println("[LEADER]CONNNODES", w.memberMsg.Nodes)
// 				w.Multisinging(cMsgRecv)
// 				fmt.Println(len(w.memberMsg.Nodes), "[LEADER]SIGNNODESAFTER", sigVec)
// 				fmt.Println("✅ AGGREGATED FINISH:", cMsgRecv.PhaseNum)
// 				w.auditMsg.HonestAuditors = cMsgRecv.HonestAuditors
// 				break
// 			} else {
// 				continue
// 			}
// 		} else {
// 			continue
// 		}
// 	}

// 	err = stream.Close()
// 	if err != nil {
// 		panic(err)
// 	}
// 	ch <- true
// }

// /* I'm a leader, send a message to a w-node*/
// func (w *CONSENSUSNODE) SubmitCommit(ch chan<- bool, ip string) {
// 	tlsConf := &tls.Config{
// 		InsecureSkipVerify: true,
// 		NextProtos:         []string{"quic-echo-example"},
// 	}
// 	session, err := quic.DialAddr(context.Background(), ip+ConGPORT, tlsConf, nil)

// 	stream, err := session.OpenStreamSync(context.Background())
// 	if err != nil {
// 		panic(err)
// 	}
// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	_, err = stream.Write(sndBuf)
// 	if err != nil {
// 		panic(err)
// 	}

// 	for {
// 		rcvBuf := make([]byte, 8192)
// 		n, err := stream.Read(rcvBuf)
// 		if err != nil {
// 			panic(err)
// 		}

// 		if n == 0 {
// 			time.Sleep(2 * time.Millisecond)
// 			continue
// 		}

// 		rcvBuf = rcvBuf[:n]
// 		cMsgRecv := &pb.AuditMsg{}
// 		err = json.Unmarshal(rcvBuf, cMsgRecv)
// 		if err != nil {
// 			fmt.Println(err)
// 			continue
// 		}
// 		if cMsgRecv.PhaseNum == pb.AuditMsg_AGGREGATED_COMMIT {
// 			sig := bls.Sign{}
// 			_ = sig.Deserialize(cMsgRecv.Signature)
// 			sigVec = append(sigVec, sig)

// 			if len(sigVec) == len(w.memberMsg.Nodes) {
// 				fmt.Println("[LEADER]SIGNNODES", sigVec)
// 				fmt.Println("[LEADER]CONNNODES", w.memberMsg.Nodes)
// 				w.Multisinging(cMsgRecv)
// 				sigVec = sigVec[:0]
// 				fmt.Println("✅ AGGREGATED FINISH:", cMsgRecv.PhaseNum)
// 				break
// 			} else {
// 				fmt.Println("⚠️ WAIT")
// 				continue
// 			}
// 		} else {
// 			continue
// 		}
// 	}

// 	err = stream.Close()
// 	if err != nil {
// 		panic(err)
// 	}
// 	ch <- true
// }

// func (w *CONSENSUSNODE) verifying(cMsg *pb.AuditMsg) bool {
// 	for _, honestID := range cMsg.HonestAuditors {
// 		for _, node := range w.memberMsg.Nodes {
// 			if node.NodeID == honestID {
// 				fmt.Println(node.NodeID, honestID)
// 				dec := &bls.PublicKey{}
// 				_ = dec.Deserialize(node.Publickey)
// 				fmt.Println(node.Publickey)
// 				pubVec = append(pubVec, *dec)
// 				break
// 			}
// 		}
// 	}
// 	fmt.Println(cMsg)
// 	fmt.Println("👑length", len(pubVec))
// 	fmt.Println("👑publickey", pubVec, "👑")

// 	dec_sign := bls.Sign{}
// 	if err := dec_sign.Deserialize(cMsg.Signature); err != nil {
// 		fmt.Println(err)
// 		return false
// 	}

// 	if cMsg.PhaseNum == pb.AuditMsg_COMMIT {
// 		hash := []byte(cMsg.CurHash)
// 		if dec_sign.FastAggregateVerify(pubVec, hash) == true {
// 			fmt.Println("AGGREGATED_COMMIT: Verification SUCCESS")
// 			pubVec = pubVec[:0]
// 			return true
// 		} else {
// 			fmt.Println("AGGREGATED_COMMIT: Verification ERROR")
// 			pubVec = pubVec[:0]
// 			return false
// 		}
// 	}
// 	return false
// }

// func WriteTomlFile(fileName string, myIP string, addrs []string) error {
// 	_ = os.Remove(fileName)
// 	f, err := os.Create(fileName)
// 	if err != nil {
// 		return err
// 	}
// 	defer f.Close()

// 	fixedConfig := fmt.Sprintf(`# MP2BTP Configurations
// VERBOSE_MODE        = true
// NUM_MULTIPATH       = 1
// MY_IP_ADDRS         = ["%s", "127.0.0.1"]
// LISTEN_PORT         = 4000
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
// THROUGHPUT_PERIOD = 0.1
// THROUGHPUT_WEIGHT = 0.1
// MULTIPATH_THRESHOLD_THROUGHPUT = 100.0
// MULTIPATH_THRESHOLD_SEND_COMPLETE_TIME = 0.0000000001
// CLOSE_TIMEOUT_PERIOD = 200
// `, myIP)
// 	_, err = f.WriteString(fixedConfig)
// 	if err != nil {
// 		return err
// 	}

// 	// [[peer_addr]]
// 	selfBlock := fmt.Sprintf(
// 		"\n[[peer_addr]]  # Offset=0\nAddr = \"%s\"\nNumChild = %d\nChildOffset = 1\n",
// 		myIP,
// 		len(addrs),
// 	)
// 	_, err = f.WriteString(selfBlock)
// 	if err != nil {
// 		return err
// 	}

// 	// 2. 나머지 peer addr들 작성 (Offset=1부터 시작)
// 	for i, addr := range addrs {
// 		block := fmt.Sprintf(
// 			"\n[[peer_addr]]  # Offset=%d\nAddr = \"%s\"\nNumChild = 0\nChildOffset = 0\n",
// 			i+1, addr,
// 		)
// 		_, err := f.WriteString(block)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// // -------------------------------------------------------------------------------------- //
// var mu sync.Mutex

// func (w *CONSENSUSNODE) MP2BTPRootOnce(ch chan<- bool, addrs []string) *PuCtrl.PuCtrl {
// 	mu.Lock()
// 	defer mu.Unlock()
// 	w.GetAddress()

// 	peerId := rd.Intn(10) + 1

// 	configFile := "./push_root.toml"
// 	fmt.Println("MP2BTPSubmit IP:", addrs)

// 	err := WriteTomlFile(configFile, w.address, addrs)
// 	if err != nil {
// 		fmt.Println("❌ TOML 파일 생성 실패:", err)
// 		ch <- false
// 		return nil
// 	}

// 	fmt.Println("✅ TOML 파일 생성 완료:", configFile)

// 	var config PuCtrl.Config
// 	if _, err := toml.DecodeFile(configFile, &config); err != nil {
// 		panic(err)
// 	}

// 	pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

// 	fmt.Println("🖥️", config.PEER_ADDRS, "🖥️")

// 	nodeInfo := pu.GetNodeInfo(config.PEER_ADDRS)

// 	pu.Listen()

// 	time.Sleep(1 * time.Second)

// 	pu.Connect(nodeInfo, byte(PuCtrl.PUSH_SESSION))

// 	time.Sleep(1 * time.Second)
// 	return pu
// }
// func (w *CONSENSUSNODE) MP2BTPSubmit(ch chan<- bool, pu *PuCtrl.PuCtrl) {

// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		fmt.Println("❌ JSON 직렬화 실패:", err)
// 		ch <- false
// 		return
// 	}
// 	pu.SendAuditMsg(sndBuf)
// 	fmt.Println("💛FINISH", sndBuf, "💛FINISH")

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	resultCh := make(chan recvResult)
// 	go receiveWorker(pu, resultCh)

// 	for {
// 		select {
// 		case result := <-resultCh:
// 			if result.err != nil {
// 				fmt.Println("❌ Error:", result.err)
// 				ch <- false
// 				return
// 			}
// 			if result.buf == nil || len(result.buf) == 0 {
// 				continue
// 			}
// 			datalength := int(result.buf[0])
// 			if datalength > len(result.buf)-1 {
// 				fmt.Println("⚠️ 데이터 길이 이상:", datalength)
// 				ch <- false
// 				return
// 			}
// 			resultData := make([]byte, datalength)
// 			copy(resultData, result.buf[1:datalength+1])

// 			fmt.Printf("📦 수신 (%d bytes): %v\n", datalength, resultData)
// 			ch <- true
// 			break

// 		case <-ctx.Done():
// 			fmt.Println("🛑 수신 시간 초과 또는 취소")
// 			ch <- false
// 			return
// 		}
// 		break
// 	}

// }

// type recvResult struct {
// 	n   int
// 	err error
// 	buf []byte
// }

// func receiveWorker(pu *PuCtrl.PuCtrl, resultCh chan<- recvResult) {
// 	for {
// 		buf := make([]byte, 8192)
// 		buf, err := pu.ReceiveRaw(buf)
// 		if err != nil {
// 			resultCh <- recvResult{buf: nil, err: err}
// 			return
// 		}
// 		resultCh <- recvResult{buf: buf, err: nil}
// 	}
// }
// func (w *CONSENSUSNODE) MP2BTPChildOnce() *PuCtrl.PuCtrl {
// 	fmt.Println("😬 Starting Consensus Child")

// 	configFile := "./push_child.toml"
// 	peerId := rd.Intn(10) + 1

// 	if _, err := toml.DecodeFile(configFile, new(PuCtrl.Config)); err != nil {
// 		fmt.Println("❌ Config 파일 파싱 실패:", err)
// 		return nil
// 	}

// 	pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

// 	time.Sleep(1 * time.Second)

// 	pu.Listen()

// 	time.Sleep(1 * time.Second)
// 	return pu
// }

// func (w *CONSENSUSNODE) MP2BTPConsensusListening(pu *PuCtrl.PuCtrl) {

// 	s, _ := os.Create(fmt.Sprintf("log.txt"))
// 	defer s.Close()

// 	startTime := time.Now()
// 	recvBytes := 0
// 	datalength := 0
// 	resultData := make([]byte, datalength)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	resultCh := make(chan recvResult)
// 	go receiveWorker(pu, resultCh)

// 	for {
// 		select {
// 		case result := <-resultCh:
// 			if result.err != nil {
// 				fmt.Println("❌ Error:", result.err)
// 				return
// 			}
// 			if result.buf == nil || len(result.buf) == 0 {
// 				continue
// 			}
// 			fmt.Println("⭐️")
// 			datalength = int(result.buf[0])
// 			if datalength > len(result.buf)-1 {
// 				fmt.Println("⚠️ 데이터 길이 이상:", datalength)
// 				return
// 			}

// 			copy(resultData, result.buf[1:datalength+1])
// 			fmt.Printf("📦 수신 (%d bytes): %v\n", datalength, resultData)
// 			break

// 		case <-ctx.Done():
// 			fmt.Println("🛑 수신 시간 초과 또는 취소")
// 			return
// 		}
// 		break
// 	}

// 	elapsedTime := time.Since(startTime)
// 	throughput := (float64(recvBytes) * 8.0) / elapsedTime.Seconds() / (1000 * 1000)
// 	logStr := fmt.Sprintf("Seconds=%f, Throughput=%f, ReceivedSize=%d\n", elapsedTime.Seconds(), throughput, recvBytes)
// 	s.Write([]byte(logStr))

// 	var auditMsg pb.AuditMsg
// 	fmt.Println(datalength, resultData[1:datalength+1])
// 	if err := json.Unmarshal(resultData[1:datalength+1], &auditMsg); err != nil {
// 		fmt.Println("Unmarshal error:", err)
// 	}

// 	if auditMsg.PhaseNum == pb.AuditMsg_PREPARE {
// 		hash := []byte(auditMsg.PrevHash)
// 		signing := sec.SignByte(hash)
// 		byteSig := signing.Serialize()
// 		w.auditMsg.Signature = byteSig
// 		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE
// 	}

// 	if auditMsg.PhaseNum == pb.AuditMsg_COMMIT {
// 		hash := []byte(auditMsg.PrevHash)
// 		signing := sec.SignByte(hash)
// 		byteSig := signing.Serialize()
// 		w.auditMsg.Signature = byteSig
// 		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
// 	}

// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = pu.SendAuditMsg(sndBuf)
// 	if err != nil {
// 		panic(err)
// 	}
// }

// // -------------------------------------------------------------------------------------- //

// /*Consensus three-phases: Announce(Completed State), Prepare, Commit] I'm not a leader*/
// func (w *CONSENSUSNODE) ConsensusListening() {

// 	w.GetAddress()
// 	listener, err := quic.ListenAddr(w.address+ConGPORT, generateTLSConfig(), nil)
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
// 			panic(err)
// 		}

// 		if n == 0 {
// 			time.Sleep(2 * time.Millisecond)
// 			continue
// 		}

// 		rcvBuf = rcvBuf[:n]
// 		cMsgRecv := &pb.AuditMsg{}
// 		err = json.Unmarshal(rcvBuf, cMsgRecv)
// 		if err != nil {
// 			fmt.Println(err)
// 			continue
// 		}
// 		fmt.Println("👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓👨🏼‍🎓", cMsgRecv.PhaseNum)

// 		if cMsgRecv.PhaseNum == pb.AuditMsg_PREPARE {
// 			hash := []byte(cMsgRecv.CurHash)
// 			signing := sec.SignByte(hash)
// 			byteSig := signing.Serialize()
// 			w.auditMsg.Signature = byteSig
// 			w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_PREPARE

// 			alreadyExists := false
// 			for _, id := range cMsgRecv.HonestAuditors {
// 				if id == w.selfId {
// 					alreadyExists = true
// 					break
// 				}

// 				// 없을 때만 추가
// 				if !alreadyExists {
// 					w.auditMsg.HonestAuditors = append(cMsgRecv.HonestAuditors, w.selfId)
// 				} else {
// 					w.auditMsg.HonestAuditors = cMsgRecv.HonestAuditors
// 				}
// 			}
// 			w.auditMsg.CurHash = cMsgRecv.CurHash
// 			w.auditMsg.MerkleRootHash = cMsgRecv.MerkleRootHash
// 			break
// 		} else if cMsgRecv.PhaseNum == pb.AuditMsg_COMMIT {
// 			w.verifying(cMsgRecv)
// 			hash := []byte(cMsgRecv.CurHash)
// 			signing := sec.SignByte(hash)
// 			byteSig := signing.Serialize()
// 			w.auditMsg.Signature = byteSig
// 			w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
// 			alreadyExists := false
// 			for _, id := range cMsgRecv.HonestAuditors {
// 				if id == w.selfId {
// 					alreadyExists = true
// 					break
// 				}

// 				// 없을 때만 추가
// 				if !alreadyExists {
// 					w.auditMsg.HonestAuditors = append(cMsgRecv.HonestAuditors, w.selfId)
// 				} else {
// 					w.auditMsg.HonestAuditors = cMsgRecv.HonestAuditors
// 				}
// 			}
// 			w.auditMsg.CurHash = cMsgRecv.CurHash
// 			w.auditMsg.MerkleRootHash = cMsgRecv.MerkleRootHash
// 			break
// 		} else {
// 			fmt.Println("📭 의미 없는 PhaseNum:", cMsgRecv.PhaseNum, "계속 대기")
// 			continue
// 		}
// 	}

// 	fmt.Println("[w.auditMsg]", w.auditMsg)
// 	sndBuf, err := json.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	_, err = stream.Write(sndBuf)
// 	if err != nil {
// 		panic(err)
// 	}
// }

// func (w *CONSENSUSNODE) SendAggregatedMsg(stream quic.Stream) {
// 	sndBuf, err := proto.Marshal(w.auditMsg)
// 	if err != nil {
// 		panic(err)
// 	}
// 	_, err = stream.Write(sndBuf)
// 	if err != nil {
// 		panic(err)
// 	}
// }

// func (w *CONSENSUSNODE) Multisinging(cMsgs *pb.AuditMsg) (bool, int) {
// 	if cMsgs.PhaseNum == pb.AuditMsg_AGGREGATED_PREPARE {
// 		// Multi-Signature Signing
// 		fmt.Println("MULTISIGNING", sigVec)
// 		var aggeSign bls.Sign
// 		aggeSign.Aggregate(sigVec)
// 		byteSig := aggeSign.Serialize()

// 		w.auditMsg.BlkNum = cMsgs.BlkNum
// 		w.auditMsg.PrevHash = cMsgs.PrevHash
// 		w.auditMsg.CurHash = cMsgs.CurHash
// 		w.auditMsg.Signature = byteSig
// 		w.auditMsg.PhaseNum = pb.AuditMsg_COMMIT

// 		sigVec = sigVec[:0]

// 		return true, len(w.auditMsg.HonestAuditors)
// 	}
// 	if cMsgs.PhaseNum == pb.AuditMsg_AGGREGATED_COMMIT {
// 		var aggeSign bls.Sign
// 		aggeSign.Aggregate(sigVec)
// 		sigVec = sigVec[:0]
// 		byteSig := aggeSign.Serialize()

// 		w.auditMsg.BlkNum = cMsgs.BlkNum
// 		w.auditMsg.PrevHash = cMsgs.PrevHash
// 		w.auditMsg.CurHash = cMsgs.CurHash

// 		w.auditMsg.Signature = byteSig
// 		w.auditMsg.PhaseNum = pb.AuditMsg_AGGREGATED_COMMIT
// 		return true, len(w.auditMsg.HonestAuditors)
// 	}
// 	return false, 0
// }

// /*GRPC SERVICE Membershhip Exchanging Procedure*/
// func (w *CONSENSUSNODE) Reporting() {
// 	conn, err := grpc.Dial(validatorGrpcAddr, grpc.WithInsecure(), grpc.WithBlock())
// 	if err != nil {
// 		log.Fatalf("did not connect: %v", err)
// 	}
// 	defer conn.Close()
// 	c := pb.NewMembershipServiceClient(conn)
// 	Membership := &pb.MemberMsg{}
// 	node := new(pb.Node)
// 	node.NodeID = w.selfId
// 	node.Port = "11730" // listening port when the leader is elected
// 	bytePub := pub.Serialize()
// 	node.Publickey = bytePub
// 	node.Alive = true
// 	// Send my info to S-node
// 	Membership.Nodes = append(Membership.Nodes, node)

// 	// Send your membership information, receive all w-nodes membership
// 	w.memberMsg, err = c.GetMembership(context.Background(), Membership)
// 	if err != nil {
// 		log.Fatalf("could not greet: %v", err)
// 	}
// }

// func MerkleHash(s []uint64) string {
// 	ss := int64(s[0])
// 	xx := strconv.FormatInt(ss, 16)
// 	return xx
// }

// func ReportExternal(id string) {
// 	sndBuf, _ := json.Marshal(id)
// 	conn, err := net.Dial("tcp", validatorNatAddr)
// 	if err != nil {
// 		fmt.Println("[NAT] Failed to Dial : ", err)
// 	}
// 	defer conn.Close()
// 	_, err = conn.Write(sndBuf)

// 	if err != nil {
// 		fmt.Println("[NAT] Failed to write data : ", err)
// 	}
// 	_ = conn.Close()
// }

// /* Setup a bare-bones TLS config for the server --------------------------------------------------------------------- */
// func generateTLSConfig() *tls.Config {
// 	key, err := rsa.GenerateKey(rand.Reader, 1024)
// 	if err != nil {
// 		panic(err)
// 	}
// 	template := x509.Certificate{SerialNumber: big.NewInt(1)}
// 	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
// 	if err != nil {
// 		panic(err)
// 	}
// 	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
// 	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

// 	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return &tls.Config{
// 		Certificates: []tls.Certificate{tlsCert},
// 		NextProtos:   []string{"quic-echo-example"},
// 	}
// }
