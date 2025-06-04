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
)

type WatchdogCommittee struct {
	ChannelID string
	Members   []pb.Committee
}

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

// Placeholder for VRF function (to be replaced with actual VRF implementation including proof).
// Will be migrated to open source later.
func VRFHash(nodeID, seed string) *big.Int {
	hash := sha256.Sum256([]byte(nodeID + seed))
	return new(big.Int).SetBytes(hash[:])
}

// setNextRound publishes new membership information in the shard
func setNextRound() {
	// Start round if there is more than one w-node in alive state
	for {
		cnt := 0
		for _, each := range Membership.Nodes {
			if each.Alive {
				cnt = cnt + 1
			}
		}
		if cnt != 0 {
			break
		}
	}

	committeeMsg := &pb.CommitteeMsg{}
	committeeMsg.Type = 1
	committeeMsg.RoundNum = round_idx

	// LOGIC: For each subscribed topic, use VRF to deterministically and fairly select a subset of W-Nodes as the committee,
	// and also use VRF to select a leader node from within the committee.
	for _, t := range wp2p.WatchdogTopics.Topics {
		RwM.RLock()

		shardMsg := &pb.Shard{}
		shardMsg.ID = t.ID

		seed := shardMsg.ID + string(committeeMsg.RoundNum) //
		fmt.Println(seed)
		var candidates []struct {
			Node *pb.Wnode
			VRF  *big.Int
		}

		for _, each := range Membership.Nodes {
			if each.Alive {

				commMember := &pb.Wnode{}
				commMember.NodeID = each.NodeID
				commMember.Addr = each.Addr
				commMember.Port = each.Port
				commMember.Publickey = each.Publickey
				commMember.Channel = each.Channel

				vrfVal := VRFHash(each.NodeID, seed)
				candidates = append(candidates, struct {
					Node *pb.Wnode
					VRF  *big.Int
				}{commMember, vrfVal})
			}
		}

		RwM.RUnlock()

		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].VRF.Cmp(candidates[j].VRF) < 0
		})

		for i := 0; i < len(candidates) && i < maxCommitteeSize; i++ {
			shardMsg.Member = append(shardMsg.Member, candidates[i].Node)
		}

		fmt.Printf("[%s] => selected %d committee members (from %d alive nodes)\n", shardMsg.ID, len(shardMsg.Member), len(candidates))

		var leaderNodeID string
		var lowestVRF *big.Int

		for _, member := range shardMsg.Member {
			vrf := VRFHash(member.NodeID, seed+"-leader")

			if lowestVRF == nil || vrf.Cmp(lowestVRF) < 0 {
				lowestVRF = vrf
				leaderNodeID = member.NodeID
			}
		}

		shardMsg.LeaderID = leaderNodeID

		committeeMsg.Shards = append(committeeMsg.Shards, shardMsg)

		fmt.Printf("=> [ ")
		for _, m := range shardMsg.Member {
			fmt.Printf("%s ", m.NodeID)
		}
		fmt.Println("] Leader: %s\n", shardMsg.LeaderID)
	}
	round_idx += 1
	wp2p.CommitteeMessage(wp2p.Wctx, wp2p.Wtopic, committeeMsg)

	setLatency(10)
}

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
