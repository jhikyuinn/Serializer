package __

import (
	"context"
	"fmt"
	"net"
	"os"

	// "reflect"
	pb "auditchain/msg"
	"encoding/json"

	"github.com/gogo/protobuf/proto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

var handles = map[string]string{}

// DiscoveryServiceTag is used in our mDNS advertisements to discover other chat peers.
const DiscoveryServiceTag = "pubsub-chat-example"

/* [Gossib Msg/Blk] ------------------------------------------------------------------------------------*/
var GMsg pb.GossipMsg

var CMsg pb.CommitteeMsg

func JoinNetwork(ctx context.Context, ps *pubsub.PubSub, pubsubTopic string, topicType int) (*pubsub.Topic, error) {
	// join the pubsub topic
	topic, err := ps.Join(pubsubTopic)
	if err != nil {
		return nil, err
	}

	// and subscribe to it
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	if topicType == 1 {
		go committeeHandler(ctx, sub)
	} else {
		go pubsubHandler(ctx, sub)
	}

	return topic, nil
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're connected,
// the PubSub system will automatically start interacting with them if they also
// support PubSub.
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		fmt.Printf("error connecting to peer %s: %s\n", pi.ID, err)
	}
}

// setupDiscovery creates an mDNS discovery service and attaches it to the libp2p Host.
// This lets us automatically discover peers on the same LAN and connect to them.
func setupDiscovery(h host.Host) error {
	// setup mDNS discovery to find local peers
	s := mdns.NewMdnsService(h, DiscoveryServiceTag, &discoveryNotifee{h: h})
	return s.Start()
}

func pubsubMessageHandler(msg *SendMessage) {
	json.Unmarshal(msg.Data, &GMsg)

}

func pubsubUpdateHandler(id peer.ID, msg *UpdatePeer) {
	oldHandle, ok := handles[id.String()]
	if !ok {
		oldHandle = id.ShortString()
	}
	handles[id.String()] = string(msg.UserHandle)
	fmt.Printf("%s -> %s\n", oldHandle, msg.UserHandle)
}

func pubsubHandler(ctx context.Context, sub *pubsub.Subscription) {
	defer sub.Cancel()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		req := &Request{}
		err = proto.Unmarshal(msg.Data, req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		switch *req.Type {
		case Request_SEND_MESSAGE:
			pubsubMessageHandler(req.SendMessage)
		case Request_UPDATE_PEER:
			pubsubUpdateHandler(msg.GetFrom(), req.UpdatePeer)
		}
	}
}

func committeeMessageHandler(id peer.ID, msg *SendMessage) {
	json.Unmarshal(msg.Data, &CMsg)

	// 📨📨 Recved CommitteeMsg including Type-1 Round Msg
	if CMsg.Type == 1 {
		sndBuf, _ := json.Marshal(CMsg)
		for {
			conn, err := net.Dial("tcp", "localhost:4242")
			if err != nil {
				// fmt.Println("Failed to Dial : ", err)
				continue
			}
			defer conn.Close()
			_, err = conn.Write(sndBuf)
			if err != nil {
				fmt.Println("Failed to write data : ", err)
			}
			err = conn.Close()
			if err != nil {
				fmt.Println("Failed to close : ", err)
			}
			break
		}
	}
}

func committeeHandler(ctx context.Context, sub *pubsub.Subscription) {
	defer sub.Cancel()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		req := &Request{}
		err = proto.Unmarshal(msg.Data, req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		switch *req.Type {
		case Request_SEND_MESSAGE:
			committeeMessageHandler(msg.GetFrom(), req.SendMessage)
		}
	}
}
