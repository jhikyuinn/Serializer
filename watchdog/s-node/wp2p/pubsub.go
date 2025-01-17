package __

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	pb "validator/msg"

	"github.com/gogo/protobuf/proto"
	"github.com/libp2p/go-libp2p/core/peer"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

var handles = map[string]string{}

// DiscoveryServiceTag is used in our mDNS advertisements to discover other chat peers.
const DiscoveryServiceTag = "pubsub-chat-example"

var GMsg pb.GossipMsg

func joinNetwork(ctx context.Context, ps *pubsub.PubSub, pubsubTopic string) (*pubsub.Topic, error) {
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

	go pubsubHandler(ctx, sub)

	return topic, nil
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're connected,
// the PubSub system will automatically start interacting with them if they also
// support PubSub.
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("discovered new peer %s\n", pi.ID)
	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		fmt.Printf("error connecting to peer %s: %s\n", pi.ID, err)
	}

	// var connectedPeers []
	fmt.Println("Connected peer list:", n.h.Peerstore().Peers())
}

// setupDiscovery creates an mDNS discovery service and attaches it to the libp2p Host.
// This lets us automatically discover peers on the same LAN and connect to them.
func setupDiscovery(h host.Host) error {
	// setup mDNS discovery to find local peersf
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
