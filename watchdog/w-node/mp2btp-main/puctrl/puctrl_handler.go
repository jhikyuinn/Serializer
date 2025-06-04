package puctrl

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

// Send a Block Info Packet
func (p *PuCtrl) SendBlockInfo(sessionID uint32, blockNumber uint32) {
	// Checks whether p.blockMap contains the requested block
	block, exists := p.blockMap.Get(blockNumber)
	if exists {
		p.sessionMap[sessionID].SendBlockInfoPacket(blockNumber, block.blockSize, uint32(PEER_HAS_BLOCK))
	} else {
		p.sessionMap[sessionID].SendBlockInfoPacket(blockNumber, 0, uint32(PEER_HAS_NO_BLOCK))
	}
}

// Send a Block Request
func (p *PuCtrl) SendBlockRequest(sessionType byte, sessionID uint32, pkt *packet.BlockInfoPacket) {
	// Update block number
	if p.expectedBlockNumber < pkt.BlockNumber {
		p.expectedBlockNumber = pkt.BlockNumber
	}

	// Count the number of peers which has requested block
	numBlockInfoReceived := uint16(0)
	numBlockOwners := uint32(0)
	if sessionType == PULL_SESSION {
		for id := range p.sessionMap {
			if p.sessionMap[id].sessionType == PULL_SESSION {
				if p.sessionMap[id].blockStatus == PEER_HAS_BLOCK {
					numBlockInfoReceived++
					numBlockOwners++
				} else if p.sessionMap[id].blockStatus == PEER_HAS_NO_BLOCK {
					numBlockInfoReceived++
				}
			}
		}
	}

	// If Block Info Packets are arrived from all peers
	if (sessionType == PUSH_SESSION) ||
		(sessionType == PULL_SESSION && numBlockOwners > 0 && p.numPullSession == numBlockInfoReceived) {
		// Create a block buffer if first segment is received
		block, exists := p.blockMap.Get(pkt.BlockNumber)
		if !exists {
			block = CreateBlockBuffer(pkt.BlockNumber, pkt.BlockSize)
			p.blockMap.Set(pkt.BlockNumber, block)
		}

		// Create a bounded channel for a semaphore
		if sessionType == PUSH_SESSION {
			p.semPush = make(chan uint32, 1)
			p.semPush <- sessionID
			go p.blockRequest(sessionType, pkt.BlockNumber, pkt.BlockSize, block.receivedBytes)
		} else { // PULL_SESSION
			switch Conf.PULL_MODE {
			case PULL_MODE_SP:
				// Push session ID into semaphore
				p.semPull = make(chan uint32, numBlockOwners)
				for id := range p.sessionMap {
					if p.sessionMap[id].sessionType == sessionType &&
						p.sessionMap[id].blockStatus == PEER_HAS_BLOCK {
						p.semPull <- id
					}
				}
				go p.blockRequest(sessionType, pkt.BlockNumber, pkt.BlockSize, block.receivedBytes)
			case PULL_MODE_MP, PULL_MODE_MP_FEC:
				// Push segment number into semaphore
				p.semPull = make(chan uint32, 1)
				p.semPull <- 0
				go p.blockRequestMP(sessionType, pkt.BlockNumber, pkt.BlockSize, block.receivedBytes, numBlockOwners)
			}
		}
	}
}

// Send a Block Request Packet (Push Session or Pull SP mode)
func (p *PuCtrl) blockRequest(sessionType byte, blockNumber uint32, blockSize uint32, offset uint32) {
	// Repeat until the entire block is requested
	idx := uint32(0)
	size := uint32(0)
	for offset < blockSize {
		selectedSessionID := uint32(0)
		if sessionType == PUSH_SESSION {
			selectedSessionID = <-p.semPush
		} else {
			selectedSessionID = <-p.semPull
		}

		// Create a segment
		if blockSize-offset >= Conf.SEGMENT_SIZE {
			size = Conf.SEGMENT_SIZE
		} else {
			size = blockSize - offset
		}

		// Add a segment for receiving
		startDataNumber := offset / packet.PAYLOAD_SIZE
		numData := uint32(math.Ceil(float64(size) / float64(packet.PAYLOAD_SIZE)))
		endDataNumber := startDataNumber + numData - 1
		lastSegment := (offset+size == blockSize)
		p.AddSegment(blockNumber, idx, startDataNumber, endDataNumber, size, 0, lastSegment)

		// Request (set block number and range)
		p.sessionMap[selectedSessionID].blockStatus = PEER_DOWNLOADING
		p.sessionMap[selectedSessionID].SendBlockRequestPacket(blockNumber, startDataNumber, endDataNumber, 0, 0)

		offset += size
		idx++

		// For block data forwarding to child nodes
		if sessionType == PUSH_SESSION && idx == 2 { // when the first two segments are recieved, send block info packet to child nodes
			for sessionID := range p.sessionMap {
				// selectedSessionID indicates the session to parent node
				if selectedSessionID != sessionID {
					if p.sessionMap[sessionID].sessionType == PUSH_SESSION {
						p.SendBlockInfo(sessionID, blockNumber)
					}
				}
			}
		}
	}
}

// Send a Block Request Packet (Pull MP mode or Pull MP with FEC mode)
func (p *PuCtrl) blockRequestMP(sessionType byte, blockNumber uint32, blockSize uint32, offset uint32, numBlockOwners uint32) {
	// We assume that numBlockOwners is less than 8
	if numBlockOwners >= 8 {
		panic("numBlockOwners >= 8")
	}

	// Repeat until the entire block is requested
	idx := uint32(0)
	size := uint32(0)
	for offset < blockSize {
		<-p.semPull

		// Create a segment
		if blockSize-offset >= Conf.SEGMENT_SIZE {
			size = Conf.SEGMENT_SIZE
		} else {
			size = blockSize - offset
		}

		// Add a segment for receiving
		startDataNumber := offset / packet.PAYLOAD_SIZE
		numData := uint32(math.Ceil(float64(size) / float64(packet.PAYLOAD_SIZE)))
		endDataNumber := startDataNumber + numData - 1
		lastSegment := (offset+size == blockSize)
		fecSeedNumber := uint32(0)
		if Conf.PULL_MODE == PULL_MODE_MP_FEC {
			fecSeedNumber = rand.Uint32()
		}
		p.AddSegment(blockNumber, idx, startDataNumber, endDataNumber, size, fecSeedNumber, lastSegment)

		// Request (set block number and range)
		mpInfo := p.setPullModePeerInfo(Conf.PULL_MODE, byte(numBlockOwners))
		for id := range p.sessionMap {
			if p.sessionMap[id].sessionType == PULL_SESSION {
				p.sessionMap[id].blockStatus = PEER_DOWNLOADING
				p.sessionMap[id].SendBlockRequestPacket(blockNumber, startDataNumber, endDataNumber, mpInfo, fecSeedNumber)
				mpInfo++
			}
		}

		offset += size
		idx++
	}
}

// Send Block Data
func (p *PuCtrl) SendBlockData(sessionID uint32, pkt *packet.BlockRequestPacket) {
	if pkt.MpInfo == 0 {
		go p.sendBlockDataSP(sessionID, pkt)
	} else {
		// Stop the existing goroutines for Pull Mode
		if p.chStopTransmission != nil {
			close(p.chStopTransmission)
		}
		p.chStopTransmission = make(chan bool)

		pullMode := p.getPullMode(pkt.MpInfo)
		if pullMode == PULL_MODE_MP {
			go p.sendBlockDataMP(sessionID, pkt, p.chStopTransmission)
		} else if pullMode == PULL_MODE_MP_FEC {
			go p.sendBlockDataMPFEC(sessionID, pkt, p.chStopTransmission)
		}
	}
}

func (p *PuCtrl) sendBlockDataSP(sessionID uint32, pkt *packet.BlockRequestPacket) {
	// Send block data by data number
	i := 0
	for dataNumber := pkt.StartDataNumber; dataNumber <= pkt.EndDataNumber; dataNumber++ {
		// Read block data by data number
		data := make([]byte, packet.PAYLOAD_SIZE)
		block, exists := p.blockMap.Get(pkt.BlockNumber)
		if exists {
			block.ReadBlockDataByDataNumber(dataNumber, data)

			// Send a Block Data Packet
			p.sessionMap[sessionID].sendBlockDataPacket(pkt.BlockNumber, dataNumber, DATA_TYPE_SOURCE, data)
			i++
		} else {
			util.Log("PuSession[%d].sendBlockDataSP(): Block number %d does not exist", pkt.BlockNumber)
		}
	}
}

func (p *PuCtrl) sendBlockDataMP(sessionID uint32, pkt *packet.BlockRequestPacket, chStopTransmission <-chan bool) {
	numBlockOwners, peerId := p.getPeerInfo(pkt.MpInfo)
	util.Log("PuCtrl.sendBlockDataMP(): Block transmission start! (PeerID=%d, DataNumber=%d-%d)", peerId, pkt.StartDataNumber, pkt.EndDataNumber)

	// Send block data by data number
	for i := 0; i < int(numBlockOwners); i++ {
		startDataNumber := pkt.StartDataNumber + (peerId+uint32(i))%numBlockOwners
		for dataNumber := startDataNumber; dataNumber <= pkt.EndDataNumber; dataNumber += numBlockOwners {
			// select {
			// case <-chStopTransmission:
			// 	util.Log("PuCtrl.sendBlockDataMP(): Block transmission stop! (PeerID=%d, DataNumber=%d-%d, i=%d, dataNumber=%d)",
			// 		peerId, pkt.StartDataNumber, pkt.EndDataNumber, i, dataNumber)
			// 	return
			// default:
			// Read block data by data number
			data := make([]byte, packet.PAYLOAD_SIZE)
			block, exists := p.blockMap.Get(pkt.BlockNumber)
			if exists {
				block.ReadBlockDataByDataNumber(dataNumber, data)

				// Send a Block Data Packet
				p.sessionMap[sessionID].sendBlockDataPacket(pkt.BlockNumber, dataNumber, DATA_TYPE_SOURCE, data)
			}
			// }
		}
	}

	util.Log("PuCtrl.sendBlockDataMP(): Block transmission Finish! (PeerID=%d, DataNumber=%d-%d)", peerId, pkt.StartDataNumber, pkt.EndDataNumber)
}

func (p *PuCtrl) sendBlockDataMPFEC(sessionID uint32, pkt *packet.BlockRequestPacket, chStopTransmission <-chan bool) {
	numBlockOwners, peerId := p.getPeerInfo(pkt.MpInfo)
	util.Log("PuCtrl.sendBlockDataMPFEC(): Block transmission start! (PeerID=%d, DataNumber=%d-%d)", peerId, pkt.StartDataNumber, pkt.EndDataNumber)

	// FEC Encoding
	block, _ := p.blockMap.Get(pkt.BlockNumber)
	startFecDataNumber, endFecDataNumber := block.FecEncodeBlockData(pkt.StartDataNumber, pkt.FecSeedNumber)

	// Send block data by data number
	for i := 0; i < int(numBlockOwners); i++ {
		startDataNumber := startFecDataNumber + (peerId+uint32(i))%numBlockOwners
		for fecDataNumber := startDataNumber; fecDataNumber <= endFecDataNumber; fecDataNumber += numBlockOwners {
			select {
			case <-chStopTransmission:
				util.Log("PuCtrl.sendBlockDataMPFEC(): Block transmission stop! (PeerID=%d, FecDataNumber=%d-%d, i=%d, dataNumber=%d)",
					peerId, startFecDataNumber, endFecDataNumber, i, fecDataNumber)
				return
			default:
				// Read block data by data number
				data := make([]byte, packet.PAYLOAD_SIZE)
				block, exists := p.blockMap.Get(pkt.BlockNumber)
				if exists {
					block.ReadBlockFecDataByDataNumber(fecDataNumber, data)

					// Send a Block Data Packet
					p.sessionMap[sessionID].sendBlockDataPacket(pkt.BlockNumber, fecDataNumber, DATA_TYPE_FEC, data)
				}
			}
		}
		// if i == 1 { // Tricky
		// 	break
		// }

	}

	util.Log("PuCtrl.sendBlockDataMPFEC(): Block transmission Finish! (PeerID=%d, DataNumber=%d-%d)", peerId, pkt.StartDataNumber, pkt.EndDataNumber)
}

// Receive Block Data
func (p *PuCtrl) ReceiveBlockData(sessionID uint32, pkt *packet.BlockDataPacket) {
	block, exists := p.blockMap.Get(pkt.BlockNumber)
	if !exists {
		panic(fmt.Sprintf("PuCtrl.ReceiveBlockData(): Not found block buffer (BlockNumber=%d)", pkt.BlockNumber))
	}

	// Store received data into segment
	endOfSegment := false
	if pkt.DataType == DATA_TYPE_SOURCE {
		endOfSegment = block.WriteBlockData(sessionID, pkt.DataNumber, pkt.Data)
	} else if pkt.DataType == DATA_TYPE_FEC {
		endOfSegment = block.WriteBlockFecData(sessionID, pkt.DataNumber, pkt.Data)
	}

	// Push session ID to start block request for next segment
	if endOfSegment {
		if p.sessionMap[sessionID].sessionType == PUSH_SESSION {
			p.semPush <- sessionID
		} else {
			p.semPull <- sessionID
		}

		// segment.FecFinish = true

		util.Log("PuCtrl.ReceiveBlockData(): Segment Download Complete!!!! DataNumber=%d", pkt.DataNumber)
	}
}

// Send a Block Find Packet
func (p *PuCtrl) SendBlockFind(sessionID uint32, blockNumber uint32) {
	p.sessionMap[sessionID].sendBlockFindPacket(blockNumber)
}

// AddSegment() function adds the given segment to blockMap[] based on block number.
func (p *PuCtrl) AddSegment(blockNumber uint32, idx uint32, start uint32, end uint32, segmentSize uint32, fecSeedNumber uint32, last bool) {
	// Push segment into block buffer
	segment, _ := CreateSegment(blockNumber, idx, start, end, segmentSize, fecSeedNumber, last)
	block, exists := p.blockMap.Get(blockNumber)
	if exists {
		block.PushSegment(segment)
	}
}

// Get pull mode (highest 2bits) from MpInfo
func (p *PuCtrl) getPullMode(mpInfo byte) byte {
	return byte((mpInfo & byte(0b11000000)) >> 6)
}

// Get numBlockOwners and peerId from MpInfo
func (p *PuCtrl) getPeerInfo(mpInfo byte) (uint32, uint32) {
	numBlockOwners := uint32((mpInfo & byte(0b00111000)) >> 3)
	peerId := uint32((mpInfo & byte(0b00000111)))
	return numBlockOwners, peerId
}

// Set MpInfo by PullMode, PeerInfo, and Peer ID
func (p *PuCtrl) setPullModePeerInfo(pullMode byte, numBlockOwners byte) byte {
	mpInfo := ((0xff & pullMode) << 6) | (0xff & byte(numBlockOwners) << 3)
	return mpInfo
}
