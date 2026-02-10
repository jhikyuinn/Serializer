package puctrl

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"time"
	"io"

	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/MCNL-HGU/mp2btp/puctrl/util"
	quic "github.com/quic-go/quic-go"
	//equic "github.com/MCNL-HGU/Enhanced-quic"
)

// PuCtrlListen() function manages PuCtrlConnection and prepares to receive data.
func Listener(chStopListen <-chan bool) {

	tlsConfig := util.GenerateTLSConfig(rand.Uint32())

	buf := make([]byte, 5)

	myAddr := fmt.Sprintf("%s:%d", Conf.MY_IP_ADDRS[0], Conf.LISTEN_PORT)
	// myAddr := fmt.Sprintf("0.0.0.0:%d", Conf.LISTEN_PORT)
	listener, err := quic.ListenAddr(myAddr, tlsConfig, &quic.Config{})
	
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-chStopListen:
			util.Log("PuCtrl.Listener(): Listening is stop")
			return

		default:
			util.Log("PuCtrl:Listener(): ListenAddr=%s", myAddr)
			conn, err := listener.Accept(context.Background())
			if err != nil {
				panic(err)
			}

			stream, err := conn.AcceptStream(context.Background())
			if err != nil {
				panic(err)
			}
			defer stream.Close()

			// Read Session ID and Type from the initiated peer
			stream.Read(buf)
			sessionType := buf[0]
			sessionID := binary.LittleEndian.Uint32(buf[1:5])
			util.Log("AcceptStream(): SessionType=%d, SessionID=%d ", sessionType, sessionID)

			puSession, err := CreatePuSession(sessionType, sessionID, conn, stream, false)
			if err != nil {
				panic(err)
			}

			// Add PuSession into sessionMap
			puCtrl := GetPuCtrlInstance()
			puCtrl.mutexSessionMap.Lock()
			puCtrl.sessionMap[puSession.sessionID] = puSession
			puCtrl.mutexSessionMap.Unlock()

			// Start handler
			go puSession.handler()
		}
	}
}

func Connector(src string, dst string, sessionType byte) *PuSession {
	
	// Bind source IP (my IP)
	srcAddr, err := net.ResolveUDPAddr("udp", src)
	if err != nil {
		panic(err)
	}

	// Bind destination IP
	dstAddr, err := net.ResolveUDPAddr("udp", dst)
	if err != nil {
		panic(err)
	}

	// Create UDP connection
	udpConn, err := net.ListenUDP("udp", srcAddr)
	if err != nil {
		panic(err)
	}

	sessionCache := tls.NewLRUClientSessionCache(10)

	// TLS Configuration for Connector
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"puctrl"},
		MaxVersion:         tls.VersionTLS13,
		MinVersion:         tls.VersionTLS10,
		ServerName:         dstAddr.IP.String(),
		ClientSessionCache: sessionCache, // For 0-RTT
	}

	quicConf := &quic.Config{
		Allow0RTT: true, // Enable 0-RTT
	}

	// Connect
	max_conn_retry := int(Conf.MAX_CONNECTION_RETRY)
	if max_conn_retry <= 0 {
		max_conn_retry = 5 // Default
	}

	var conn quic.Connection
	if _, ok := sessionCache.Get(dstAddr.IP.String()); !ok {
		for i := 0; i < max_conn_retry; i++ {
			conn, err = quic.Dial(context.Background(), udpConn, dstAddr, tlsConf, quicConf)
			if conn != nil && err == nil {
				break
			}
			util.Log("PuSession.Connector(): Connection attempt to %s - %d failed!", dst, i+1)

			if i == max_conn_retry-1 && err != nil {
				fmt.Println(err)
				// panic(err)
			}
			//time.Sleep(1 * time.Second)
		}
	} else {
		// 0-RTT connection
		util.Log("PuSession.Connector(): 0-RTT connection attempt to %s", dst)
		conn, err = quic.DialAddrEarly(context.Background(), dst, tlsConf, quicConf)
		if err != nil {
			panic(err)
		}
	}

	if conn == nil || err != nil {
		util.Log("quic dial failed: %s", err)
		return nil
	}
	
	// Get stream
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		util.Log("%s",err)
		// panic(err)
	}

	// Convert session type and ID to byte slice
	sessionInfo := make([]byte, 5)
	sessionInfo[0] = sessionType
	sessionID := rand.Uint32() // Create Session ID
	binary.LittleEndian.PutUint32(sessionInfo[1:5], sessionID)
	_, err = stream.Write(sessionInfo)
	if err != nil {
		panic(err)
	}

	// Create PuSession
	puSession, err := CreatePuSession(sessionType, sessionID, conn, stream, true)
	if err != nil {
		panic(err)
	}

	// Add PuSession into sessionMap
	puCtrl := GetPuCtrlInstance()
	puCtrl.mutexSessionMap.Lock()
	puCtrl.sessionMap[puSession.sessionID] = puSession
	puCtrl.mutexSessionMap.Unlock()

	// Start handler
	go puSession.handler()

	return puSession
}

type PuSession struct {
	sessionType     byte
	sessionID       uint32
	sessionOwner    bool // true if this session is created by this peer
	index           uint16
	conn            quic.Connection
	stream          quic.Stream
	puCtrl          *PuCtrl
	blockNumber     uint32
	blockStatus     uint32
	pushPullTimeout time.Duration
	blockReceiving  bool // True when block is receiving
}

func CreatePuSession(sessionType byte, sessionID uint32,
	conn quic.Connection, stream quic.Stream, sessionOwner bool) (*PuSession, error) {
	s := PuSession{
		sessionType:    sessionType,
		sessionID:      sessionID,
		sessionOwner:   sessionOwner,
		conn:           conn,
		stream:         stream,
		blockNumber:    0,
		blockStatus:    PEER_NOT_KNOWN,
		blockReceiving: false,
	}

	s.puCtrl = GetPuCtrlInstance()
	s.index = s.puCtrl.numSession
	s.puCtrl.numSession++

	// Push to Pull transition timeout
	if Conf.PUSH_PULL_TRANSITION_TIMEOUT > 0 {
		s.pushPullTimeout = time.Duration(Conf.PUSH_PULL_TRANSITION_TIMEOUT) * time.Millisecond
	} else {
		s.pushPullTimeout = time.Duration(1000) * time.Millisecond
	}

	return &s, nil
}

// Start PuSession handler: Receive packets and handle received packet
func (s *PuSession) handler() {
	util.Log("PuSession[%d].handler(): Session handler started!", s.sessionID)

	pushpulltransition := false

	for {

			header := make([]byte, 5)
			if _, err := io.ReadFull(s.stream, header); err != nil { break }

			pktType := header[0]
			bodyLen := int(binary.LittleEndian.Uint32(header[1:5]))

			if bodyLen <= 0 || bodyLen > packet.PACKET_SIZE {
				util.Log("DESYNC: type=%d bodyLen=%d", pktType, bodyLen)
				break
			}

			body := make([]byte, bodyLen)
			if _, err := io.ReadFull(s.stream, body); err != nil { break }

			pkt := append(header, body...)
			s.parsePacket(pktType, pkt)


		// // Read packet type and length (3 bytes)
		// pkt := make([]byte, packet.PACKET_SIZE)
		// n, err := s.readWithSize(pkt, 3)
		// if n == 0 {
		// 	// pushpulltransition = true
		// 	break
		// }
		// if err != nil {
		// 	panic(err)
		// }
		// r := bytes.NewReader(pkt[:3])
		// pktType, _ := r.ReadByte()
		// pktLength, _ := util.ReadUint16(r)
		// if pktType == 0 && pktLength == 0 {
		// 	util.Log("PuSession[%d].handler(): Packet Type and Length read error! pkt=%v", s.sessionID, pkt[0:3])
		// 	break
		// }
		// // util.Log("PuSession[%d].handler(): Packet Type=%d, Length=%d", s.sessionID, pktType, pktLength)

		// // Read remaining packet data
		// if pktType == 23 {
		// 	_, err = s.readWithSize(pkt[3:pktLength+7], int(pktLength+4)) // Read after field of packet length
		// 	if err != nil {
		// 		continue
		// 	}

		// } else if pktType == 24 {
		// 	_, err = s.readWithSize(pkt[3:pktLength+7], int(pktLength+4)) // Read after field of packet length
		// 	if err != nil {
		// 		continue
		// 	}
		// } else {
		// 	n, err = s.readWithSize(pkt[3:pktLength], int(pktLength-3)) // Read after field of packet length
		// 	if n == 0 {
		// 		// pushpulltransition = true
		// 		break
		// 	}
		// 	if err != nil {
		// 		panic(err)
		// 	}
		// }

		// // Parse packet
		// s.parsePacket(pktType, pkt)
	}

	if pushpulltransition {
		util.Log("PuSession[%d].handler(): Push-Pull transition!!!", s.sessionID)

		// If push-pull transition, change peerPushMode to false
		s.puCtrl.peerPushMode = false
		s.blockReceiving = false

		// Close stream
		if err := s.stream.Close(); err != nil {
			panic(err)
		}

		util.Log("PuSession[%d].handler(): Stream closed!", s.sessionID)

		// Push to Pull transition
		s.puCtrl.PushToPull()
	} else {
		util.Log("PuSession[%d].handler(): Session handler finished!", s.sessionID)
	}
}

// Read data with size
func (s *PuSession) readWithSize(buf []byte, size int) (int, error) {
	readBytes := int(0)

	for {
		// Set read deadline for push-pull transition
		if s.puCtrl.peerPushMode && s.blockReceiving {
			s.stream.SetReadDeadline(time.Now().Add(s.pushPullTimeout))
		} else {
			s.stream.SetReadDeadline(time.Time{})
		}

		// Read data
		n, err := s.stream.Read(buf[readBytes:size])
		if err != nil {
			if n == 0 { // When it meets dedadline, transit push mode to pull mode
				if s.puCtrl.peerPushMode && s.blockReceiving {
					util.Log("PuSession[%d].readWithSize(): Deadline!!!", s.sessionID)
				// } else {
				// 	util.Log("PuSession[%d].readWithSize(): Deadline but block is not receiving! Continue to read!", s.sessionID)
				// 	continue
				}
			}

			return n, err
		}

		readBytes += n
		if readBytes >= size {
			break
		}
	}

	return readBytes, nil
}

func (s *PuSession) parsePacket(pktType byte, data []byte) {
	// r := bytes.NewReader(data)
	r := bytes.NewReader(data[5:]) 

	switch pktType {
	// Block Find Packet
	case packet.BLOCK_FIND_PACKET:
		pkt, err := packet.ParseBlockFindPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleBlockFindPacket(pkt)
	// Block Info Packet
	case packet.BLOCK_INFO_PACKET:
		pkt, err := packet.ParseBlockInfoPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleBlockInfoPacket(pkt)
	// Block Request Packet
	case packet.BLOCK_REQUEST_PACKET:
		pkt, err := packet.ParseBlockRequestPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleBlockRequestPacket(pkt)
	// Block Data Packet
	case packet.BLOCK_DATA_PACKET:
		pkt, err := packet.ParseBlockDataPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleBlockDataPacket(pkt)
	// Node Info Packet
	case packet.NODE_INFO_PACKET:
		pkt, err := packet.ParseNodeInfoPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleNodeInfoPacket(pkt)
	// Path Info Packet
	case packet.PATH_INFO_PACKET:
		pkt, err := packet.ParsePathInfoPacket(r)
		if err != nil {
			panic(err)
		}
		s.handlePathInfoPacket(pkt)
	// Path Info ACK Packet
	case packet.PATH_INFO_ACK_PACKET:
		pkt, err := packet.ParsePathInfoPacket(r) // Same function with Path Info Packet
		if err != nil {
			panic(err)
		}
		s.handlePathInfoAckPacket(pkt)
	case packet.FIN_PACKET:
		pkt, err := packet.ParseFinPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleFinPacket(pkt)
	case packet.FIN_ACK_PACKET:
		pkt, err := packet.ParseFinAckPacket(r)
		if err != nil {
			panic(err)
		}
		s.handleFinAckPacket(pkt)

	case packet.AUDIT_MSG_PACKET:
		pkt, err := packet.ParseAuditDataPacket(r)
		if err != nil {
			fmt.Println(err)
			// panic(err)
		}
		s.handleAuditMsgPacket(pkt)
	case packet.AUDIT_MSG_ACK_PACKET:
		pkt, err := packet.ParseAuditDataAckPacket(r)
		if err != nil {
			fmt.Println(err)
			// panic(err)
		}
		s.handleAuditMsgAckPacket(pkt)

	// // Block Data ACK Packet
	// case pc.BLOCK_DATA_ACK_PACKET:
	// 	packet, err := pc.ParseBlockDataAckPacket(r)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	s.handleBlockDataAckPacket(packet)
	// // Block FIN Packet
	// case pc.BLOCK_FIN_PACKET:
	// 	packet, err := pc.ParseBlockFinPacket(r)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	s.handleBlockFinPacket(packet, sessionID)
	// // Control Packet
	// case pc.CONTROL_PACKET:
	// 	packet, err := pc.ParseControlPacket(r)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	s.handleControlPacket(packet)
	// Node Info ACK Packet
	// case pc.NODE_INFO_ACK_PACKET:
	// 	packet, err := pc.ParseNodeInfoAckPacket(r)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	s.handleNodeInfoAckPacket(packet, sessionID)
	default:
		panic(fmt.Sprintf("PuSession[%d].handler(): Unknown packet type (%d)", s.sessionID, pktType))
	}
}

// ////////////////////////////////////////////////////////////////////////
// Handle a Node Info Packet
func (s *PuSession) handleNodeInfoPacket(pkt *packet.NodeInfoPacket) {
	util.Log("PuSession[%d].handleNodeInfoPacket(): NumberOfnodes=%d", s.sessionID, len(pkt.NodeInfos))

	// Save node info
	s.puCtrl.SetNodeInfo(pkt.NodeInfos)

	// Connect to Child nodes
	s.puCtrl.Connect(pkt.NodeInfos, byte(pkt.SessionType))
}

// Handle a Path Info Packet
func (s *PuSession) handlePathInfoPacket(pkt *packet.PathInfoPacket) {
	util.Log("PuSession[%d].handlePathInfoPacket(): Source Info: NumberOfPath=%d, IP=%v", s.sessionID, pkt.NumPath, util.Int2IpArray(pkt.IP))

	// Send path info ack packet
	s.sendPathInfoAckPacket(Conf.MY_IP_ADDRS[0:Conf.NUM_MULTIPATH])

	// Start Enhanced QUIC (FEC tunnel)
	// if Conf.EQUIC_ENABLE {
	// 	isReceiver := false
	// 	if pkt.SessionType == PUSH_SESSION {
	// 		isReceiver = true
	// 	} else {
	// 		isReceiver = false
	// 	}

	// 	// Enable packet forwarding
	// 	if s.sessionType == PULL_SESSION && !isReceiver {
	// 		EnablePacketForwarding(0, true)
	// 	}

	// 	// Run Enhanced QUIC
	// 	go s.RunEnhancedQuic(pkt.IP, isReceiver, s.index)
	// }
}

// Handle a Path Info Ack Packet
func (s *PuSession) handlePathInfoAckPacket(pkt *packet.PathInfoPacket) {
	util.Log("PuSession[%d].handlePathInfoAckPacket(): Destination Info: NumberOfPath=%d, IP=%v", s.sessionID, pkt.NumPath, pkt.IP)

	// Start Enhanced QUIC (FEC tunnel)
	// if Conf.EQUIC_ENABLE {
	// 	isReceiver := false
	// 	if pkt.SessionType == PUSH_SESSION {
	// 		isReceiver = false
	// 	} else {
	// 		isReceiver = true
	// 	}

	// 	// Enable packet forwarding
	// 	if s.sessionType == PUSH_SESSION {
	// 		EnablePacketForwarding(s.index, false)
	// 	}

	// 	// Run Enhanced QUIC
	// 	go s.RunEnhancedQuic(pkt.IP, isReceiver, s.index)
	// }
}

//KYUKYU
// Handle a Audit MSG Packet
func (s *PuSession) handleAuditMsgPacket(pkt *packet.AuditDataPacket) {
	util.Log("PuSession[%d].handleAuditMsgPacket()", s.sessionID)

	s.puCtrl.ReceiveAuditMsg(s.sessionID, pkt)

}

// Handle a Audit MSG Ack Packet
func (s *PuSession) handleAuditMsgAckPacket(pkt *packet.AuditDataAckPacket) {
	util.Log("PuSession[%d].handleAuditMsgAckPacket()", s.sessionID)

	s.puCtrl.ReceiveAuditMsgAck(s.sessionID, pkt)

}

// Handle a Block Find Packet
func (s *PuSession) handleBlockFindPacket(pkt *packet.BlockFindPacket) {
	util.Log("PuSession[%d].handleBlockFindPacket(): BlockNumber=%d", s.sessionID, pkt.BlockNumber)

	// Send block info
	s.puCtrl.SendBlockInfo(s.sessionID, pkt.BlockNumber)
}

// Handle a Block Info Packet
func (s *PuSession) handleBlockInfoPacket(pkt *packet.BlockInfoPacket) {
	util.Log("PuSession[%d].handleBlockInfoPacket(): BlockNumber=%d, Status=%d, BlockSize=%d ", s.sessionID, pkt.BlockNumber, pkt.Status, pkt.BlockSize)

	// Update block information
	s.blockNumber = pkt.BlockNumber
	s.blockStatus = pkt.Status

	// Send block request
	s.puCtrl.SendBlockRequest(s.sessionType, s.sessionID, pkt)
}

// Handle a Block Request Packet
func (s *PuSession) handleBlockRequestPacket(pkt *packet.BlockRequestPacket) {
	util.Log("PuSession[%d].handleBlockRequestPacket(): BlockNumber=%d, RequstedDataNumber=%d-%d, MpInfo=%d, FecSeedNumber=%d",
		s.sessionID, pkt.BlockNumber, pkt.StartDataNumber, pkt.EndDataNumber, pkt.MpInfo, pkt.FecSeedNumber)

	// Send block data
	s.puCtrl.SendBlockData(s.sessionID, pkt)
}

// Handle a Block Data Packet
func (s *PuSession) handleBlockDataPacket(pkt *packet.BlockDataPacket) {
	// Find proper segment
	block, exists := s.puCtrl.blockMap.Get(s.blockNumber)

	if exists {
		if pkt.DataNumber%1000 == 0 {
			util.Log("PuSession[%d].handleBlockDataPacket(): BlockNumber=%d / DataNumber=%d / SegmentList=%d / DataLen=%d",
				s.sessionID, pkt.BlockNumber, pkt.DataNumber, block.GetLength(), len(pkt.Data))
		}

		s.puCtrl.ReceiveBlockData(s.sessionID, pkt)
	} else {
		util.Log("PuSession[%d].handleBlockDataPacket(): Block does not exsit! BlockNumber=%d", s.sessionID, pkt.BlockNumber)
	}
}

// Send a Block Data ACK Packet
// func (s *PuSession) sendBlockDataAckPacket(sessionID uint32, blockNumber uint32, lastDataNumber uint32) {

// 	pkt := packet.CreateBlockDataAckPacket(blockNumber, lastDataNumber)
// 	util.Log("Mp2Session.sendBlockDataAckPacket(): SessionID=%d, BlockNumber=%d, lastDataNumber=%d", sessionID, blockNumber, lastDataNumber)

// 	b := &bytes.Buffer{}
// 	pkt.Write(b)

// 	// Send bytes of packet
// 	s.sendPacket(b.Bytes())
// }

// Send a Node Info Packet (Node Info includes primary path only)
func (s *PuSession) sendNodeInfoPacket(nodeInfos []packet.NodeInfo) {

	pkt := packet.CreateNodeInfoPacket(uint16(s.sessionType), nodeInfos)
	util.Log("PuSession[%d].sendNodeInfoPacket(): SessionType=%d, Number of nodes=%d", s.sessionID, s.sessionType, len(nodeInfos))

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

// Send a Path Info Packet
func (s *PuSession) sendPathInfoPacket(ipAddrs []string) {

	pkt := packet.CreatePathInfoPacket(uint16(s.sessionType), ipAddrs)
	util.Log("PuSession[%d].sendPathInfoPacket(): SessionType=%d, NumPath=%d, IP=%v", s.sessionID, s.sessionType, len(ipAddrs), ipAddrs)

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

// Send a Path Info Ack Packet
func (s *PuSession) sendPathInfoAckPacket(ipAddrs []string) {

	pkt := packet.CreatePathInfoAckPacket(uint16(s.sessionType), ipAddrs)
	util.Log("PuSession[%d].sendPathInfoAckPacket(): SessionType=%d, NumPath=%d, IP=%v", s.sessionID, s.sessionType, len(ipAddrs), ipAddrs)

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

// Send a Block Find Packet
func (s *PuSession) sendBlockFindPacket(blockNumber uint32) {

	pkt := packet.CreateBlockFindPacket(blockNumber)
	util.Log("PuSession[%d].sendBlockFindPacket(): BlockNumber=%d", s.sessionID, blockNumber)

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

// Send a Block Info Packet
func (s *PuSession) SendBlockInfoPacket(blockNumber uint32, blockSize uint32, status uint32) {

	pkt := packet.CreateBlockInfoPacket(blockNumber, status, blockSize)
	util.Log("PuSession[%d].SendBlockInfoPacket(): BlockNumber=%d, BlockSize=%d, Status=%d", s.sessionID, blockNumber, blockSize, status)

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

// Send a Block Request Packet
func (s *PuSession) SendBlockRequestPacket(blockNumber uint32, startDataNumber uint32, endDataNumber uint32, mpInfo byte, fecSeedNumer uint32) {

	pkt := packet.CreateBlockRequestPacket(blockNumber, startDataNumber, endDataNumber, mpInfo, fecSeedNumer)
	util.Log("PuSession[%d].sendBlockRequestPacket(): BlockNumber=%d, DataNumber=%d-%d, mpInfo=%d, FecSeedNumber=%d", s.sessionID, blockNumber, startDataNumber, endDataNumber, mpInfo, fecSeedNumer)

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

// Send a Block Data Packet
func (s *PuSession) sendBlockDataPacket(blockNumber uint32, dataNumber uint32, dataType byte, data []byte) {

	pkt := packet.CreateBlockDataPacket(blockNumber, dataNumber, s.sessionID, dataType, data)
	// util.Log("PuSession[%d].sendBlockDataPacket(): BlockNumber=%d, DataNumber=%d, Length of data=%d", s.sessionID, blockNumber, dataNumber, len(data))

	b := &bytes.Buffer{}
	pkt.Write(b)

	// Send bytes of packet
	s.sendPacket(b.Bytes())
}

//KYUKYU
func (s *PuSession) sendAuditDataPacket(data []byte) {

	// pkt := packet.CreateAuditDataPacket(s.sessionID, data)
	// util.Log("PuSession[%d].sendAuditDataPacket(): Length of data=%d", s.sessionID, len(data))

	// b := &bytes.Buffer{}
	// pkt.Write(b)

	// // Send bytes of packet
	// s.sendPacket(b.Bytes())

	pkt := packet.CreateAuditDataPacket(s.sessionID, data)
    util.Log("PuSession[%d].sendAuditDataPacket(): Length of data=%d",
        s.sessionID, len(data))

    // 2) AuditDataPacket → body bytes
    bodyBuf := new(bytes.Buffer)
    if err := pkt.Write(bodyBuf); err != nil {
		fmt.Println(err)
    }
    body := bodyBuf.Bytes()

    // 3) 프레임 헤더 + body 전송
    if err := pkt.WriteFrame(s.stream, body); err != nil {
		fmt.Println(err)
    }

}

func (s *PuSession) sendAuditDataAckPacket(data []byte) {


	pkt := packet.CreateAuditDataAckPacket(s.sessionID, data)
	util.Log("PuSession[%d].CreateAuditDataAckPacket(): Length of data=%d",s.sessionID, len(data))

    // 2) AuditDataPacket → body bytes
    bodyBuf := new(bytes.Buffer)
    if err := pkt.Write(bodyBuf); err != nil {
		fmt.Println(err)
    }
    body := bodyBuf.Bytes()

    // 3) 프레임 헤더 + body 전송
    if err := pkt.WriteFrame(s.stream, body); err != nil {
		fmt.Println(err)
    }
}

// Send a FIN Packet
func (s *PuSession) sendFinPacket(blockNumber uint32) {
	pkt := packet.CreateFinPacket(s.sessionID, blockNumber)
	util.Log("PuSession[%d].sendFinPacket(): BlockNumber=%d", s.sessionID, blockNumber)

	b := &bytes.Buffer{}
	pkt.Write(b)

	s.sendPacket(b.Bytes())
}

// Send a FIN ACK Packet
func (s *PuSession) sendFinAckPacket(blockNumber uint32) {
	pkt := packet.CreateFinAckPacket(s.sessionID, blockNumber)
	util.Log("PuSession[%d].sendFinAckPacket(): BlockNumber=%d", s.sessionID, blockNumber)

	b := &bytes.Buffer{}
	pkt.Write(b)

	s.sendPacket(b.Bytes())
}

// // Send a Block FIN Packet
// func (s *PuSession) sendBlockFinPacket(sessionID uint32) {

// 	packet := pc.CreateBlockFinPacket(sessionID, s.blockNumber)
// 	util.Log("Mp2Session.sendBlockFinPacket(): SessionID=%d, BlockNumber=%d", sessionID, s.blockNumber)

// 	b := &bytes.Buffer{}
// 	packet.Write(b)

// 	// Send bytes of packet
// 	s.sendPacket(sessionID, b.Bytes(), packet.Type)
// }

// // Send a Control Packet
// func (s *PuSession) sendControlPacket(sessionID uint32, data []byte) {

// 	packet := pc.CreateControlPacket(data)
// 	util.Log("Mp2Session.sendControlPacket(): SessionID=%d, Length of data=%d", sessionID, len(data))

// 	b := &bytes.Buffer{}
// 	packet.Write(b)

// 	// Send bytes of packet
// 	s.sendPacket(sessionID, b.Bytes(), packet.Type)
// }

// // Send a Node Info ACK Packet
// func (s *PuSession) sendNodeInfoAckPacket(sessionID uint32, numOfInfo uint16) {

// 	packet := pc.CreateNodeInfoAckPacket(numOfInfo)
// 	util.Log("Mp2Session.sendNodeInfoAckPacket(): SessionID=%d, Number of nodes=%d", sessionID, numOfInfo)

// 	b := &bytes.Buffer{}
// 	packet.Write(b)

// 	// Send bytes of packet
// 	s.sendPacket(sessionID, b.Bytes(), packet.Type)
// }

// Send a packet
func (s *PuSession) sendPacket(buf []byte) {
	// util.Log("PuSession[%d].sendPacket(): pkt=%v", s.sessionID, buf)
	_, err := s.stream.Write(buf)
	if err != nil {
		// panic(err)
		fmt.Println(err)

	}
}

// // Read a Control Packet
// func (s *PuSession) ReadControl(buf []byte) int {
// 	// TODO: infinite-loop
// 	for len(s.controlRecvBuffer) == 0 && !s.goodbye {
// 	}

// 	readLen := 0

// 	if len(s.controlRecvBuffer) > 0 {
// 		s.mutexControl.Lock()
// 		// Copy packet data to buf
// 		packet := s.controlRecvBuffer[0]
// 		copy(buf, packet.Data)
// 		readLen = len(packet.Data)

// 		// Remove first packet in control receive buffer
// 		s.controlRecvBuffer = s.controlRecvBuffer[1:]
// 		s.mutexControl.Unlock()
// 	}

// 	return readLen
// }

// // Write a Control Packet
// func (s *PuSession) WriteControl(buf []byte) {

// 	offset := uint32(0)
// 	buf_len := uint32(len(buf))
// 	sessionID := uint32(0)

// 	// TODO: handling session ID for Write()
// 	for offset < buf_len {
// 		size := uint32(0)
// 		if offset+uint32(pc.PAYLOAD_SIZE) <= buf_len {
// 			size = uint32(pc.PAYLOAD_SIZE)
// 		} else {
// 			size = (uint32(len(buf)) - offset)
// 		}

// 		data := make([]byte, size)
// 		copy(data, buf[offset:offset+size])

// 		// Send a Control Packet
// 		for sessionID = range s.sessionMap {
// 			s.sendControlPacket(sessionID, data)
// 		}

// 		offset += size
// 	}
// }

// Session close
func (s *PuSession) Close() {
	// if (s.sessionType == PUSH_SESSION && !s.sessionOwner) ||
	// 	(s.sessionType == PULL_SESSION && s.sessionOwner) {
	// 	// Send Close Packet to remote peer
	s.conn.CloseWithError(0, "closing connection")
	// 	util.Log("PuSession[%d].Close(): Close packet sent!", s.sessionID)
	// } else {
	// 	// wait until close packets are received
	// }

	util.Log("PuSession[%d].Close(): Session Close!", s.sessionID)
}

func (s *PuSession) handleFinPacket(pkt *packet.FinPacket) {
	util.Log("PuSession[%d].handleFinPacket(): BlockNumber=%d", s.sessionID, pkt.BlockNumber)

	s.puCtrl.chFinReceived <- pkt.BlockNumber // Notify that FIN packet is received
}

func (s *PuSession) handleFinAckPacket(pkt *packet.FinAckPacket) {
	util.Log("PuSession[%d].handleFinAckPacket(): BlockNumber=%d", s.sessionID, pkt.BlockNumber)

	s.puCtrl.chFinAckReceived <- pkt.BlockNumber // Notify that FIN ACK packet is received

	// // If this session is owner, close session
	// if s.sessionOwner {
	// 	s.Close()
	// }

	// // Remove session from sessionMap
	// puCtrl := GetPuCtrlInstance()
	// puCtrl.mutexSessionMap.Lock()
	// delete(puCtrl.sessionMap, s.sessionID)
	// puCtrl.mutexSessionMap.Unlock()
	// util.Log("PuSession[%d].handleFinAckPacket(): Session closed and removed from sessionMap", s.sessionID)
}
