package __

import (
	"fmt"
	"log"
	"os"
	"path"
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

func FormCommittee() {
	fmt.Println("Formming committees ...")

	RwM.Lock()
	defer RwM.Unlock()

	committeeExist := false

	if len(Membership.Nodes) == 0 {
		return
	}

	for _, jc := range wp2p.JoinedChannels.ID {
		aliveNodes := []pb.Committee{}

		// LOGIC: append one node for each channel for now
		nodes := Membership.Nodes
		for _, each := range nodes {
			if each.Alive {
				aliveNode := pb.Committee{}
				aliveNode.NodeID = each.NodeID
				aliveNode.Addr = each.Addr
				aliveNode.Port = each.Port
				aliveNode.Publickey = each.Publickey

				aliveNodes = append(aliveNodes, aliveNode)
			}
		}

		// randomly select committee members within alive nodes
		var mem []pb.Committee
		for i := 0; i < len(aliveNodes); i++ {
			mem = append(mem, aliveNodes[i])

			if len(mem) >= 10 {
				break
			}
		}

		if len(Watchdogs) == 0 {
			Watchdogs = append(Watchdogs, WatchdogCommittee{
				ChannelID: jc,
				Members:   mem,
			})

			fmt.Println("New joined channel:", jc)
			continue
		}

		for i, comm := range Watchdogs {
			if jc == comm.ChannelID {
				Watchdogs[i].Members = mem
				committeeExist = true
			}
		}

		if !committeeExist {
			Watchdogs = append(Watchdogs, WatchdogCommittee{
				ChannelID: jc,
				Members:   mem,
			})
			fmt.Println("New joined channel:", jc)
			continue
		}
	}

	for _, w := range Watchdogs {
		fmt.Printf("[%s]", w.ChannelID)
		for _, n := range w.Members {
			fmt.Printf("=> %s ", n.NodeID)
		}
		fmt.Println()
	}
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

	// LOGIC: Repeat as many topics currently subscribed and randomly select among W-Nodes to deliver channel information to those nodes
	for _, t := range wp2p.WatchdogTopics.Topics {
		RwM.RLock()

		shardMsg := &pb.Shard{}
		shardMsg.ID = t.ID

		nodes := Membership.Nodes
		for _, each := range nodes {
			if each.Alive {

				commMember := &pb.Wnode{}
				commMember.NodeID = each.NodeID
				commMember.Addr = each.Addr
				commMember.Port = each.Port
				commMember.Publickey = each.Publickey
				commMember.Channel = each.Channel

				shardMsg.Member = append(shardMsg.Member, commMember)
			} else {
				fmt.Println("DIED:", each.NodeID)
			}
		}

		RwM.RUnlock()

		shardMsg.Member = makeNodeUnique(shardMsg.Member)

		fmt.Printf("[%s] => %d members\n", shardMsg.ID, len(shardMsg.Member))

		if len(shardMsg.Member) == 0 {
			break
		}

		// select leader node for the channel (i.e., dApp)
		shardMsg.LeaderID = shardMsg.Member[len(shardMsg.Member)-1].NodeID

		committeeMsg.Shards = append(committeeMsg.Shards, shardMsg)

		fmt.Printf("=> [ ")
		for _, m := range shardMsg.Member {
			fmt.Printf("%s ", m.NodeID)
		}
		fmt.Println("]")
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
