package __

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

var Wctx context.Context
var Wtopic *pubsub.Topic

var Shard []*pubsub.Topic

var ps *pubsub.PubSub

var Host string

// discoveryNotifee gets notified when we find a new peer via mDNS discovery
type discoveryNotifee struct {
	h host.Host
}

func Start(isLeader bool, dest string, sourcePort int) {

	ctx, _ := context.WithCancel(context.Background())
	Wctx = ctx

	r := rand.Reader

	// create a new libp2p Host that listens on a TCP port

	hotty, err := makeHost(sourcePort, r)
	if err != nil {
		panic(err)
	}
	Host = hotty.ID().String()

	fmt.Printf("Peer ID: %s\n", hotty.ID())

	_, err = startPeerAndConnect(hotty, dest)
	if err != nil {
		log.Println(err)
	}

	// create a new PubSub service using the GossipSub router
	ps, err = pubsub.NewGossipSub(ctx, hotty)
	if err != nil {
		panic(err)
	}

	// setup local mDNS discovery
	if err := setupDiscovery(hotty); err != nil {
		panic(err)
	}

	topic, err := JoinNetwork(ctx, ps, "watchdog/group/w-node", 1)
	if err != nil {
		panic(err)
	}

	Wtopic = topic
}

func makeHost(port int, randomness io.Reader) (host.Host, error) {
	// Creates a new RSA key pair for this host.
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, randomness)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// 0.0.0.0 will listen on any interface device.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	return libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)
}

func startPeerAndConnect(h host.Host, destination string) (*bufio.ReadWriter, error) {
	log.Println("This node's multiaddresses:")
	for _, la := range h.Addrs() {
		log.Printf(" - %v\n", la)
	}
	log.Println()

	// Turn the destination into a multiaddr.
	maddr, err := multiaddr.NewMultiaddr(destination)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// Extract the peer ID from the multiaddr.
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// Add the destination's peer multiaddress in the peerstore.
	// This will be used during connection and stream creation by libp2p.
	h.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)

	// Start a stream with the destination.
	// Multiaddress of the destination peer is fetched from the peerstore using 'peerId'.
	s, err := h.NewStream(context.Background(), info.ID, "/chat/1.0.0")
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("Established connection to destination")

	// Create a buffered stream so that read and writes are non-blocking.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	return rw, nil
}

func JoinShard(channel string) {
	ctx, _ := context.WithCancel(context.Background())

	topic, err := JoinNetwork(ctx, ps, "watchdog/"+channel, 2)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Shard = topic
	Shard = append(Shard, topic)
	for _, s := range Shard {
		fmt.Println(s)
	}
}
