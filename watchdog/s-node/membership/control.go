package __

import (
	"crypto/sha256"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"sort"
	"strconv"
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

// type WNodeVRF struct {
// 	Node     *pb.Wnode // 기존 노드 정보
// 	PubKey   ecvrf.PublicKey
// 	PrivKey  ecvrf.PrivateKey
// 	VRFOut   *big.Int
// 	VRFProof []byte
// }

var Watchdogs []WatchdogCommittee

var Slog *log.Logger

// this parameter should indicates watchdog pubsub's round index
var round_idx uint32

var Membership = &pb.MemberMsg{} // *pb.MemberMsg
var RwM = new(sync.RWMutex)      // Membership RW Mutex

// Maximum number of committee members
const maxCommitteeSize = 5

func Start() {
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

	round_idx = 0

	go setNextRound()
}

type VRFCandidate struct {
	Node *pb.Wnode
	VRF  *big.Int
}

// setNextRound publishes new membership information in the shard
func setNextRound() {
	bls.Init(bls.BLS12_381)
	// Start round if there is more than one w-node alive
	for {
		cnt := 0
		for _, each := range Membership.Nodes {
			if each != nil {
				fmt.Println(cnt, each)
				cnt++
			}
		}
		if cnt != 0 {
			break
		}
	}

	committeeMsg := &pb.CommitteeMsg{
		Type:     1,
		RoundNum: round_idx,
	}

	// 🔹 각 토픽별로 커미티 선정
	for _, t := range wp2p.WatchdogTopics.Topics {
		RwM.RLock()
		shardMsg := &pb.Shard{ID: t.ID}

		seed := shardMsg.ID + strconv.Itoa(int(committeeMsg.RoundNum))
		var candidates []VRFCandidate

		// 🔹 각 노드 VRF 검증 및 후보 리스트
		for _, each := range Membership.Nodes {
			commMember := &pb.Wnode{
				NodeID:    each.NodeID,
				Addr:      each.Addr,
				Port:      each.Port,
				Publickey: each.Publickey,
				Seed:      each.Seed,
				Proof:     each.Proof,
				Value:     each.Value,
			}

			var pubKey bls.PublicKey
			if err := pubKey.Deserialize(commMember.Publickey); err != nil {
				fmt.Printf("❌ Node %s PublicKey 역직렬화 실패\n", commMember.NodeID)
				continue
			}

			if !VerifyVRF(commMember.Seed, commMember.Proof, pubKey) {
				fmt.Printf("❌ Node %s VRF 검증 실패\n", commMember.NodeID)
				continue
			}

			vrfVal := VRFHash(each.NodeID, seed)
			candidates = append(candidates, VRFCandidate{commMember, vrfVal})
		}
		RwM.RUnlock()

		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].VRF.Cmp(candidates[j].VRF) < 0
		})

		for i := 0; i < len(candidates) && i < maxCommitteeSize; i++ {
			shardMsg.Member = append(shardMsg.Member, candidates[i].Node)
		}

		fmt.Printf("[%s] => selected %d committee members (from %d alive nodes)\n",
			shardMsg.ID, len(shardMsg.Member), len(candidates))

		var leaderNodeID string
		var lowestVRF *big.Int

		for _, member := range shardMsg.Member {
			vrf := VRFHash(member.NodeID, seed+"-leader")

			if lowestVRF == nil || vrf.Cmp(lowestVRF) < 0 ||
				(vrf.Cmp(lowestVRF) == 0 && member.NodeID < leaderNodeID) {
				lowestVRF = vrf
				leaderNodeID = member.NodeID
			}
		}
		fmt.Printf("선정된 리더: %s (VRF=%s)\n", leaderNodeID)
		shardMsg.LeaderID = leaderNodeID

		committeeMsg.Shards = append(committeeMsg.Shards, shardMsg)

		fmt.Printf("=> [ ")
		for _, m := range shardMsg.Member {
			fmt.Printf("%s ", m.NodeID)
		}
	}

	// 🔹 Round index 증가
	round_idx += 1

	// 🔹 WP2P를 통해 합의노드에게 CommitteeMsg 전송
	wp2p.CommitteeMessage(wp2p.Wctx, wp2p.Wtopic, committeeMsg)
	setLatency(10)
}

// VRF 검증 예시
func VerifyVRF(seed string, proof []byte, pubKey bls.PublicKey) bool {
	var sig bls.Sign
	if err := sig.Deserialize(proof); err != nil {
		return false
	}

	seedHash := sha256.Sum256([]byte(seed))
	return sig.FastAggregateVerify([]bls.PublicKey{pubKey}, seedHash[:])
}

// 🔹 VRFHash 계산
func VRFHash(nodeID, seed string) *big.Int {
	data := []byte(nodeID + seed)
	hash := sha256.Sum256(data)
	return new(big.Int).SetBytes(hash[:])
}

// // setNextRound publishes new membership information in the shard
// func setNextRound() {
// 	// Start round if there is more than one w-node in alive state
// 	for {
// 		cnt := 0
// 		for _, each := range Membership.Nodes {
// 			if each != nil {
// 				fmt.Println(cnt, each)
// 				cnt = cnt + 1
// 			}
// 		}
// 		if cnt != 0 {
// 			fmt.Println(cnt)
// 			break
// 		}
// 	}

// 	committeeMsg := &pb.CommitteeMsg{}
// 	committeeMsg.Type = 1
// 	committeeMsg.RoundNum = round_idx

// 	// LOGIC: For each subscribed topic, use VRF to deterministically and fairly select a subset of W-Nodes as the committee,
// 	// and also use VRF to select a leader node from within the committee.
// 	for _, t := range wp2p.WatchdogTopics.Topics {
// 		RwM.RLock()

// 		shardMsg := &pb.Shard{}
// 		shardMsg.ID = t.ID
// 		seed := []bytes(shardMsg.ID + strconv.Itoa(int(committeeMsg.RoundNum)))

// 		var candidates []WNodeVRF

// 		for _, each := range Membership.Nodes {
// 			if each.Consensusstatus {
// 				sk, pk, err := ecvrf.P256Sha256Tau.GenerateKey(rand.Reader)
// 				if err != nil {
// 					log.Fatal(err)
// 				}

// 				proof, output := sk.Prove(seed)
// 				outputInt := new(big.Int).SetBytes(output)

// 				wn := &pb.Wnode{
// 					NodeID:    each.NodeID,
// 					Addr:      each.Addr,
// 					Port:      each.Port,
// 					Publickey: each.Publickey,
// 					Channel:   each.Channel,
// 					VRFSeed:   seed,
// 					VRFProof:  proof,
// 				}
// 				candidates = append(candidates, WNodeVRF{
// 					Node:     wn,
// 					PubKey:   pk,
// 					PrivKey:  sk,
// 					VRFOut:   outputInt,
// 					VRFProof: proof,
// 				})
// 			}
// 		}
// 		RwM.RUnlock()

// 		sort.Slice(candidates, func(i, j int) bool {
// 			return candidates[i].VRFOut.Cmp(candidates[j].VRFOut) < 0
// 		})

// 		for i := 0; i < len(candidates) && i < maxCommitteeSize; i++ {
// 			shardMsg.Member = append(shardMsg.Member, candidates[i].Node)
// 		}

// 		leaderID := candidates[0].Node.NodeID
// 		shardMsg.LeaderID = leaderID

// 		fmt.Printf("[%s] Committee Members: ", shardMsg.ID)
// 		for _, m := range shardMsg.Member {
// 			fmt.Printf("%s ", m.NodeID)
// 		}
// 		fmt.Printf(" | Leader: %s\n", shardMsg.LeaderID)

// 		committeeMsg.Shards = append(committeeMsg.Shards, shardMsg)
// 	}
// 	round_idx += 1
// 	wp2p.CommitteeMessage(wp2p.Wctx, wp2p.Wtopic, committeeMsg)

// 	setLatency(10)
// }

func makeNodeUnique(s []*pb.Wnode) []*pb.Wnode {
	res := make([]*pb.Wnode, 0)
	flag := false
	for _, val1 := range s {
		for _, val2 := range res {
			if val1.NodeID == val2.NodeID {
				fmt.Println("SAME ADDRESS! -> ", val1.NodeID, val2.NodeID)
				flag = true
				break
			}
		}
		if !flag {
			res = append(res, val1)
		}
	}
	return res
}

func setLatency(delta uint32) {
	timer := time.NewTimer(time.Second * time.Duration(delta))
	go func() {
		<-timer.C
		setNextRound()
	}()
}
