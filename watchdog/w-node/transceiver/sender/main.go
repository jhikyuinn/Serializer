package main

import (
	"time"
	// "io"

	// "github.com/golang/protobuf/proto"

	pb "auditchain/msg"
)

const addr = "localhost:4242"

type ConsensusMsg struct {
	msgType   int32
	memberMsg *pb.MemberMsg `msgType: 1`
	auditMsg  *pb.AuditMsg  `msgType: 2`
}

func main() {
	start()
}

func start() {
	msg := ConsensusMsg{
		msgType:   100,
		memberMsg: &pb.MemberMsg{},
		auditMsg:  &pb.AuditMsg{},
	}
	// [recv] block msg listening from HLF
	go msg.BlkListening()

	// [recv] consensus msg. listening from leader
	go msg.ConsListening()

	// [snd] peer information every 5 second
	for {
		go msg.AliveMsg()
		time.Sleep(time.Second * 5)
	}
}

func (c *ConsensusMsg) BlkListening()  {}
func (c *ConsensusMsg) ConsListening() {}
func (c *ConsensusMsg) Listening()     {}
