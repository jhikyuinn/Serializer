package puctrl

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/MCNL-HGU/mp2btp/puctrl/util"
	//pc "github.com/MCNL-HGU/mp2btp/puctrl/packet"
	//equic "github.com/MCNL-HGU/Enhanced-quic"
)

var i = uint16(1)

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
	semPush             chan uint32
	semPull             chan uint32
	mutexForTimeout     sync.Mutex      // Mutext for timeout
	goodbye             bool            // For reading
	timeoutFlag         map[uint32]bool // Timeout flag (key: blockNumber)

	// New
	chStopListen       chan bool // Stop goroutine for listening
	chStopTransmission chan bool // Stop goroutine for sender's transmission
	mutexSessionMap    sync.Mutex

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
	if p.chStopListen != nil {
		util.Log("PuCtrl.Listen(): Already listening")
		return
	}
	p.chStopListen = make(chan bool)

	// Start listening
	go Listener(p.chStopListen)
}

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
		nodeAddr := fmt.Sprintf("%s:%d", util.Int2ip(nodeInfo[i].IP), 4100)
		p.childAddr = append(p.childAddr, nodeAddr)
	}
	fmt.Println("CHILDLENGTH", len(p.childAddr), p.childAddr)
	if len(p.childAddr) > 0 {
		count := 0
		for _, peerAddr := range p.childAddr {
			// My address
			myBindAddr := fmt.Sprintf("%s:%d", Conf.MY_IP_ADDRS[0], Conf.BIND_PORT+uint16(i))
			fmt.Println("📞", peerAddr, "📞", myBindAddr)

			// Connect to peer
			session := Connector(myBindAddr, peerAddr, sessionType)

			// util.Log("PuCtrl.Connect(): MyAddr=%s, PeerAddr=%s, SessionType=%d, SessionID=%d",
			// 	myBindAddr, peerAddr, sessionType, session.sessionID)

			if session == nil {
				log.Println("❌ Failed to create session")
				continue
			}

			// Send Node Info
			session.sendNodeInfoPacket(nodeInfo)

			// Send Path Info
			session.sendPathInfoPacket(Conf.MY_IP_ADDRS[0:Conf.NUM_MULTIPATH])

			// Tricky method for test
			i++
			count++
			fmt.Println("뭐죠뭐죠뭐죠", len(p.childAddr), i, count)
			if count == len(p.childAddr) || len(p.childAddr) == 0 {

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
	fmt.Println("[SESSIONMAP]", p.sessionMap)
	for sessionID := range p.sessionMap {
		fmt.Println("💰", p.sessionMap[sessionID].sessionType, PUSH_SESSION)
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

func (p *PuCtrl) SendAuditAckMsg(data []byte) error {

	isPushSessionExist := false

	fmt.Println("SendAuditAckMsgSendAuditAckMsgSendAuditAckMsgSendAuditAckMsgSendAuditAckMsgSendAuditAckMsg")
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

// KYUKYU
func (p *PuCtrl) SendRaw(sessionID uint32, data []byte) error {
	fmt.Println("🎉SendRaw", sessionID)
	session, ok := p.sessionMap[sessionID]
	if !ok {
		return fmt.Errorf("session %d does not exist", sessionID)
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	fmt.Println("🎉SendRawdataLength", uint32(len(data)), data)

	session.sendPacket(data)
	// if _, err := session.stream.Write(lenBuf); err != nil {
	// 	return fmt.Errorf("failed to write length prefix: %v", err)
	// }

	// _, err := session.stream.Write(data)
	// if err != nil {
	// 	return fmt.Errorf("failed to write data: %v", err)
	// }

	return nil
}

func (p *PuCtrl) ReceiveAuditMsg(sessionID uint32, pkt *packet.AuditDataPacket) {
	fmt.Println("ReceiveAuditMsgReceiveAuditMsgReceiveAuditMsgReceiveAuditMsgReceiveAuditMsgReceiveAuditMsg")
	p.AuditBroadcaster.Broadcast(pkt)
}

func (p *PuCtrl) ReceiveAuditMsgAck(sessionID uint32, pkt *packet.AuditDataAckPacket) {
	fmt.Println("ReceiveAuditMsgAckReceiveAuditMsgAckReceiveAuditMsgAckReceiveAuditMsgAckReceiveAuditMsgAck")
	p.AuditMsgAckChan <- pkt
}

// KYUKYU

func (p *PuCtrl) ReceiveRaw() ([]byte, error) {
	for sessionID := range p.sessionMap {
		session, ok := p.sessionMap[sessionID]
		if !ok {
			return nil, fmt.Errorf("session %d does not exist", sessionID)
		}

		// 먼저 길이 prefix (4바이트) 읽기
		lenBuf := make([]byte, 4)
		_, err := io.ReadFull(session.stream, lenBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to read length prefix: %v", err)
		}
		dataLen := binary.BigEndian.Uint32(lenBuf)

		// 실제 데이터 읽기
		data := make([]byte, dataLen)
		_, err = io.ReadFull(session.stream, data)
		if err != nil {
			return nil, fmt.Errorf("failed to read data: %v", err)
		}

		return data, nil
	}
	return nil, fmt.Errorf("no active sessions")
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
		if peerAddr[i].NumChild == 0 {
			nodeInfo[i].Port = 4100
		}
		if peerAddr[i].NumChild != 0 {
			nodeInfo[i].Port = 4000
		}
		nodeInfo[i].NumOfChilds = peerAddr[i].NumChild
		nodeInfo[i].OffsetOfChild = peerAddr[i].ChildOffset

		util.Log("PuCtrl.GetNodeInfo(): NodeInfo[%d] = Addr=%s:%d, NumChild=%d, ChildOffset=%d",
			i, util.Int2ip(nodeInfo[i].IP), nodeInfo[i].Port, nodeInfo[i].NumOfChilds, nodeInfo[i].OffsetOfChild)
	}

	return nodeInfo
}

func (pu *PuCtrl) CloseAllSessions() {
	pu.mutexSessionMap.Lock()
	defer pu.mutexSessionMap.Unlock()

	for sessionID, session := range pu.sessionMap {
		fmt.Printf("🔌 Closing session %d\n", sessionID)
		session.Close() // session.handler 내부에서 graceful하게 처리되면 더 좋음
	}
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
