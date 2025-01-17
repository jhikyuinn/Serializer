package __

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

type WatchdogPubsub struct {
	ctx   context.Context
	topic *pubsub.Topic
	ps    *pubsub.PubSub
	h     host.Host
}

type Channels struct {
	ID []string `json:"id"`
}

type WTopics struct {
	Topics []WatchdogTopic
}

type WatchdogTopic struct {
	ID    string
	Topic *pubsub.Topic
}

// discoveryNotifee gets notified when we find a new peer via mDNS discovery
type discoveryNotifee struct {
	h host.Host
}

var Wctx context.Context
var Wtopic *pubsub.Topic

var WatchdogTopics WTopics

var JoinedChannels Channels

func handleStream(s network.Stream) {
	// Create a buffer stream for non-blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	go readData(rw)
	go writeData(rw)

	// stream 's' will stay open until you close it (or the other side closes it).
}

func readData(rw *bufio.ReadWriter) {
	for {
		str, _ := rw.ReadString('\n')
		if str == "" {
			return
		}
		if str != "\n" {
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)
		}

	}
}

func writeData(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			log.Println(err)
			return
		}

		rw.WriteString(fmt.Sprintf("%s\n", sendData))
		rw.Flush()
	}
}

var certPath string

func init() {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Failed to get current frame")
	}

	certPath = path.Dir(filename)
}

// AddRootCA adds the root CA certificate to a cert pool
func AddRootCA(certPool *x509.CertPool) {
	// caCertPath := path.Join("./src/ca.pem")
	caCertPath := path.Join(certPath, "../testdata/ca.pem")
	caCertRaw, err := os.ReadFile(caCertPath)
	if err != nil {
		panic(err)
	}
	if ok := certPool.AppendCertsFromPEM(caCertRaw); !ok {
		panic("Could not add root ceritificate to pool.")
	}
}

func (wp *WatchdogPubsub) Start(processorIP *string) {

	wp.ctx = context.Background()

	r := rand.Reader

	h, err := makeHost(4041, r)
	if err != nil {
		wp.ctx.Done()
		panic(err)
	}
	wp.h = h
	fmt.Printf("Peer ID: %s\n", wp.h.ID())

	wp.startPeer(handleStream)

	// create a new PubSub service using the GossipSub router
	// ps, err := pubsub.NewGossipSub(wp.ctx, h)
	wp.ps, err = pubsub.NewGossipSub(wp.ctx, h)
	if err != nil {
		wp.ctx.Done()
		panic(err)
	}

	// setup local mDNS discovery
	if err := setupDiscovery(wp.h); err != nil {
		wp.ctx.Done()
		panic(err)
	}

	// watchdog/group/w-node topic publishes committee information
	// which committee for the channel, and who is the leader node
	wp.topic, err = joinNetwork(wp.ctx, wp.ps, "watchdog/group/w-node")
	if err != nil {
		wp.ctx.Done()
		panic(err)
	}

	Wtopic = wp.topic

	// setting TLS for QUIC
	pool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatal(err)
	}
	AddRootCA(pool)

	var qconf quic.Config
	roundTripper := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{
			RootCAs:            pool,
			InsecureSkipVerify: true,
		},
		QuicConfig: &qconf,
	}

	defer roundTripper.Close()
	hclient := &http.Client{
		Transport: roundTripper,
	}

	for {
		processorAddr := "https://" + *processorIP + ":8000/fabric/channels"

		rsp, err := hclient.Get(processorAddr)
		if err != nil {
			fmt.Println(err)
			continue
		}

		body := &bytes.Buffer{}
		_, err = io.Copy(body, rsp.Body)
		if err != nil {
			log.Fatal(err)
		}

		var newChannels Channels
		b, _ := ioutil.ReadAll(body)
		err = json.Unmarshal(b, &newChannels)
		if err != nil {
			fmt.Println("Unmarshal error:", err)
		}
		wp.openPubsubNetwork(&newChannels)

		time.Sleep(10 * time.Second)
	}
}

// openPubsubNetwork opens new pubsub network channel if
// there is no opened network
func (wp *WatchdogPubsub) openPubsubNetwork(newChannels *Channels) {
	for _, nc := range newChannels.ID {
		if !existChannel(nc) {
			fmt.Println("Opening network:", nc)
			newTopic, err := joinNetwork(wp.ctx, wp.ps, "watchdog/"+nc)
			if err != nil {
				fmt.Println("Joining network error:", err)
			}

			new := &WatchdogTopic{
				ID:    nc,
				Topic: newTopic,
			}

			WatchdogTopics.Topics = append(WatchdogTopics.Topics, *new)
		}
	}
}

func existChannel(channel string) bool {
	for _, jc := range JoinedChannels.ID {
		if jc == channel {
			return true
		}
	}
	JoinedChannels.ID = append(JoinedChannels.ID, channel)
	return false
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

func (wp *WatchdogPubsub) startPeer(streamHandler network.StreamHandler) {
	// Set a function as stream handler.
	// This function is called when a peer connects, and starts a stream with this protocol.
	// Only applies on the receiving side.
	wp.h.SetStreamHandler("/chat/1.0.0", streamHandler)

	// Let's get the actual TCP port from our listen multiaddr, in case we're using 0 (default; random available port).
	var port string
	for _, la := range wp.h.Network().ListenAddresses() {
		if p, err := la.ValueForProtocol(multiaddr.P_TCP); err == nil {
			port = p
			break
		}
	}

	if port == "" {
		log.Println("was not able to find actual local port")
		return
	}

	log.Println("Waiting for incoming connection")
	log.Println()
}
