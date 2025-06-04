package puctrl

import (
	"encoding/binary"
	"fmt"
	"sync"

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
	semPush             chan uint32
	semPull             chan uint32
	mutexForTimeout     sync.Mutex      // Mutext for timeout
	goodbye             bool            // For reading
	timeoutFlag         map[uint32]bool // Timeout flag (key: blockNumber)

	// New
	chStopListen       chan bool // Stop goroutine for listening
	chStopTransmission chan bool // Stop goroutine for sender's transmission
	mutexSessionMap    sync.Mutex
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
		nodeAddr := fmt.Sprintf("%s:%d", util.Int2ip(nodeInfo[i].IP), nodeInfo[i].Port)
		p.childAddr = append(p.childAddr, nodeAddr)
	}

	if len(p.childAddr) > 0 {
		i := uint16(0)
		for _, peerAddr := range p.childAddr {
			// My address
			myBindAddr := fmt.Sprintf("%s:%d", Conf.MY_IP_ADDRS[0], Conf.BIND_PORT+uint16(i))

			// Connect to peer
			session := Connector(myBindAddr, peerAddr, sessionType)

			util.Log("PuCtrl.Connect(): MyAddr=%s, PeerAddr=%s, SessionType=%d, SessionID=%d",
				myBindAddr, peerAddr, sessionType, session.sessionID)

			// Send Node Info
			session.sendNodeInfoPacket(nodeInfo)

			// Send Path Info
			session.sendPathInfoPacket(Conf.MY_IP_ADDRS[0:Conf.NUM_MULTIPATH])

			// Tricky method for test
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
		fmt.Println("🥳SendAuditMsg", sessionID)
		if p.sessionMap[sessionID].sessionType == PUSH_SESSION {
			err := p.SendRaw(sessionID, data)
			if err != nil {
				return fmt.Errorf("failed to send audit msg on session %d: %v", sessionID, err)
			}
			isPushSessionExist = true
		}
	}

	if !isPushSessionExist {
		return fmt.Errorf("SendAuditMsg(): Push session does not exist")
	}
	return nil
}

// KYUKYU
func (p *PuCtrl) SendRaw(sessionID uint32, data []byte) error {
	// fmt.Println("🎉SendRaw", sessionID)
	session, ok := p.sessionMap[sessionID]
	if !ok {
		return fmt.Errorf("session %d does not exist", sessionID)
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	// fmt.Println("🎉SendRawdataLength", uint32(len(data)))

	if _, err := session.stream.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write length prefix: %v", err)
	}

	_, err := session.stream.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %v", err)
	}
	// fmt.Println("🎉SendRawdata", data)
	// session.stream.Close()

	return nil
}

// KYUKYU
func (p *PuCtrl) ReceiveRaw(buf []byte) ([]byte, error) {
	// fmt.Println("👑ReceiveRaw", p.sessionMap)
	for sessionID := range p.sessionMap {

		session, ok := p.sessionMap[sessionID]
		if !ok {
			return nil, fmt.Errorf("session %d does not exist", sessionID)
		}
		n, err := session.stream.Read(buf)
		// fmt.Println("👑ReceiveRawdataLength", n)
		if err != nil {
			return nil, err
		}
		// fmt.Println("👑ReceiveRawdata", buf[1:n])
		// session.stream.Close()

		return buf[:n], nil
	}
	return nil, nil
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
		fmt.Println("🤎", nodeInfo)
		nodeInfo[i].IP = util.Ip2int(peerAddr[i].Addr)
		nodeInfo[i].Port = Conf.LISTEN_PORT
		nodeInfo[i].NumOfChilds = peerAddr[i].NumChild
		nodeInfo[i].OffsetOfChild = peerAddr[i].ChildOffset

		util.Log("PuCtrl.GetNodeInfo(): NodeInfo[%d] = Addr=%s:%d, NumChild=%d, ChildOffset=%d",
			i, util.Int2ip(nodeInfo[i].IP), nodeInfo[i].Port, nodeInfo[i].NumOfChilds, nodeInfo[i].OffsetOfChild)
	}

	return nodeInfo
}
