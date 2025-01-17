package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"encoding/json"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/examples/data"

	tx "validator/fabric-conn"
	msp "validator/membership"
	mp "validator/mempool"
	pb "validator/msg"
	wp2p "validator/wp2p"
)

type SNODE struct {
	IP string
}

var (
	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "", "The TLS cert file")
	keyFile  = flag.String("key_file", "", "The TLS key file")
)

var myAddr string

type server struct {
	pb.UnimplementedMembershipServiceServer
}

func GetExternalIP() {
	l, err := net.Listen("tcp", myAddr+":11730")
	if nil != err {
		log.Println(err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if nil != err {
			log.Println(err)
			continue
		}
		go ConnHandler(conn)
	}
}

func ConnHandler(conn net.Conn) {
	recvBuf := make([]byte, 64)
	defer conn.Close()

	// the size of recv data is about 5 byte
	n, err := conn.Read(recvBuf)
	if nil != err {
		if io.EOF == err {
			log.Printf("connection is closed from client; %v", conn.RemoteAddr().String())
			return
		}
		log.Printf("fail to receive data; err: %v", err)
		return
	}
	data := recvBuf[:n]
	id := new(string)

	json.Unmarshal(data, id)

	// Change internal IP Address to external IP address
	fmt.Println("Connected NODE_ID:", *id)

	msp.RwM.Lock()
	defer msp.RwM.Unlock()

	for idx, each := range msp.Membership.Nodes {
		if each.NodeID == *id {
			strtmp := conn.RemoteAddr().String()
			slice := strings.Split(strtmp, ":")
			msp.Membership.Nodes[idx].Addr = slice[0]
			break
		}
	}
}

func nodeExist(id string) bool {
	for _, mem := range msp.Membership.Nodes {
		if mem.NodeID == id {
			return true
		}
	}
	return false
}

func FabricHandler(conn net.Conn) {
	recvBuf := make([]byte, 8192)

	for {
		n, err := conn.Read(recvBuf)
		if nil != err {
			if io.EOF == err {
				log.Printf("connection is closed from client; %v", conn.RemoteAddr().String())
				return
			}
			log.Printf("fail to receive data; err: %v", err)
			return
		}
		data := recvBuf[:n]

		if n < len(recvBuf) {
			TxRecv := &mp.Fabric{}
			json.Unmarshal(data, TxRecv)
			// Keep fabric transactions in off-chain storage
			tx.Insert(TxRecv)
		} else {
			log.Printf("data overlength")
		}
	}
}

// Check heartbeat msg and update
func UpdateMembership() {
	for {
		// change flag to false
		checkAliveNode()
		// wait for time more than exchanging Membership msg
		time.Sleep(time.Second * 40000)
		// removw the w-node that flag is still false
		removeNode()
	}
}

// Check node's status
func checkAliveNode() {
	msp.RwM.Lock()
	for idx := range msp.Membership.Nodes {
		msp.Membership.Nodes[idx].Alive = false
	}
	msp.RwM.Unlock()
}

func removeNode() {
	msp.RwM.Lock()
	for i := 0; i < len(msp.Membership.Nodes); i++ {
		if !msp.Membership.Nodes[i].Alive {
			slice := &pb.MemberMsg{}
			slice.Nodes = append(msp.Membership.Nodes[:i], msp.Membership.Nodes[i+1:]...)
			msp.Membership.Nodes = slice.Nodes
			i = i - 1
		}
	}
	msp.RwM.Unlock()
}

func InsertNode(node *pb.Node) {
	msp.RwM.Lock()
	flag := false
	for _, each := range msp.Membership.Nodes {
		if each.NodeID == node.NodeID {
			flag = true
			if !each.Alive {
				each.Alive = true
				break
			}
		}
	}
	if !flag {
		msp.Membership.Nodes = append(msp.Membership.Nodes, node)
	}

	fmt.Println("Current Members:", msp.Membership.Nodes)
	msp.RwM.Unlock()
}

// GRPC SERVICE
func (s *server) GetMembership(ctx context.Context, msg *pb.MemberMsg) (*pb.MemberMsg, error) {
	node := &pb.Node{
		NodeID:    msg.Nodes[0].NodeID,
		Addr:      msg.Nodes[0].Addr,
		Publickey: msg.Nodes[0].Publickey,
		Alive:     msg.Nodes[0].Alive,
		Channel:   msg.Nodes[0].Channel,
	}

	if !nodeExist(node.NodeID) {
		InsertNode(node)
	}

	return msp.Membership, nil
}

func main() {

	snodeAddr := flag.String("snode", "117.16.244.33", "")
	processor := flag.String("processor", "117.16.244.33", "")
	flag.Parse()

	myAddr = *snodeAddr

	snode := SNODE{
		IP: myAddr,
	}
	fmt.Println("Host IP:", snode.IP)

	// run libp2p that opens watchdog pubsub channel
	wps := wp2p.WatchdogPubsub{}
	go wps.Start(processor)

	// (Goroutine) Prepare for the next round based on the membership of the w-nodes
	msp.Start()

	// (Goroutine) TCP Listener for updating IP Address of w-node
	lis, err := net.Listen("tcp", myAddr+":16220")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go GetExternalIP()

	// (Goroutine) Exchanging membership message via GRPC
	var opts []grpc.ServerOption
	if *tls {
		if *certFile == "" {
			*certFile = data.Path("x509/server_cert.pem")
		}
		if *keyFile == "" {
			*keyFile = data.Path("x509/server_key.pem")
		}
		creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			log.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterMembershipServiceServer(grpcServer, &server{})
	grpcServer.Serve(lis)
}
