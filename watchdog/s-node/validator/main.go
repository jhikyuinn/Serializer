package main

import (
	"context"
	"errors"
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

type VALIDATOR struct {
	IP string
}

type server struct {
	pb.UnimplementedMembershipServiceServer
}

var (
	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "", "The TLS cert file")
	keyFile  = flag.String("key_file", "", "The TLS key file")
)

var myAddr string

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

// 이건 왜있지.
func nodeExist(id string) bool {
	for _, mem := range msp.Membership.Nodes {
		if mem.NodeID == id {
			return true
		}
	}
	return false
}

// 이건 왜있지.
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

type NodeMeta struct {
	LastSeen time.Time
}

var NodeMetaMap = make(map[string]*NodeMeta)

func UpdateNode(node *pb.Node) {
	msp.RwM.Lock()
	defer msp.RwM.Unlock()

	for i := range msp.Membership.Nodes {
		if msp.Membership.Nodes[i].NodeID == node.NodeID {
			msp.Membership.Nodes[i].Alive = true
			if meta, ok := NodeMetaMap[node.NodeID]; ok {
				meta.LastSeen = time.Now()
			} else {
				NodeMetaMap[node.NodeID] = &NodeMeta{LastSeen: time.Now()}
			}
			return
		}
	}

	node.Alive = true
	msp.Membership.Nodes = append(msp.Membership.Nodes, node)
	NodeMetaMap[node.NodeID] = &NodeMeta{LastSeen: time.Now()}

	fmt.Println("🆕 새로운 노드 추가:", node.NodeID)
}
func RemoveDeadNodes() {
	for {
		time.Sleep(time.Second * 10)

		msp.RwM.Lock()
		now := time.Now()
		var aliveNodes []*pb.Node

		for _, node := range msp.Membership.Nodes {
			meta, ok := NodeMetaMap[node.NodeID]
			fmt.Println(now.Sub(meta.LastSeen))
			if ok && now.Sub(meta.LastSeen) <= 13*time.Second {
				aliveNodes = append(aliveNodes, node)
			} else {
				delete(NodeMetaMap, node.NodeID)
			}
		}

		fmt.Println("🧹 살아있는 노드 수:", len(aliveNodes))
		msp.Membership.Nodes = aliveNodes
		msp.RwM.Unlock()
	}
}

// GRPC SERVICE
func (s *server) GetMembership(ctx context.Context, msg *pb.MemberMsg) (*pb.MemberMsg, error) {
	if len(msg.Nodes) == 0 {
		return nil, errors.New("no node information received")
	}

	node := &pb.Node{
		NodeID:    msg.Nodes[0].NodeID,
		Addr:      msg.Nodes[0].Addr,
		Publickey: msg.Nodes[0].Publickey,
		Channel:   msg.Nodes[0].Channel,
	}

	UpdateNode(node)

	msp.RwM.RLock()
	defer msp.RwM.RUnlock()
	return msp.Membership, nil
}

func main() {
	validatorAddr := flag.String("validator", "117.16.244.33", "")
	kafkaprocessorAddr := flag.String("kafkaprocessor", "117.16.244.33", "")
	flag.Parse()

	myAddr = *validatorAddr
	validator := VALIDATOR{
		IP: myAddr,
	}
	fmt.Println("Host IP:", validator.IP)

	go RemoveDeadNodes()

	time.Sleep(time.Second * 3)

	// run libp2p that opens watchdog pubsub channel
	wps := wp2p.WatchdogPubsub{}
	go wps.Start(kafkaprocessorAddr)

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
