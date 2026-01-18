package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"

	"encoding/json"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/examples/data"

	msp "validator/membership"
	pb "validator/msg"
	wp2p "validator/wp2p"
)

type Validator struct {
    IP string `json:"ip"`
}

type server struct {
	pb.UnimplementedMembershipServiceServer
}

type NodeMeta struct {
	LastSeen time.Time
}

const nodeTimeout = 10 * time.Second

var NodeMetaMap = make(map[string]*NodeMeta)

var myAddr string

func GetExternalIP() {
	l, err := net.Listen("tcp", myAddr+":11730")
	if nil != err {
		log.Println(err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
            log.Println("Accept error:", err)
            continue
        }
		go ConnHandler(conn)
	}
}

func ConnHandler(conn net.Conn) {
	defer conn.Close()

	recvBuf := make([]byte, 64)
	n, err := conn.Read(recvBuf)
	if err != nil {
		if err == io.EOF {
            log.Printf("Connection closed by client: %v", conn.RemoteAddr())
        } else {
            log.Printf("Failed to read data: %v", err)
        }
		return
	}
	var id string
    if err := json.Unmarshal(recvBuf[:n], &id); err != nil {
        log.Printf("JSON unmarshal error: %v", err)
        return
    }

	fmt.Println("Connected NODE_ID:", id)

	msp.RwM.Lock()
	defer msp.RwM.Unlock()

	for idx, each := range msp.Membership.Nodes {
		if each.NodeID == id {
            ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
            if err != nil {
                log.Println("Failed to parse remote address:", err)
                return
            }
            msp.Membership.Nodes[idx].Addr = ip
            break
        }
	}
}

func UpdateNode(node *pb.Node) {
	now := time.Now()
	msp.RwM.Lock()
	defer msp.RwM.Unlock()

	for i := range msp.Membership.Nodes {
		if msp.Membership.Nodes[i].NodeID == node.NodeID {
			msp.Membership.Nodes[i] = node

			if meta, ok := NodeMetaMap[node.NodeID]; ok {
				meta.LastSeen = now
			} else {
				NodeMetaMap[node.NodeID] = &NodeMeta{LastSeen: now}
			}
			return
		}
	}

	msp.Membership.Nodes = append(msp.Membership.Nodes, node)
	NodeMetaMap[node.NodeID] = &NodeMeta{LastSeen: now}
	fmt.Println("🆕 Add new node:", node.NodeID)
}

func IsNodeAlive(nodeID string) bool {
	msp.RwM.RLock()
    meta, ok := NodeMetaMap[nodeID]
    msp.RwM.RUnlock()

    if !ok {
        return false
    }
    return time.Since(meta.LastSeen) <= nodeTimeout
}

func RemoveDeadNodes() {
	ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

	for range ticker.C {
        now := time.Now()

        msp.RwM.Lock()
        alive := msp.Membership.Nodes[:0] 
        for _, node := range msp.Membership.Nodes {
            meta, ok := NodeMetaMap[node.NodeID]
            if ok && now.Sub(meta.LastSeen) <= nodeTimeout {
                alive = append(alive, node)
            } else {
                fmt.Println("🗑️ Removing inactive node:", node.NodeID)
                delete(NodeMetaMap, node.NodeID)
            }
        }
        msp.Membership.Nodes = alive
        msp.RwM.Unlock()
    }
}

// GRPC SERVICE
func (s *server) GetMembership(ctx context.Context, msg *pb.MemberMsg) (*pb.MemberMsg, error) {
	if len(msg.Nodes) == 0 {
		return nil, errors.New("no node information received")
	}

	node := msg.Nodes[0]
	UpdateNode(node)
	fmt.Println("MSG INFO",msg.Nodes[0].Publickey)

	msp.HandleMembershipRequest(msg)

	msp.RwM.RLock()
	defer msp.RwM.RUnlock()
	return msp.Membership, nil
}

func main() {
	validatorAddr := flag.String("validator", "117.16.244.33", "Validator IP address")
    kafkaprocessorAddr := flag.String("kafkaprocessor", "117.16.244.33", "Kafka processor IP")
    tls := flag.Bool("tls", false, "Use TLS for gRPC")
    certFile := flag.String("cert_file", "", "TLS cert file")
    keyFile := flag.String("key_file", "", "TLS key file")
	flag.Parse()

	validator := Validator{IP: *validatorAddr}
    log.Println("Host IP:", validator.IP)

	go RemoveDeadNodes()
	time.Sleep(10 * time.Second)

	// libp2p watchdog pubsub
	wps := wp2p.WatchdogPubsub{}
	go wps.Start(kafkaprocessorAddr)

	// Membership processor
	msp.Start()

	// TCP listener for updating external IPs
	lis, err := net.Listen("tcp", myAddr+":16220")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go GetExternalIP()

	// --- gRPC server ---
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
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("gRPC server failed: %v", err)
    }
}
