package main

import (
	lg "auditchain/ledger"
	pb "auditchain/msg"
	wp2p "auditchain/wp2p"

	// "os/exec"
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
	"io/ioutil"
	"log"
	"math/big"
	rd "math/rand"
	"net"
	"net/http"
	"os"
	"regexp"

	"google.golang.org/protobuf/proto"

	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"sort"

	"github.com/BurntSushi/toml"
	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/herumi/bls-eth-go-binary/bls"
	// "github.com/prometheus/client_golang/prometheus"
	// "github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CONSENSUSNODE struct {
	addresses          []string
	selfId           string
	roundIdx         uint32
	members          []*pb.Wnode
	isLeader         atomic.Bool
	leader           bool
	channelID        string
	done             chan bool
	pendingCommittee *pb.CommitteeMsg // 새 커미티 대기용

	kafkaConsumer    *kafka.Consumer
	blockMsg         *pb.TransactionMsg
	memberMsg        *pb.MemberMsg
	auditMsg         *pb.AuditMsg
	cancel           context.CancelFunc
	isRunning        atomic.Bool
	isChildListening atomic.Bool

	mp2btpOnce 		 sync.Once
	pu               *PuCtrl.PuCtrl
	puMu             sync.Mutex
	publicKeyMap map[string]*bls.PublicKey
}

const baseleaderTomlConfig = `
LISTEN_PORT         = 4000
BIND_PORT           = 5000
EQUIC_ENABLE        = true
EQUIC_PATH          = "./transceiver/node/fectun"
EQUIC_APP_SRC_PORT  = 6000
EQUIC_APP_DST_PORT  = 5000
EQUIC_TUN_SRC_PORT  = 7000
EQUIC_TUN_DST_PORT  = 7000
EQUIC_FEC_MODE      = false
SEGMENT_SIZE        = 2097152
FEC_SEGMENT_SIZE    = 2466816
THROUGHPUT_PERIOD   = 0.1
THROUGHPUT_WEIGHT   = 0.1
MULTIPATH_THRESHOLD_THROUGHPUT = 100.0
MULTIPATH_THRESHOLD_SEND_COMPLETE_TIME = 0.0000000001
CLOSE_TIMEOUT_PERIOD = 200
`

// ======================= Constants =======================
const (
	validatorIP       = "117.16.244.33"
	validatorGrpcPort = "16220"
	validatorNatPort  = "11730"
	gossipMsgPort     = ":4242"
	grpcPort          = ":5252"
	gossipListenPort  = ":6262"
	prometheusPort    = ":12345"
	abortTopic ="mychannel-abort"
	addrDB = "mongodb://localhost:27017"
)

type VRFNode struct {
	NodeID string
	PubKey []byte
	Seed   string
	Proof  []byte
	Value  *big.Int
}

var VRFGlobal *VRFNode

// ======================= Global Variables =======================
var (
	startConsensusTime int64
	nodename,_           = os.Hostname()
	kafkaGroupID      = "GROUP"+nodename
	seed string

	// ======================= Kafka / Network =======================
	validatorGrpcAddr string
	validatorNatAddr  string
	brokers           string
	leaderChan        string

	// ======================= BLS =======================
	sec    bls.SecretKey // 노드의 BLS 비밀키, VRF에도 활용
	pub    *bls.PublicKey // 노드의 BLS 공개키, VRF에도 활용
	sigVec []bls.Sign // 네트워크에 참여하는 노드들의 BLS 서명 모음
	pubVec []bls.PublicKey // 네트워크에 참여하는 노드들의 BLS 공개키 모음

	mongoclient *mongo.Client

	// ======================= Context =======================
	ctx    context.Context
	cancel context.CancelFunc

	// ======================= Consensus Status =======================
	consensusStatusMap map[string]bool

)

var mu sync.RWMutex
var consensusprotocol *bool
var MP2BTPsession *PuCtrl.PuCtrl

func NewKafkaConsumer() *kafka.Consumer {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
					"bootstrap.servers":       brokers,
					"group.id":                kafkaGroupID,
					"auto.offset.reset":       "earliest",
					"enable.auto.commit":      false,
					"fetch.min.bytes":         1,
					"fetch.wait.max.ms":       10,
	})
	if err != nil {
					log.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	return c
}

func (w *CONSENSUSNODE) InitKafka() {
	w.kafkaConsumer = NewKafkaConsumer()

	if err := w.kafkaConsumer.SubscribeTopics([]string{abortTopic}, nil); err != nil {
			log.Fatalf("❌ Failed to subscribe Kafka topic %s: %v", abortTopic, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	go w.KafkaListener(ctx, abortTopic)
}

func main() {
	defaultValidatorAddr := flag.String("snode", "117.16.244.33", "Validator node IP address")
	kafkaProcessorAddr := flag.String("broker", "117.16.244.33", "Kafka broker IP address")
	libp2pAddr := flag.String("libp2p", "/ip4/117.16.244.33/tcp/4001/p2p/QmYETE98oFbZNSrfp64Ms63VMLoRRRGRejw8S2kVqqnDEU", "Libp2p multiaddress")
	channel := flag.String("channel", "mychannel", "Channel name")
	consensusprotocol = flag.Bool("networktype", true, "true = MP2BTP, false = QUIC")

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

	// http.Handle("/metrics", promhttp.Handler())
	// go monitoring()

	wp2p.JoinShard(leaderChan)

	consensusNode.start()
}

func (w *CONSENSUSNODE) GetAddresses() []string {
    var ips []string

    ifaces, err := net.Interfaces()
    if err != nil {
        return ips
    }

    for _, iface := range ifaces {
        if iface.Flags&net.FlagLoopback != 0 {
            continue
        }
        if iface.Flags&net.FlagUp == 0 {
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
            if ip == nil {
                continue
            }

            ip4 := ip.To4()
            if ip4 == nil {
                continue
            }

            ips = append(ips, ip4.String())
        }
		fmt.Println(ips)
    }
    return ips
}

// getPublicIP retrieves the instance's public IPv4 from AWS metadata
func getPublicIP() (string, error) {
	client := &http.Client{
		Timeout: 2 * time.Second, // 메타데이터 호출에 타임아웃 설정
	}

	resp, err := client.Get("http://169.254.169.254/latest/meta-data/public-ipv4")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(ip), nil
}

// 도커 컨테이너 ip 받기
func getContainerIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			if !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", nil
}

func (w *CONSENSUSNODE) start() {
	// w.getOutboundIP()
	w.isLeader.Store(false)
	w.done = make(chan bool, 1)
	w.isRunning.Store(false)

	w.addresses=w.GetAddresses()

	consensusStatusMap = make(map[string]bool)

	pu := w.MP2BTPChildOnce()

	go w.InitKafka()
	go w.MP2BTPConsensusListening(pu)
	go w.CommitteeListening()
	go w.BlockListening()

	w.publicKeyMap = make(map[string]*bls.PublicKey)

	w.Reporting()

	time.Sleep(3 * time.Second)
	ReportExternal(w.selfId)

	w.ListenForCommitteeMsgs()
}

func (w *CONSENSUSNODE) ListenForCommitteeMsgs() {
	for {
		msg := <-committeeMsgChannel // 어떤 방식으로든 메시지를 수신했다고 가정
		log.Println(msg)
	}
}

var committeeMsgChannel = make(chan *pb.CommitteeMsg, 10)


// ======================= Kafka Listener =======================
type AbortPayload struct {
	Timestamp int64                `json:"timestamp"`
	Data      []*pb.TransactionMsg `json:"data"`
}

func (w *CONSENSUSNODE) KafkaListener(ctx context.Context, topic string) {
	re := regexp.MustCompile(`User(\d+)`)

	for {
	select {
	case <-ctx.Done():
		fmt.Println("KafkaListener stopped")
		// return
	default:
			msg, err := w.kafkaConsumer.ReadMessage(-1)
			if err != nil {
							log.Printf("Kafka read error: %v", err)
							continue
			}

			_, err = w.kafkaConsumer.CommitMessage(msg)
			if err != nil {
							log.Printf("Commit failed: %v", err)
			}

			if !w.leader {
					continue
			}

			// JSON 언마샬
			var abortPayload AbortPayload
			err = json.Unmarshal(msg.Value, &abortPayload)
			if err != nil {
							log.Printf("Failed to unmarshal Kafka message: %v", err)
							continue
			}

			userCount := make(map[string]int32)
			for _, data := range abortPayload.Data {
					match := re.FindStringSubmatch(data.String())
					if len(match) > 1 {
									idx := idxToInt(match[1])
									userKey := "User" + strconv.Itoa(idx)
									userCount[userKey]++
					}
			}

			startConsensusTime = abortPayload.Timestamp
			w.auditMsg = &pb.AuditMsg{}
			w.bftConsensus(userCount)
		}
	}
}

func idxToInt(s string) int {
	var num int
	fmt.Sscanf(s, "%d", &num)
	return num
}

// ======================= Committee Listening =======================
func (w *CONSENSUSNODE) CommitteeListening() {
	listener, err := net.Listen("tcp", w.addresses[0]+gossipListenPort)
        if err != nil {
                        log.Fatalf("Failed to listen on %s: %v", w.addresses[0]+gossipListenPort, err)
        }
        defer listener.Close()

        fmt.Println("🟢 Committee Listening on", w.addresses[0]+gossipListenPort)

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

	recvBuf := make([]byte, 20000)
	n, err := conn.Read(recvBuf)
	if err != nil {
			if err != io.EOF {
			log.Printf("❌ Failed to read data: %v", err)
			}
			return
	}

	recvMsg := &pb.CommitteeMsg{}
	if err := json.Unmarshal(recvBuf[:n], recvMsg); err != nil {
			log.Printf("❌ Failed to unmarshal CommitteeMsg: %v", err)
			return
	}

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
	listener, err := net.Listen("tcp", w.addresses[0]+grpcPort)
	if err != nil {
			log.Fatalf("❌ Failed to listen on %s: %v", w.addresses[0]+grpcPort, err)
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
	go lg.BlkInsert(MsgRecv.Rndblk)
	elapsedMs := time.Now().UnixMilli() - MsgRecv.StartConsensusTime
	elapsedSec := float64(elapsedMs) / 1000.0

	fmt.Printf("consensus elapsed time: %.3f seconds\n", elapsedSec)
	w.Reporting()
}

   
type VRFCandidate struct {
	Node *pb.Wnode
	VRF  *big.Int
}

func (w *CONSENSUSNODE) applyCommittee(msg *pb.CommitteeMsg) {
	w.done = make(chan bool, 1)
	w.roundIdx = msg.RoundNum

	for _, shard := range msg.Shards {
		// 해당 shard에 내가 포함되어 있는지 확인
		containsSelf := false
		for _, mem := range shard.Member {
			if mem.NodeID == w.selfId {
				containsSelf = true
				break
			}
		}
		if !containsSelf {
			continue
		}

		// === (1) VRF 검증 및 후보 리스트 생성 ===
		var candidates []VRFCandidate
		for _, mem := range shard.Member {
			var pubKey bls.PublicKey
			if err := pubKey.Deserialize(mem.Publickey); err != nil {
				fmt.Printf("❌ Node %s PublicKey 역직렬화 실패\n", mem.NodeID)
				return
			}
                        w.publicKeyMap[mem.NodeID] = &pubKey

			vrfVal, ok := VerifyVRF(mem.Seed, mem.Proof, pubKey)
			if !ok {
				fmt.Printf("❌ Node %s VRF 검증 실패\n", mem.NodeID)
				return
			}
			fmt.Printf("✅ Node %s VRF 검증 성공\n", mem.NodeID)

			candidates = append(candidates, VRFCandidate{
				Node: mem,
				VRF:  vrfVal,
			})
		}

		// === (2) VRF 순으로 정렬 ===
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].VRF.Cmp(candidates[j].VRF) < 0
		})

		// === (3) 위조 여부 확인 (순서 및 리더ID) ===
		for i := range shard.Member {
			if shard.Member[i].NodeID != candidates[i].Node.NodeID {
				fmt.Println("❌ Committee 멤버 순서 불일치 → 메시지 위조 가능성")
				return
			}
		}
		if shard.LeaderID != candidates[0].Node.NodeID {
			fmt.Println("❌ Leader 불일치 → 메시지 위조 가능성")
			return
		}

		// === (4) 합의 상태 적용 ===
		log.Printf("🔹 CommitteeMsg Round: %d 적용됨", msg.RoundNum)
		w.members = shard.Member
		w.leader = (shard.LeaderID == w.selfId)
		w.channelID = leaderChan

		if w.leader {
			fmt.Println("🎉 Leader consensus node")
			go func() {
				defer w.setDone(false)
				defer w.isRunning.Store(false)
			}()
		} else {
			fmt.Println("🧩 Follower consensus node")
		}
		return
	}

	fmt.Println("CommitteeMsg에 해당 노드가 포함되어 있지 않음")
}

func (w *CONSENSUSNODE) setDone(b bool) {
	w.done <- b
}

func VerifyVRF(seed string, proof []byte, pubKey bls.PublicKey) (*big.Int, bool) {
	var sig bls.Sign
	if err := sig.Deserialize(proof); err != nil {
		return nil, false
	}

	seedHash := sha256.Sum256([]byte(seed))
	if !sig.FastAggregateVerify([]bls.PublicKey{pubKey}, seedHash[:]) {
		return nil, false
	}

	sigHash := sha256.Sum256(proof)
	vrfValue := new(big.Int).SetBytes(sigHash[:])

	return vrfValue, true
}

func VRFHash(nodeID, seed string) *big.Int { 
		data := []byte(nodeID + seed) 
		hash := sha256.Sum256(data) 
		return new(big.Int).SetBytes(hash[:]) 
}

func (w *CONSENSUSNODE) setPu(pu *PuCtrl.PuCtrl) {
	w.puMu.Lock()
	defer w.puMu.Unlock()
	w.pu = pu
}

func (w *CONSENSUSNODE) closePu(pu *PuCtrl.PuCtrl) {
	// w.puMu.Lock()
	// defer w.puMu.Unlock()
	pu.CloseAllSessions() // 또는 Close()
}

func GetMongoClient() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mongoclient, _ = mongo.Connect(ctx, options.Client().ApplyURI(addrDB))
}

func GetPrevBlockHash() (string, error) {
	GetMongoClient()
	collection := mongoclient.Database("ledger").Collection("blk")

	var lastBlock struct {
			CurHash string `bson:"CurHash"`
	}

	opts := options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})

	err := collection.FindOne(context.Background(), bson.D{}, opts).Decode(&lastBlock)
	if err != nil {
			if err == mongo.ErrNoDocuments {
					// 데이터가 없을 경우 기본값 반환
					defaultHash := strings.Repeat("0", 64) // 64자리 0 (SHA-256 기본 길이)
					return defaultHash, nil
			}
			return "", err // 다른 에러일 경우 그대로 반환
	}

	return lastBlock.CurHash, nil
}

func makePadding(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = 'A'
	}
	return string(b)
}

func (w *CONSENSUSNODE) bftConsensus(userCount map[string]int32) {
	PrevHash, _ := GetPrevBlockHash()

	w.auditMsg = &pb.AuditMsg{
		BlkNum:              w.roundIdx,
		LeaderID:            w.selfId,
		PrevHash:            PrevHash,
		PhaseNum:            pb.AuditMsg_PREPARE,
		MerkleRootHash:      makePadding(10000),
		Aborttransactionnum: userCount,
		HonestAuditors: []*pb.HonestAuditor{
			// {Id: w.selfId},
		},
	}

	hashBytes := lg.BlockHashCalculator(w.auditMsg)
	hash := sha512.Sum512(hashBytes)
	w.auditMsg.CurHash = hex.EncodeToString(hash[:63])

	signing := sec.SignByte([]byte(w.auditMsg.CurHash))
	w.auditMsg.Signature = signing.Serialize()

	// ==== PREPARE & COMMIT Broadcasting ====
	fmt.Println("🚀 Broadcasting PREPARE")
	chPreparetest := make(chan bool, len(w.members))
	chPrepare := make(chan *pb.AuditMsg, len(w.members))
	var addrs []string

	self := make(map[string]struct{})
	for _, a := range w.addresses {
		self[a] = struct{}{}
	}

	for _, each := range w.members {
		// 자기 자신 노드는 스킵
		isSelfNode := false
		for _, addr := range each.Addrs {
			if _, ok := self[addr]; ok {
				isSelfNode = true
				break
			}
		}
		if isSelfNode {
			continue
		}

		// 상대 노드의 주소들 추가
		addrs = append(addrs, each.Addrs...)
	}
	fmt.Println("남의 주소",addrs)


	MP2BTPsession = w.MP2BTPRootOnce(chPreparetest, addrs)
	ctxPrepare, cancelPrepare := context.WithCancel(context.Background())
	w.MP2BTPSubmit(ctxPrepare,chPrepare, MP2BTPsession,pb.AuditMsg_PREPARE, pb.AuditMsg_AGGREGATED_PREPARE)

	quorum := len(w.members)-1 // TODO: 2f+1 로 계산
	w.waitForVotes(chPrepare, quorum, pb.AuditMsg_AGGREGATED_PREPARE)
	cancelPrepare()

	fmt.Println("🚀 Broadcasting COMMIT")
	chCommit := make(chan *pb.AuditMsg, len(w.members))
	ctxCommit, cancelCommit := context.WithCancel(context.Background())

	w.MP2BTPSubmit(ctxCommit,chCommit, MP2BTPsession, pb.AuditMsg_COMMIT,pb.AuditMsg_AGGREGATED_COMMIT)

	w.waitForVotes(chCommit, quorum, pb.AuditMsg_AGGREGATED_COMMIT)
	cancelCommit()

	fmt.Println("📦 Disseminating block to peers")
	w.auditMsg.MerkleRootHash="abcdefghijklnmop"
	gossipMsg := &pb.GossipMsg{
		Type:               2,
		Rndblk:             w.auditMsg,
		StartConsensusTime: startConsensusTime,
	}
	wp2p.WDNMessage(wp2p.Wctx, wp2p.Shard[0], gossipMsg)
	time.Sleep(100 * time.Millisecond) 
	w.closePu(MP2BTPsession)
}

func (w *CONSENSUSNODE) waitForVotes(ch chan *pb.AuditMsg, quorum int,targetPhase pb.AuditMsg_Phases) bool {
	votes := 0
	var sigVec []bls.Sign
	honestAuditorsSet := make(map[string]*pb.HonestAuditor)

	timeout := time.After(20 * time.Second)

	for {
		select {
		case msg := <-ch:
			if msg == nil {
				log.Println("⚠️ received nil vote")
				continue
			}

			if msg.PhaseNum == pb.AuditMsg_PHASE_UNSPECIFIED {
				fmt.Println("pb.AuditMsg_PHASE_UNSPECIFIED")
				continue
			}

			if msg.PhaseNum != targetPhase {
				log.Printf("❌ Unexpected phase: got=%v expected=%v",msg.PhaseNum, targetPhase)
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

			w.auditMsg.HonestAuditors = make([]*pb.HonestAuditor, 0, len(honestAuditorsSet))
			for _, auditor := range honestAuditorsSet {
				w.auditMsg.HonestAuditors = append(w.auditMsg.HonestAuditors, auditor)
			}

			if votes == quorum {
				fmt.Printf("✅ Phase %v Aggregation Complete\n", targetPhase)
				w.Multisinging(msg, sigVec)
				return true
			}

		case <-timeout:
			log.Printf(
				"⏰ waitForVotes timeout: votes=%d quorum=%d targetPhase=%v",
				votes, quorum, targetPhase,
			)
			return false
		}
	}
}

func (w *CONSENSUSNODE) sendMP2BTPResponse(targetPhase pb.AuditMsg_Phases,cMsg *pb.AuditMsg, pu *PuCtrl.PuCtrl) {
	resp := proto.Clone(cMsg).(*pb.AuditMsg)

	signing := sec.SignByte([]byte(resp.CurHash))
	resp.Signature = signing.Serialize()
	resp.PhaseNum = targetPhase

	w.updateAuditFields(resp)

	sndBuf, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	pu.SendAuditAckMsg(sndBuf)

}

func (w *CONSENSUSNODE) updateAuditFields(msg *pb.AuditMsg) {
	auditors := make([]*pb.HonestAuditor, 0, len(msg.HonestAuditors)+1)

	already := false
	for _, a := range msg.HonestAuditors {
		if a == nil {
			continue
		}
		auditors = append(auditors, a)
		if a.Id == w.selfId {
			already = true
		}
	}

	if !already {
		auditors = append(auditors, &pb.HonestAuditor{Id: w.selfId})
	}

	msg.HonestAuditors = auditors
}

func (w *CONSENSUSNODE) verifying(cMsg *pb.AuditMsg) bool {
	pubVec = pubVec[:0]
	seenKeys := make(map[string]bool)

	fmt.Println("cMsg.HonestAuditors",cMsg.HonestAuditors)
	for _, honestID := range cMsg.HonestAuditors {
			if pk := w.getPublicKeyByNodeID(honestID.Id); pk != nil {
					keyBytes := pk.Serialize()
					keyStr := string(keyBytes) // serialize된 바이트를 문자열로

					if !seenKeys[keyStr] {
					pubVec = append(pubVec, *pk)
					seenKeys[keyStr] = true
					} else {
					fmt.Printf("⚠️ Duplicate public key ignored for node: %s\n", honestID.Id)
					}
			} else {
					fmt.Printf("Missing public key for node: %s\n", honestID.Id)
			}
	}
	

	var decSign bls.Sign
	if err := decSign.Deserialize(cMsg.Signature); err != nil {
			fmt.Println("Signature deserialization error:", err)
			return false
	}

	switch cMsg.PhaseNum {
	case pb.AuditMsg_COMMIT:
			// if decSign.FastAggregateVerify(pubVec, []byte(cMsg.CurHash)) {
				fmt.Println("✅ AGGREGATED_COMMIT: Verification SUCCESS")
				return true
			// }  
			fmt.Println("❌ AGGREGATED_COMMIT: Verification ERROR",len(pubVec))
	}
	return false
}

func (w *CONSENSUSNODE) getPublicKeyByNodeID(id string) *bls.PublicKey {
	if pk, ok := w.publicKeyMap[id]; ok {
	return pk
}
fmt.Printf("🔍 Public key for node %s not found in publicKeyMap\n", id)
return nil
}

// func (w *CONSENSUSNODE) getOutboundIP() string {
// 	conn, err := net.Dial("udp", "8.8.8.8:80")
// 	if err != nil {
// 			panic(err)
// 	}
// 	defer conn.Close()
// 	localAddr := conn.LocalAddr().(*net.UDPAddr)
// 	w.address=localAddr.IP.String()

// 	return localAddr.IP.String()
// }


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
	GenerateVRF(w.addresses[0], seed)
	req := createSelfMembership(w.selfId, w.addresses,"11730", VRFGlobal)

	w.memberMsg, err = c.GetMembership(context.Background(), req)
	if err != nil {
			log.Fatalf("Failed to get membership: %v", err)
	}
}

func getConsensusStatus(id string) bool {
	return !consensusStatusMap[id]
}

func GenerateVRF(nodeID string, seed string) {
	seed = strconv.FormatInt(time.Now().UnixMilli(), 10)
	nodeinit := &VRFNode{
			NodeID: nodeID,
			PubKey: pub.Serialize(),
	}

	hash := sha256.Sum256([]byte(seed))
	sign := sec.SignByte(hash[:]) 
	nodeinit.Proof = sign.Serialize()
	nodeinit.Seed = seed
	vrfHash := sha256.Sum256(sign.Serialize())
	nodeinit.Value = new(big.Int).SetBytes(vrfHash[:])
	VRFGlobal = nodeinit

}

func createSelfMembership(id string, addr []string, port string, nodeinfo *VRFNode) *pb.MemberMsg {

	node := &pb.Node{
			NodeID:    id,
			Addrs:     addr,
			Port:      port,
			Publickey: nodeinfo.PubKey,
			Seed:      nodeinfo.Seed,
			Proof:     nodeinfo.Proof,
			Value:     nodeinfo.Value.Bytes(),
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

func WriteTomlFile(fileName string, numMultipath int, myAddrs []string, childAddrs []string) error {
	_ = os.Remove(fileName)
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	var sb strings.Builder

	sb.WriteString("# MP2BTP Configurations\n")
	sb.WriteString(fmt.Sprintf("VERBOSE_MODE = true\n"))
	sb.WriteString(fmt.Sprintf("NUM_MULTIPATH = %d\n", numMultipath))

	formattedIPs := make([]string, len(myAddrs))
	fmt.Println(myAddrs)
	for i, ip := range myAddrs {
		formattedIPs[i] = fmt.Sprintf("\"%s\"", ip)
	}
	sb.WriteString(fmt.Sprintf("MY_IP_ADDRS = [%s]\n", strings.Join(formattedIPs, ", ")))

	sb.WriteString(baseleaderTomlConfig)

	sb.WriteString(fmt.Sprintf(
		"\n[[peer_addr]]  # Offset=0\nAddr = \"%s\"\nNumChild = %d\nChildOffset = 1\n",
		myAddrs[0], len(childAddrs),
	))

	for i, addr := range childAddrs {
		sb.WriteString(fmt.Sprintf(
			"\n[[peer_addr]]  # Offset=%d\nAddr = \"%s\"\nNumChild = 0\nChildOffset = 0\n",
			i+1, addr,
		))
	}

	_, err = f.WriteString(sb.String())
	return err
}

func (w *CONSENSUSNODE) MP2BTPRootOnce(ch chan<- bool, addrs []string) *PuCtrl.PuCtrl {
	mu.Lock()
	defer mu.Unlock()
	ips := w.GetAddresses()

	myaddrs := append([]string(nil), ips...) // myaddrs = 내 IP들 그대로

	// self 주소들을 set으로 만들어서 빠르게 제외
	self := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		self[ip] = struct{}{}
	}

	// peerId := rd.Intn(10) + 1

	// configFile := "./push_root.toml"
	configFile := "/app/transceiver/node/push_root.toml"

	filtered := make([]string, 0, len(addrs))
	seen := make(map[string]struct{}, len(addrs)) // (선택) 중복 제거까지 하고 싶으면

	for _, addr := range addrs {
		if _, isSelf := self[addr]; isSelf {
			continue
		}
		// (선택) 중복 제거
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		filtered = append(filtered, addr)
	}

	err := WriteTomlFile(configFile, 2, myaddrs, filtered)
	if err != nil {
		fmt.Println("❌ TOML 파일 생성 실패:", err)
		ch <- false
		return nil
	}

	fmt.Println("🗂️ TOML Create:", configFile)

	var config PuCtrl.Config
	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		panic(err)
	}

	// pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

	nodeInfo := w.pu.GetNodeInfo(config.PEER_ADDRS)

	// pu.Listen()

	w.pu.Connect(nodeInfo, byte(PuCtrl.PUSH_SESSION))

	return w.pu
}

func (w *CONSENSUSNODE) MP2BTPSubmit(ctx context.Context,ch chan<- *pb.AuditMsg, pu *PuCtrl.PuCtrl, currentPhase pb.AuditMsg_Phases,targetPhase pb.AuditMsg_Phases) {

	sndBuf, err := json.Marshal(w.auditMsg)
	if err != nil {
		fmt.Println("❌ JSON 직렬화 실패:", err)
		// ch <- false
		return
	}
	pu.SendAuditMsg(sndBuf)

	go func() {
		for {
			fmt.Println("Submit", ch, targetPhase)
			select {
			case <-ctx.Done():
				return
			case msg := <-pu.AuditMsgAckChan:

				auditMsg := &pb.AuditMsg{}
				if err := json.Unmarshal(msg.Data, &auditMsg); err != nil {
					fmt.Println(err)
					continue
				}

				if auditMsg.PhaseNum == targetPhase {
					ch <- auditMsg
				} 
			}
		}
	}()
}

func (w *CONSENSUSNODE) MP2BTPChildOnce() *PuCtrl.PuCtrl {

	w.mp2btpOnce.Do(func() {
        fmt.Println("😬 Starting MP2BTP listening")

        configFile := "/app/transceiver/node/push_child.toml"
		// configFile := "./push_child.toml"

        peerId := rd.Intn(10) + 1

        localIP := w.GetAddresses()
		// fmt.Println(localIP)
        // if localIP == "" {
        //     localIP = "127.0.0.1"
        // }

        cfg := new(PuCtrl.Config)
        if _, err := toml.DecodeFile(configFile, cfg); err != nil {
            panic(err)
        }
        cfg.MY_IP_ADDRS = localIP

        f, err := os.Create(configFile)
        if err != nil { panic(err) }
        if err := toml.NewEncoder(f).Encode(cfg); err != nil { panic(err) }

        pu := PuCtrl.CreatePuCtrl(configFile, uint32(peerId))

        // ⭐️ Listen은 노드 lifetime에 1번만
        pu.Listen()

        w.pu = pu
    })

    return  w.pu
}

func (w *CONSENSUSNODE) MP2BTPConsensusListening(pu *PuCtrl.PuCtrl) {
	var msg *packet.AuditDataPacket

	auditMsg := &pb.AuditMsg{}
	auditChan := pu.AuditBroadcaster.Register()

	go func() {
		for {
			msg = <-auditChan

			auditMsg = &pb.AuditMsg{}
			if err := json.Unmarshal(msg.Data, &auditMsg); err != nil {
				fmt.Println(err)
				continue
			}

			switch auditMsg.PhaseNum {
			case pb.AuditMsg_PREPARE:
				w.sendMP2BTPResponse(pb.AuditMsg_AGGREGATED_PREPARE,auditMsg,pu)
			case pb.AuditMsg_COMMIT:
				if w.verifying(auditMsg) {
					w.sendMP2BTPResponse(pb.AuditMsg_AGGREGATED_COMMIT,auditMsg,pu)
				}else {
					// ✅ COMMIT 검증 실패에도 응답하도록
					w.sendMP2BTPResponse(pb.AuditMsg_PHASE_UNSPECIFIED, auditMsg, pu)
				}
			default:
				continue
			}
		}
	}()
}

// ======================= Monitoring =======================
// func monitoring() {
//   counter := prometheus.NewCounter(prometheus.CounterOpts{Namespace: "WDN", Name: "counter_total"})
//   gauge := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "WDN", Name: "gauge_value"})
//   histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
//           Namespace: "WDN", Name: "histogram_value", Buckets: prometheus.LinearBuckets(0, 5, 10),
//   })

//   prometheus.MustRegister(counter, gauge, histogram)

//   go func() {
//           for {
//                   counter.Add(rand.Float64() * 5)
//                   gauge.Add(rand.Float64()*15 - 5)
//                   histogram.Observe(rand.Float64() * 10)
//                   time.Sleep(2 * time.Second)
//           }
//   }()
//   fmt.Println(http.ListenAndServe(prometheusPort, nil))
// }