package __

import (
	"crypto/sha256"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"sort"
	"sync"
	"time"
	pb "validator/msg"
	wp2p "validator/wp2p"

	"github.com/herumi/bls-eth-go-binary/bls"
)

type WatchdogCommittee struct {
	ChannelID string
	Members   []pb.Committee
}

type VRFCandidate struct {
	Node *pb.Wnode
	VRF  *big.Int
}

var Watchdogs []WatchdogCommittee

var Slog *log.Logger

var Membership = &pb.MemberMsg{} // *pb.MemberMsg
var RwM = new(sync.RWMutex)      // Membership RW Mutex


type RoundRequest struct {
	Node *pb.Node
	VRF  *big.Int
}

var (
	requestQueue    = make(map[uint32][]RoundRequest)
	queueLock       sync.Mutex
	roundTimers     = make(map[uint32]*time.Timer)
	roundTimerSet   = make(map[uint32]bool)
	roundCommitted  = make(map[uint32]bool)

	currentRound    uint32 = 0 // 현재 수집 중인 라운드
	roundActive     bool   = false
	minCommitteeSize       = 2
	maxCommitteeSize       = 20
)


// BLS 연산 전에 꼭 내부 C 포인터 구조 초기화
func Init() {
    bls.Init(bls.BLS12_381)
    bls.SetETHmode(bls.EthModeDraft07)
}

func Start() {
	Init()
	LOGFILE := path.Join("./snode.log")
	f, err := os.OpenFile(LOGFILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()

	Slog = log.New(f, "SLog ", log.LstdFlags)
	Slog.Println("Start")

	Watchdogs = []WatchdogCommittee{}
}

func startCommitteeTimer(round uint32) {
    startedAt := time.Now()
    fmt.Printf("⏱️ timer started round=%d at=%s\n", round, startedAt.Format(time.RFC3339Nano))

    timer := time.AfterFunc(3*time.Second, func() {
        firedAt := time.Now()
        fmt.Printf("🔥 timer fired round=%d at=%s elapsed=%s\n",
            round, firedAt.Format(time.RFC3339Nano), firedAt.Sub(startedAt))

        queueLock.Lock()
        defer queueLock.Unlock()

        if roundCommitted[round] { return }
        if len(requestQueue[round]) == 0 {
            fmt.Printf("⚠️ No nodes received for round %d\n", round)
            roundActive = false
            return
        }
        go BuildCommittee(round)
    })
    roundTimers[round] = timer
}



func HandleMembershipRequest(req *pb.MemberMsg) {
	queueLock.Lock()
	defer queueLock.Unlock()

	if !roundActive {
		currentRound = uint32(time.Now().UnixNano() / int64(time.Millisecond))
		roundActive = true
	}

	// 현재 라운드의 노드 ID 중복 확인용 맵 생성
	existing := make(map[string]bool)
	for _, r := range requestQueue[currentRound] {
		existing[r.Node.NodeID] = true
	}

	for _, node := range req.Nodes {
		if existing[node.NodeID] {
			fmt.Printf("⚠️ Duplicate node %s ignored for round %d\n", node.NodeID, currentRound)
			continue
		}

		var pubKey bls.PublicKey
		if err := pubKey.Deserialize(node.Publickey); err != nil {
			fmt.Printf("❌ Node %s public key deserialize failed\n", node.NodeID)
			continue
		}

		vrfVal, ok := VerifyVRF(node.Seed, node.Proof, pubKey)
		if !ok {
			fmt.Printf("❌ Node %s VRF verification failed\n", node.NodeID)
			continue
		}

		// ✅ 중복이 아니면 추가
		requestQueue[currentRound] = append(requestQueue[currentRound], RoundRequest{
			Node: node,
			VRF:  vrfVal,
		})
		existing[node.NodeID] = true
	}

	if len(requestQueue[currentRound]) >= minCommitteeSize && !roundTimerSet[currentRound] {
		startCommitteeTimer(currentRound)
		roundTimerSet[currentRound] = true
	}
}



func BuildCommittee(round uint32) {
	queueLock.Lock()

	if roundCommitted[round] {
		queueLock.Unlock()
		return
	}
	roundCommitted[round] = true
	requests := requestQueue[round]
	delete(requestQueue, round)
	cancelRoundTimer(round)
	queueLock.Unlock()

	if len(requests) == 0 {
		fmt.Printf("⚠️ No valid requests for round %d\n", round)
		return
	}

	for _, topic := range wp2p.WatchdogTopics.Topics {
		
		var candidates []RoundRequest
		candidates = append(candidates, requests...)

		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].VRF.Cmp(candidates[j].VRF) < 0
		})

		limit := len(candidates)
		if limit > maxCommitteeSize {
			limit = maxCommitteeSize
		}

		shardMsg := &pb.Shard{
			ID:       topic.ID,
			LeaderID: candidates[0].Node.NodeID,
		}

		for i := 0; i < limit; i++ {
			shardMsg.Member = append(shardMsg.Member, &pb.Wnode{
				NodeID:    candidates[i].Node.NodeID,
				Addrs:      candidates[i].Node.Addrs,
				Port:      candidates[i].Node.Port,
				Publickey: candidates[i].Node.Publickey,
				Seed:      candidates[i].Node.Seed,
				Proof:     candidates[i].Node.Proof,
				Value:     candidates[i].Node.Value,
			})
			fmt.Println(candidates[i].Node.NodeID)
		}

		committeeMsg := &pb.CommitteeMsg{
			Type:     1,
			RoundNum: round,
			Shards:   []*pb.Shard{shardMsg},
		}

		wp2p.CommitteeMessage(wp2p.Wctx, wp2p.Wtopic, committeeMsg)
		fmt.Printf("✅ Committee created for round %d and topic %s\n", round, topic.ID)
	}

	queueLock.Lock()
	roundActive = false
	currentRound = uint32(time.Now().UnixNano() / int64(time.Millisecond)) // 다음 라운드 번호 갱신
	roundActive = true
	startCommitteeTimer(currentRound)
	roundTimerSet[currentRound] = true
	queueLock.Unlock()
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

func cancelRoundTimer(round uint32) {
	if timer, ok := roundTimers[round]; ok {
		timer.Stop()
		delete(roundTimers, round)
	}
	delete(roundTimerSet, round)
}

