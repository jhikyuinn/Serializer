package __

import (
	//"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	//"strings"
	"encoding/json"
	pb "validator/msg"

	"github.com/gogo/protobuf/proto"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

// WDNMessage propagates Msg/Blk to all w-nodes in WDN
func WDNMessage(ctx context.Context, topic *pubsub.Topic, data *pb.GossipMsg) {
	msg, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	msgId := make([]byte, 10)
	_, err = rand.Read(msgId)
	defer func() {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	if err != nil {
		return
	}
	now := time.Now().Unix()
	req := &Request{
		Type: Request_SEND_MESSAGE.Enum(),
		SendMessage: &SendMessage{
			Id:      msgId,
			Data:    msg,
			Created: &now,
		},
	}
	msgBytes, err := proto.Marshal(req)
	if err != nil {
		return
	}

	err = topic.Publish(ctx, msgBytes)
	if err != nil {
		fmt.Println(err)
	}
}

// CommitteeMessage broadcasts committee information for all W-Nodes
func CommitteeMessage(ctx context.Context, topic *pubsub.Topic, data *pb.CommitteeMsg) {
	msg, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	msgId := make([]byte, 10)
	_, err = rand.Read(msgId)
	defer func() {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	if err != nil {
		return
	}
	now := time.Now().Unix()
	req := &Request{
		Type: Request_SEND_MESSAGE.Enum(),
		SendMessage: &SendMessage{
			Id:      msgId,
			Data:    msg,
			Created: &now,
		},
	}
	msgBytes, err := proto.Marshal(req)
	if err != nil {
		return
	}

	err = topic.Publish(ctx, msgBytes)
	if err != nil {
		fmt.Println(err)
	}
}

func updatePeer(ctx context.Context, topic *pubsub.Topic, id peer.ID, handle string) {
	oldHandle, ok := handles[id.String()]
	if !ok {
		oldHandle = id.ShortString()
	}
	handles[id.String()] = handle

	req := &Request{
		Type: Request_UPDATE_PEER.Enum(),
		UpdatePeer: &UpdatePeer{
			UserHandle: []byte(handle),
		},
	}
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	err = topic.Publish(ctx, reqBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	fmt.Printf("%s -> %s\n", oldHandle, handle)
}
