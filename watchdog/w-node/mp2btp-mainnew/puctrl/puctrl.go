package puctrl

import (
	"fmt"
	"sync"
	"time"
	"net"

	"github.com/BurntSushi/toml"
	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/MCNL-HGU/mp2btp/puctrl/util"
	//pc "github.com/MCNL-HGU/mp2btp/puctrl/packet"
	//equic "github.com/MCNL-HGU/Enhanced-quic"
)

// -t means that it is a variable for testing.
// It will be removed.
type PuCtrl struct {
	peerId              uint32                   // Peer ID -t
	peerPushMode        bool                     // PUSH mode flag -t
	peerModeStatus      string                   // Mode status (PUSH or PULL) -t
	childAddr           []string                 // Child addresses
	numSession          uint16                   // Number of Sessions
	numPullSession      uint16                   // Number of Pull Sessions
	sessionMap          map[uint32](*PuSession)  // Push/Pull Session list
	blockMap            *SafeMap[uint32, *Block] //blockMap            map[uint32](*Block)     // Receive buffer (key: blockNumber, value: BlockBuffer)
	expectedBlockNumber uint32                   // Block number expected to receive (Receiving Block Number)

	semPush         chan uint32
	semPull         chan uint32
	mutexForTimeout sync.Mutex      // Mutext for timeout
	goodbye         bool            // For reading
	timeoutFlag     map[uint32]bool // Timeout flag (key: blockNumber)

	// New
	chStopListen       chan bool // Stop goroutine for listening
	chStopTransmission chan bool // Stop goroutine for sender's transmission
	mutexSessionMap    sync.Mutex
	nodeInfo           []packet.NodeInfo // Node Info
	state              int
	chFinReceived      chan uint32 // Channel for receiving FIN packet
	chFinAckReceived   chan uint32 // Channel for receiving FIN ACK packet


	listenOnce   sync.Once
	AuditBroadcaster *Broadcaster
	AuditMsgChan     chan *packet.AuditDataPacket
	AuditMsgAckChan  chan *packet.AuditDataAckPacket
}

type Broadcaster struct {
	mu        sync.Mutex
	listeners []chan *packet.AuditDataPacket
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		listeners: make([]chan *packet.AuditDataPacket, 0),
	}
}

// 리스너 등록 (각 소비자마다 사용)
func (b *Broadcaster) Register() chan *packet.AuditDataPacket {
	ch := make(chan *packet.AuditDataPacket, 10) // 버퍼 조정 가능
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners = append(b.listeners, ch)
	return ch
}

// 메시지 브로드캐스트
func (b *Broadcaster) Broadcast(pkt *packet.AuditDataPacket) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.listeners {
		select {
		case ch <- pkt:
		default:
			fmt.Println("⚠️ 채널이 가득 찼습니다. 해당 리스너는 메시지를 받지 못했을 수 있음")
		}
	}
}

// Singleton instance
var puCtrlInstance *PuCtrl

// CreatePuCtrl() function creates Push/Pull Control instance and returns it.
func CreatePuCtrl(configFile string, peerID uint32) *PuCtrl {

	// Parse the config file
	if _, err := toml.DecodeFile(configFile, &Conf); err != nil {
		panic(err)
	}

	util.SetFlag(Conf.VERBOSE_MODE)

	// Default
	Conf.MULTIPEER_FEC_MODE = true

	// Pull Mode
	util.Log("PuCtrl.CreatePuCtrl(): PULL_MODE=%d", Conf.PULL_MODE)

	// Create a PuCtrl Manager instance
	p := PuCtrl{
		peerId:              peerID,
		sessionMap:          make(map[uint32]*PuSession),
		blockMap:            NewSafeMap[uint32, *Block](),
		expectedBlockNumber: 0,
		goodbye:             false,
		peerPushMode:        true,
		peerModeStatus:      "",
		timeoutFlag:         make(map[uint32]bool),
		numSession:          0,
		numPullSession:      0,
		chStopTransmission:  nil,
		mutexSessionMap:     sync.Mutex{},
		chFinReceived:       make(chan uint32, 1), // Channel for receiving FIN packet
		chFinAckReceived:    make(chan uint32, 1), // Channel for receiving FIN packet
		AuditMsgChan:        make(chan *packet.AuditDataPacket),
		AuditMsgAckChan:     make(chan *packet.AuditDataAckPacket),
		AuditBroadcaster:    NewBroadcaster(),
	}

	// Singletone instance
	puCtrlInstance = &p

	// Flush iptables
	DisablePacketForwarding()

	return &p
}

// GetPuCtrlInstance() returns single instance of PuCtrl
func GetPuCtrlInstance() *PuCtrl {
	if puCtrlInstance == nil {
		util.Log("GetPuCtrlInstance(): PuCtrl Instance is nil")
		panic("GetPuCtrlInstance(): PuCtrl Instance is nil")
	}
	return puCtrlInstance
}

// Listen() makes ready state to establish PuSession
func (p *PuCtrl) Listen() {
    p.listenOnce.Do(func() {
        p.chStopListen = make(chan bool)

        if Conf.EQUIC_ENABLE {
            EnablePacketForwarding(0, false)
            go RunEnhancedQuic(nil, false, PULL_SESSION, 0)
        }

        go Listener(p.chStopListen)
        util.Log("PuCtrl.Listen(): Started listening")
    })
}

var UDPConn    []*net.UDPConn

// Connect() makes connection with nodes
func (p *PuCtrl) Connect(nodeInfo []packet.NodeInfo, sessionType byte) {
	// Get peer addresses to connect to from the node info list
	// Find my address
	var myInfo packet.NodeInfo
	for _, node := range nodeInfo {
		nodeAddr := util.Int2ip(node.IP)
		if nodeAddr == Conf.MY_IP_ADDRS[0] {
			myInfo = node
			break
		}
	}

	// Get child node's address
	p.childAddr = make([]string, 0)
	for i := myInfo.OffsetOfChild; i < myInfo.OffsetOfChild+myInfo.NumOfChilds; i++ {
		nodeAddr := fmt.Sprintf("%s:%d", util.Int2ip(nodeInfo[i].IP), 5000)
		p.childAddr = append(p.childAddr, nodeAddr)
	}

	if len(p.childAddr) > 0 {
		i := uint16(0)
		for _, peerAddr := range p.childAddr {
			// My address
			sessionID := p.numSession
			p.numSession++ 
			myBindAddr := fmt.Sprintf("%s:%d", Conf.MY_IP_ADDRS[0], Conf.BIND_PORT+sessionID)

			// Start fectun if enabled
			if Conf.EQUIC_ENABLE {
				isReceiver := false
				if sessionType == PULL_SESSION {
					isReceiver = true
					// EnablePacketForwarding(p.numSession, false) // Enable packet forwarding
					// go RunEnhancedQuic([]string{util.GetIP(peerAddr)}, isReceiver, int(sessionType), p.numSession)
				}

				if sessionType == PUSH_SESSION && !isReceiver {
					EnablePacketForwarding(p.numSession, false) // Enable packet forwarding
					go RunEnhancedQuic([]string{util.GetIP(peerAddr)}, isReceiver, int(sessionType), sessionID)
				}
			}

			util.Log("PuCtrl.Connect(): MyAddr=%s, PeerAddr=%s, SessionType=%d, numSession=%d",
				myBindAddr, peerAddr, sessionType, sessionID)

			// Connect to peer
			session := Connector(myBindAddr, peerAddr, sessionType)

			util.Log("PuCtrl.Connect(): MyAddr=%s, PeerAddr=%s, SessionType=%d, SessionID=%d, numSession=%d",
				myBindAddr, peerAddr, sessionType, session.sessionID, sessionID)

			// Send Node Info
			// session.sendNodeInfoPacket(nodeInfo)

			// Send Path Info
			// session.sendPathInfoPacket(Conf.MY_IP_ADDRS[0:Conf.NUM_MULTIPATH])

			// Tricky method for test4
			i++
			if Conf.NUM_PEERS > 0 && i == Conf.NUM_PEERS {
				break
			}
		}
	} else {
		util.Log("PuCtrl.Connect(): No child peer to connect!")
	}

}

// SendBlock() function sends the requested block.
// We assume that the requested block is "blockName" file.
//
// It will be called by the root peer trying to propagate the block.
func (p *PuCtrl) Send(blockFileName string, blockNumber uint32) {

	// Wait for Node Info Packet to arrive
	//<-p.nodeInfoChan

	// Append to blockMap
	block := CreateBlockBuffer(blockNumber, 0)
	block.FillBlockFromFile(blockFileName)
	p.blockMap.Set(blockNumber, block)

	// Send Block Info Packet to PUSH_SESSION
	isPushSessionExist := false
	for sessionID := range p.sessionMap {
		if p.sessionMap[sessionID].sessionType == PUSH_SESSION {
			p.SendBlockInfo(sessionID, blockNumber)
			isPushSessionExist = true
		}
	}

	if !isPushSessionExist {
		panic("PuCtrl.SendBlock(): Push session does not exist!")
	}
}


// KYUKYU
func (p *PuCtrl) SendAuditMsg(data []byte) error {

	isPushSessionExist := false
	for sessionID := range p.sessionMap {
		if p.sessionMap[sessionID].sessionType == PUSH_SESSION {
			p.SendAuditInfo(sessionID, data)
			isPushSessionExist = true
		}
	}

	if !isPushSessionExist {
		return fmt.Errorf("SendAuditMsg(): Push session does not exist")
	}
	return nil
}

//KYUKYU
func (p *PuCtrl) SendAuditAckMsg(data []byte) error {

	isPushSessionExist := false

	for sessionID := range p.sessionMap {
		if p.sessionMap[sessionID].sessionType == PUSH_SESSION {
			p.SendAuditAckInfo(sessionID, data)
			isPushSessionExist = true
		}
	}

	if !isPushSessionExist {
		return fmt.Errorf("SendAuditAckMsg(): Push session does not exist")
	}
	return nil
}

func (p *PuCtrl) ReceiveAuditMsg(sessionID uint32, pkt *packet.AuditDataPacket) {
	fmt.Println("ReceiveAuditMsg")
	p.AuditBroadcaster.Broadcast(pkt)
}

func (p *PuCtrl) ReceiveAuditMsgAck(sessionID uint32, pkt *packet.AuditDataAckPacket) {
	fmt.Println("ReceiveAuditMsgAck")
	p.AuditMsgAckChan <- pkt
}


// ReceiveBlock() function reads the requested block and stores it in the given byte slice.
func (p *PuCtrl) Receive(buf []byte) (uint32, uint32, int, error) {
	// Wait until unread data exist for the block with expectedBlockNumber
	var block *Block
	block = nil
	for {
		exists := false
		block, exists = p.blockMap.Get(p.expectedBlockNumber)
		if exists && block.readBytes < block.blockSize {
			break
		}
	}

	// Read data from the block
	readLen, _ := block.ReadBlockData(buf)

	// TODO ???
	if p.peerPushMode {
		p.peerModeStatus = "Push Mode: Download from parent!"
	} else {
		p.peerModeStatus = "Pull Mode: Download from pull node(s)!"
	}

	return p.expectedBlockNumber, block.blockSize, readLen, nil
}

func (p *PuCtrl) RegisterBlock(blockFileName string, blockNumber uint32) {
	block := CreateBlockBuffer(blockNumber, 0)
	block.FillBlockFromFile(blockFileName)
	p.blockMap.Set(blockNumber, block)
}

func (p *PuCtrl) FindBlock(blockNumber uint32) {
	// Send Block Find Packet to all PULL_SESSION
	isPullSessionExist := false
	p.numPullSession = 0
	for sessionID := range p.sessionMap {
		if p.sessionMap[sessionID].sessionType == PULL_SESSION {
			p.SendBlockFind(sessionID, blockNumber)
			isPullSessionExist = true
			p.numPullSession++
		}
	}

	if !isPullSessionExist {
		panic("PuCtrl.FindBlock(): Pull session does not exist!")
	}
}

// Covert PeerAddr to NodeInfo
func (p *PuCtrl) GetNodeInfo(peerAddr []PeerAddr) []packet.NodeInfo {

	nodeInfosLen := len(peerAddr)
	nodeInfo := make([]packet.NodeInfo, nodeInfosLen)

	for i := 0; i < len(nodeInfo); i++ {
		// Set NodeInfo
		nodeInfo[i].IP = util.Ip2int(peerAddr[i].Addr)
		nodeInfo[i].Port = uint16(Conf.LISTEN_PORT)
		nodeInfo[i].NumOfChilds = peerAddr[i].NumChild
		nodeInfo[i].OffsetOfChild = peerAddr[i].ChildOffset

		util.Log("PuCtrl.GetNodeInfo(): NodeInfo[%d] = Addr=%s:%d, NumChild=%d, ChildOffset=%d",
			i, util.Int2ip(nodeInfo[i].IP), nodeInfo[i].Port, nodeInfo[i].NumOfChilds, nodeInfo[i].OffsetOfChild)
	}

	return nodeInfo
}

func (pu *PuCtrl) CloseAllSessions() {
	if pu ==nil{
		return
	}
	for _, session := range pu.sessionMap {
		session.Close() // session.handler 내부에서 graceful하게 처리되면 더 좋음
	}

	// for i, conn := range UDPConn {
    //     if conn != nil {
    //         fmt.Printf("CLOSE UDPConn[%d]: %v\n", i, conn.LocalAddr())
    //         err := conn.Close()
    //         if err != nil {
    //             fmt.Println("❌ Failed to close UDPConn:", err)
    //         } else {
    //             fmt.Println("✅ UDPConn closed successfully")
    //         }
    //     }
    // }

    // // 전부 닫았으니 슬라이스 초기화
    // UDPConn = nil

    // 세션 맵 초기화
    pu.sessionMap = make(map[uint32]*PuSession)
}

func (p *PuCtrl) DisconnectFromChildren() {
	for sessionID, session := range p.sessionMap {
		for _, child := range p.childAddr {
			fmt.Printf("🔌 Disconnecting from child: %s\n", child)
			session.Close() // stream, session 종료
			delete(p.sessionMap, sessionID)
			break
		}
	}
}


func (p *PuCtrl) SetNodeInfo(nodeinfos []packet.NodeInfo) {
	nodeInfosLen := len(nodeinfos)
	p.nodeInfo = make([]packet.NodeInfo, nodeInfosLen)

	for i := 0; i < len(p.nodeInfo); i++ {
		// Set NodeInfo
		p.nodeInfo[i].IP = nodeinfos[i].IP
		p.nodeInfo[i].Port = nodeinfos[i].Port
		p.nodeInfo[i].NumOfChilds = nodeinfos[i].NumOfChilds
		p.nodeInfo[i].OffsetOfChild = nodeinfos[i].OffsetOfChild

		util.Log("PuCtrl.SetNodeInfo(): NodeInfo[%d] = Addr=%s:%d, NumChild=%d, ChildOffset=%d",
			i, util.Int2ip(p.nodeInfo[i].IP), p.nodeInfo[i].Port, p.nodeInfo[i].NumOfChilds, p.nodeInfo[i].OffsetOfChild)
	}
}

func (p *PuCtrl) getAncestorInfo() []packet.NodeInfo {
	// Find my address and parent address from nodeInfo
	var myInfo packet.NodeInfo
	var parentInfo packet.NodeInfo
	i := 0
	for _, node := range p.nodeInfo {
		nodeAddr := util.Int2ip(node.IP)
		if nodeAddr == Conf.MY_IP_ADDRS[0] {
			myInfo = node
			break
		}
		i++
	}

	if i-1 <= 0 {
		panic("PuCtrl.PushToPull(): No parent node found!")
	}

	// Find parent address
	for j := 0; j < i; j++ {
		node := p.nodeInfo[j]
		if node.OffsetOfChild <= byte(i) && byte(i) < node.OffsetOfChild+node.NumOfChilds {
			parentInfo = node
			break
		}
	}

	util.Log("PuCtrl.PushToPull(): MyAddr=%s:%d, ParentAddr=%s:%d, i=%d",
		util.Int2ip(myInfo.IP), myInfo.Port, util.Int2ip(parentInfo.IP), parentInfo.Port, i)

	// Extract ancestor nodes from nodeInfo
	ancestorInfos := make([]packet.NodeInfo, i)
	ancestorInfos[0] = myInfo
	ancestorInfos[0].NumOfChilds = byte(i - 1)
	ancestorInfos[0].OffsetOfChild = 1
	k := 1
	for j := 0; j < i-1; j++ {
		if parentInfo.IP != p.nodeInfo[j].IP {
			ancestorInfos[k] = p.nodeInfo[j]
			ancestorInfos[k].NumOfChilds = 0
			ancestorInfos[k].OffsetOfChild = 0
			k++
		}
	}

	return ancestorInfos
}

func (p *PuCtrl) PushToPull() {
	ancestorInfos := p.getAncestorInfo()

	// Connect to other nodes
	p.Connect(ancestorInfos, byte(PULL_SESSION))

	// Find block
	p.FindBlock(uint32(p.expectedBlockNumber))
}

func (p *PuCtrl) Close(blockNumber uint32) {
	numOwnerSessions := 0
	for _, session := range p.sessionMap {
		if session.sessionOwner {
			numOwnerSessions++
		}
	}

	if numOwnerSessions == len(p.sessionMap) {
		// Root node sends FIN packet to all sessions
		util.Log("PuCtrl.Close(): Sending FIN Packet (root node), numOwnerSessions=%d", numOwnerSessions)
		for _, session := range p.sessionMap {
			if session.sessionOwner {
				session.sendFinPacket(blockNumber)
			}
		}

		// Wait for FIN ACK Packets
		for i := 0; i < numOwnerSessions; i++ {
			<-p.chFinAckReceived
		}
		util.Log("PuCtrl.Close(): Received all FIN ACK packets (root node)")

	} else if numOwnerSessions < len(p.sessionMap) {
		// Child nodes wait for FIN packet
		util.Log("PuCtrl.Close(): Waiting for FIN Packet (child node)")
		lastBlocknumber := <-p.chFinReceived

		if numOwnerSessions > 0 {
			// Child nodes not a leaf send FIN packet when FIN packet is received
			util.Log("PuCtrl.Close(): Sending FIN Packet (child node but not leaf), numOwnerSessions=%d", numOwnerSessions)
			for _, session := range p.sessionMap {
				if session.sessionOwner {
					session.sendFinPacket(lastBlocknumber)
				}
			}

			// Wait for FIN ACK Packets
			for i := 0; i < numOwnerSessions; i++ {
				<-p.chFinAckReceived
			}
			util.Log("PuCtrl.Close(): Received all FIN ACK packets (child node but not leaf)")

			// Send FIN ACK Packet to all sessions not owned by this peer
			for _, session := range p.sessionMap {
				if !session.sessionOwner {
					session.sendFinAckPacket(lastBlocknumber)
				} else {
					// session.Close()
					// p.mutexSessionMap.Lock()
					// delete(p.sessionMap, session.sessionID)
					// p.mutexSessionMap.Unlock()
				}
			}
			//p.chStopListen <- true
			time.Sleep(100 * time.Millisecond)
		} else if numOwnerSessions == 0 {
			// Leaf node sends FIN ACK packet when FIN packet is received
			util.Log("PuCtrl.Close(): Sending FIN ACK packet (leaf node)")
			for _, session := range p.sessionMap {
				if !session.sessionOwner {
					session.sendFinAckPacket(lastBlocknumber)
					// session.Close()
					// p.mutexSessionMap.Lock()
					// delete(p.sessionMap, session.sessionID)
					// p.mutexSessionMap.Unlock()
				}
			}
			//p.chStopListen <- true
			time.Sleep(100 * time.Millisecond)
		}
	}

	// // Close all sessions
	// for _, session := range p.sessionMap {
	// 	session.Close()
	// }

	// // Close the channel for listening
	// if p.chStopListen != nil {
	// 	p.chStopListen <- true
	// 	p.chStopListen = nil
	// }

	// util.Log("PuCtrl.Close(): Closed PuCtrl")
}
