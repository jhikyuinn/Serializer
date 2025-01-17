// Copyright INLab in Dongguk University All Rights Reserved

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	overlay "github.com/docbull/inlab-fabric-modules/inlab-sdn/protos/overlay"
	info "github.com/docbull/inlab-fabric-modules/inlab-sdn/protos/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Peer stores peer information e.g., Endpoint and Network status
type Peer struct {
	peerConnection   *info.PeerConnection
	OverlayStructure *info.OverlayStructure
	clientConn       *grpc.ClientConn
}

// Start runs SDN Client for INLab fabric
func Start(partner string) {
	peer := Peer{}

	peer.PeerInitialization(partner)

	// SDN Client establishes Peer connection and SDN Server connection
	go peer.StartSDNListening()
	go peer.StartOverlayListening()

	// It sends peer information every a second
	for {
		go peer.NetworkUpdate()
		go peer.ConnectSDNServer()

		time.Sleep(time.Second * 5)
	}
}

func (p *Peer) PeerInitialization(partner string) {
	// Initialize Peer information
	// if the peer has D2D partner, it stores the information
	// of D2D partner name.
	p.peerConnection = &info.PeerConnection{
		Mobile: &info.PeerInfo{},
		D2D:    &info.PeerInfo{},
	}
	p.peerConnection.D2D = &info.PeerInfo{
		Endpoint: partner,
		NetInfo:  nil,
	}
	p.OverlayStructure = &info.OverlayStructure{
		SuperPeer: "",
		Endpoint:  "",
		SubPeers:  nil,
	}

	// Establish an connection with SDN Server
	MEC_IP := os.Getenv("MEC_IP")
	connMEC := string(MEC_IP) + ":9000"
	conn, err := grpc.Dial(connMEC, grpc.WithInsecure())
	checkError(err)
	p.clientConn = conn
	defer p.clientConn.Close()
}

// NetworkUpdate updates network status of peer
func (p *Peer) NetworkUpdate() {
	MEC_IP := os.Getenv("MEC_IP")
	cmd, _ := exec.Command("ping", string(MEC_IP), "-c", "1").Output()

	parseTime := "time="
	parseMillisec := " ms"
	parseRTT := strings.Index(string(cmd), parseTime)

	tempRTT := string(cmd[(parseRTT + 5):])
	tempMillisec := strings.Index(tempRTT, parseMillisec)
	RTT, err := strconv.ParseFloat(tempRTT[:tempMillisec], 32)
	checkError(err)
	fmt.Println()

	peerNetInfo := info.NetworkInfo{
		NetworkInterface: "Cellular",
		NetworkStrength:  float32(RTT),
	}

	p.peerConnection.Mobile.NetInfo = &peerNetInfo

	if p.peerConnection.D2D.Endpoint == "" {
		fmt.Println("None of D2D partner")
	} else {
		D2D_PAIR := os.Getenv("D2D_PAIR")
		cmd, _ = exec.Command("ping", string(D2D_PAIR), "-c", "1").Output()

		parseRTT = strings.Index(string(cmd), parseTime)
		tempRTT = string(cmd[(parseRTT + 5):])
		tempMillisec = strings.Index(tempRTT, parseMillisec)
		if tempMillisec == -1 {
			cmd, _ = exec.Command("ping", string(D2D_PAIR), "-c", "1").Output()

			parseRTT = strings.Index(string(cmd), parseTime)
			tempRTT = string(cmd[(parseRTT + 5):])
			tempMillisec = strings.Index(tempRTT, parseMillisec)
		}
		RTT, err = strconv.ParseFloat(tempRTT[:tempMillisec], 32)
		checkError(err)
		fmt.Println(RTT)

		peerD2DNetInfo := info.NetworkInfo{
			NetworkInterface: "D2D",
			NetworkStrength:  float32(RTT),
		}

		p.peerConnection.D2D.NetInfo = &peerD2DNetInfo
	}

	fmt.Println(p.peerConnection.Mobile.NetInfo)
	fmt.Println(p.peerConnection.D2D.NetInfo)
}

// StartSDNListening waits for Peer container's connection
// It communicates with peer container over golang TCP socket
func (p *Peer) StartSDNListening() {
	lis, err := net.Listen("tcp", ":8000")
	checkError(err)
	defer lis.Close()

	fmt.Println("SDN Client ...")
	for {
		conn, err := lis.Accept()
		checkError(err)

		go p.ReceivePeerInformation(conn)
	}
}

// ReceivePeerInformation receives peer information from peer container
func (p *Peer) ReceivePeerInformation(conn net.Conn) {
	endpoint := make([]byte, 1024)
	_, err := conn.Read(endpoint)
	checkError(err)

	p.peerConnection.Mobile.Endpoint = string(endpoint)
	p.NetworkUpdate()

	fmt.Println(string(endpoint))

	_, err = conn.Write(endpoint)
	if err != nil {
		fmt.Println(err)
		return
	}

	// when received peer's endpoint, it sends peer information to SDN Server
	p.ConnectSDNServer()
}

// ConnectSDNServer connects to SDN Server for constructing overlay structure
func (p *Peer) ConnectSDNServer() {
	MEC_IP := os.Getenv("MEC_IP")
	connMEC := string(MEC_IP) + ":9000"

	clnt := info.NewSDNServiceClient(p.clientConn)
	//mPeer := info.PeerInfo{Endpoint: p.peerConnection.Mobile.Endpoint, NetInfo: p.peerConnection.Mobile.NetInfo}
	mPeer := info.PeerConnection{Mobile: p.peerConnection.Mobile, D2D: p.peerConnection.D2D}

	res, err := clnt.StorePeerInformation(context.Background(), &mPeer)
	if err != nil {
		log.Println("Server connection loss. Try to re-connect to SDN Server")

		conn, err := grpc.Dial(connMEC, grpc.WithInsecure())
		checkError(err)
		p.clientConn = conn
		return
	}
	if res.Code != info.StatusCode_Ok {
		log.Println(res.Code)
		log.Println("It might be not connected with Peer container. Please check that is the peer container running.")
		return
	}

	p.OverlayStructure, err = clnt.UpdateOverlayStructure(context.Background(), p.peerConnection.Mobile)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Super Peer:", p.OverlayStructure.SuperPeer)
	fmt.Println(p.OverlayStructure.Endpoint)
	fmt.Println(p.OverlayStructure.SubPeers)
}

// StartOverlayListening waits connection of peer container's
// Overlay Structure requests
func (p *Peer) StartOverlayListening() {
	lis, err := net.Listen("tcp", ":8080")
	checkError(err)

	grpcServer := grpc.NewServer()
	overlay.RegisterOverlayStructureServiceServer(grpcServer, p)
	reflection.Register(grpcServer)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal(err)
	}
}

// SeeOverlayStructure returns current overlay structure to the peer
func (p *Peer) SeeOverlayStructure(ctx context.Context, endpoint *overlay.PeerEndpoint) (*overlay.OverlayStructure, error) {
	fmt.Println("************************************")
	fmt.Println("Connected Peer:", endpoint.Endpoint)
	fmt.Println("************************************")

	myOverlayStructure := &overlay.OverlayStructure{
		SuperPeer: p.OverlayStructure.SuperPeer,
		Endpoint:  p.OverlayStructure.Endpoint,
		SubPeers:  p.OverlayStructure.SubPeers,
	}

	return myOverlayStructure, nil
}

func checkError(err error) {
	if err != nil {
		log.Println(err)
	}
}

func main() {
	var partner string

	if len(os.Args) < 2 {
		partner = ""
	} else {
		partner = string(os.Args[1])
	}

	fmt.Println("D2D partner:", partner)

	Start(partner)
}
