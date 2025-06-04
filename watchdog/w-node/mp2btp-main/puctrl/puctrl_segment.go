package puctrl

import (
	"fmt"

	"github.com/MCNL-HGU/mp2btp/fec/lt"
	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

type Segment struct {
	//mutex              sync.Mutex
	blockNumber        uint32
	index              uint32
	numData            uint32
	startDataNumber    uint32
	endDataNumber      uint32 // segment covers this data number
	expectedDataNumber uint32
	segmentSize        uint32 // size of source data
	dataSize           []uint32
	buffer             []byte
	fec                *lt.LT
	numFecData         uint32
	numFecDecFail      uint32
	startFecDataNumber uint32
	endFecDataNumber   uint32 // FEC buffer covers this data number
	fecSegmentSize     uint32
	fecSeedNumber      uint32
	fecDataSize        []byte
	fecBuffer          []byte
	stats              *Stats
	// // startFecDataNumber uint32
	// // endFecDataNumber   uint32
	// recvFecBuffer   []byte   // for receive buffer
	// recvFecDataSize []uint32 // for symbol map
	// //FecFinish          bool
	// fecDecStart bool
}

func CreateSegment(block uint32, idx uint32, start uint32, end uint32, size uint32, seed uint32, last bool) (*Segment, error) {
	g := Segment{
		blockNumber:        block,
		index:              idx,
		startDataNumber:    start,
		endDataNumber:      end,
		segmentSize:        size,
		expectedDataNumber: start,
		numData:            0,
		buffer:             make([]byte, size),
		dataSize:           make([]uint32, end-start+1),
		fec:                nil,
	}

	for i := 0; i < int(end-start+1); i++ {
		g.dataSize[i] = 0
	}

	// FEC
	g.fec = lt.CreateLTCode(packet.PAYLOAD_SIZE, Conf.SEGMENT_SIZE/packet.PAYLOAD_SIZE, float32(Conf.SEGMENT_SIZE)/float32(Conf.FEC_SEGMENT_SIZE), 0.95) //0.90
	g.numFecData = 0
	g.numFecDecFail = 0
	g.fecSegmentSize = g.fec.GetNumEncSyms() * packet.PAYLOAD_SIZE
	g.fecSeedNumber = seed
	g.startFecDataNumber = (g.index * g.fecSegmentSize) / packet.PAYLOAD_SIZE
	g.endFecDataNumber = g.startFecDataNumber + g.fec.GetNumEncSyms() - 1
	g.fecBuffer = make([]byte, g.fecSegmentSize)
	g.fecDataSize = make([]byte, g.endFecDataNumber-g.startFecDataNumber+1)

	for i := 0; i < int(g.endFecDataNumber-g.startFecDataNumber+1); i++ {
		g.fecDataSize[i] = 0
	}

	// Create Stats
	g.stats = CreateStats(g.blockNumber, g.index)
	g.stats.SetSegmentResponseTime(false)
	g.stats.SetSegmentSize(g.segmentSize)
	g.stats.SetPullMode(Conf.PULL_MODE)

	return &g, nil
}

func (g *Segment) PushData(sessionID uint32, dataNumber uint32, data []byte) (int, bool) {
	if dataNumber >= g.startDataNumber && dataNumber <= g.endDataNumber {
		dataIdx := dataNumber - g.startDataNumber    // data number
		startOffset := dataIdx * packet.PAYLOAD_SIZE // bytes

		pushLen := uint32(0)
		dataLen := uint32(len(data))
		unreadDataLen := g.segmentSize - startOffset
		if unreadDataLen >= dataLen {
			pushLen = dataLen
		} else {
			pushLen = unreadDataLen
		}
		endOffset := startOffset + pushLen // bytes

		if g.dataSize[dataIdx] == pushLen {
			g.stats.IncreaseDuplicatedNumData(sessionID) // stats
			return int(-1), false
		}

		copy(g.buffer[startOffset:endOffset], data)
		g.dataSize[dataIdx] = pushLen
		g.numData++

		// Update expectedDataNumber and lastRecvDataOffset
		if dataNumber == g.expectedDataNumber {
			for g.dataSize[dataIdx] > 0 {
				// if g.expectedDataNumber == g.endDataNumber {
				// 	g.Complete = true
				// }
				g.expectedDataNumber++
				dataIdx = g.expectedDataNumber - g.startDataNumber
				if dataIdx == uint32(len(g.dataSize)) {
					break
				}
			}
		}

		// stats
		g.stats.IncreaseReceivedBytes(sessionID, pushLen)
		g.stats.IncreaseReceivedNumData(sessionID)
		if endOffset == g.segmentSize {
			g.stats.SetSegmentResponseTime(true)
		}

		return int(endOffset - startOffset), (endOffset == g.segmentSize)
	}

	return int(0), false
}

// PushFecData() returns the length of pushed FEC data and the decoding success status
func (g *Segment) PushFecData(sessionID uint32, dataNumber uint32, data []byte) (int, bool) {
	if (dataNumber >= g.startFecDataNumber && dataNumber <= g.endFecDataNumber) &&
		!(g.numFecData == g.fec.GetNumEncSyms() && g.numFecDecFail == 0) {
		dataIdx := dataNumber - g.startFecDataNumber // data number
		startOffset := dataIdx * packet.PAYLOAD_SIZE // bytes

		pushLen := uint32(0)
		dataLen := uint32(len(data))
		unreadDataLen := g.fecSegmentSize - startOffset
		if unreadDataLen >= dataLen {
			pushLen = dataLen
		} else {
			pushLen = unreadDataLen
		}
		endOffset := startOffset + pushLen // bytes

		if g.fecDataSize[dataIdx] == 1 {
			g.stats.IncreaseDuplicatedNumData(sessionID) // stats
			return int(-1), false
		}

		copy(g.fecBuffer[startOffset:endOffset], data)
		g.fecDataSize[dataIdx] = 1
		g.numFecData++

		// Check FEC decoding!
		decodingSuccess := g.FecDecodeData(g.fecSeedNumber)

		// stats
		g.stats.IncreaseReceivedBytes(sessionID, pushLen)
		g.stats.IncreaseReceivedNumData(sessionID)
		if decodingSuccess {
			g.stats.SetSegmentResponseTime(true)
		}

		return int(endOffset - startOffset), decodingSuccess
	}

	return int(0), false
}

func (g *Segment) PopData(buf []byte, startOffsetInBlock uint32, endOffsetInBlock uint32) (int, bool) {
	startOffset := startOffsetInBlock - (g.startDataNumber * packet.PAYLOAD_SIZE)
	endOffset := endOffsetInBlock - (g.startDataNumber * packet.PAYLOAD_SIZE)
	if endOffset > g.segmentSize {
		endOffset = g.segmentSize
	}

	// util.Log("Segment.PopData(): offset in block=%d-%d (offset in segment=%d-%d), size=%d ",
	// 	startOffsetInBlock, endOffsetInBlock, startOffset, endOffset, g.segmentSize)s

	copy(buf, g.buffer[startOffset:endOffset])

	return int(endOffset - startOffset), (endOffset == g.segmentSize)
}

func (g *Segment) PopFecData(buf []byte, startOffsetInBlock uint32, endOffsetInBlock uint32) (int, bool) {
	startOffset := startOffsetInBlock - (g.startFecDataNumber * packet.PAYLOAD_SIZE)
	endOffset := endOffsetInBlock - (g.startFecDataNumber * packet.PAYLOAD_SIZE)
	if endOffset > g.fecSegmentSize {
		endOffset = g.fecSegmentSize
	}

	copy(buf, g.fecBuffer[startOffset:endOffset])

	return int(endOffset - startOffset), (endOffset == g.fecSegmentSize)
}

func (g *Segment) FecEncodeData(fecSeedNumber uint32) (uint32, uint32) {
	g.stats.SetFecEncodingDelay(false)
	g.fecBuffer = g.fec.Encode(g.buffer, fecSeedNumber)
	g.stats.SetFecEncodingDelay(true)
	EncDelayTot += g.stats.fecEncodingDelay

	if g.fecBuffer == nil {
		panic(fmt.Sprintf("Segment.FecEncodeData(): FEC Encoding fail! (segmentNumber=%d)", g.index))
	}

	util.Log("Segment.FecEncodeData(): FEC Encoding success! (segmentNumber=%d, DataNumber=%d-%d, FecDataNumber=%d-%d, EncDelay=%.4f, EncDelayTot=%.4f)",
		g.index, g.startDataNumber, g.endDataNumber, g.startFecDataNumber, g.endFecDataNumber, g.stats.fecEncodingDelay, EncDelayTot)

	return g.startFecDataNumber, g.endFecDataNumber
}

func (g *Segment) FecDecodeData(fecSeedNumber uint32) bool {
	decStep := g.numFecDecFail * uint32(float32(g.fec.GetNumSymsForDec())*0.1)
	if g.numFecData+decStep > g.fec.GetNumSymsForDec() {
		g.stats.SetFecDecodingDelay(false)
		g.buffer = g.fec.Decode(g.fecBuffer, g.fecDataSize, fecSeedNumber)
		g.stats.SetFecDecodingDelay(true)
		DecDelayTot += g.stats.fecDecodingDelay

		if g.buffer != nil {
			util.Log("Segment.FecDecodeData(): FEC Decoding success! (segmentNumber=%d, numFecData=%d, decStep=%d, numDecFail=%d, DecDelay=%.4f, DecDelayTot=%.4f)",
				g.index, g.numFecData, decStep, g.numFecDecFail, g.stats.fecDecodingDelay, DecDelayTot)
			g.numFecData = g.fec.GetNumEncSyms()
			g.numFecDecFail = 0
			return true
		} else {
			g.numFecDecFail++
			util.Log("Segment.FecDecodeData(): FEC Decoding Fail! (segmentNumber=%d, numFecData=%d, decStep=%d, numDecFail=%d)",
				g.index, g.numFecData, decStep, g.numFecDecFail)
		}
	} else if g.numFecData >= g.fec.GetNumEncSyms() {
		g.numFecDecFail++
		panic(fmt.Sprintf("Segment.FecDecodeData(): FEC Decoding Fail!!! (Last chance)(segmentNumber=%d, numFecData=%d, decStep=%d, numDecFail=%d)",
			g.index, g.numFecData, decStep, g.numFecDecFail))
	}

	return false
}
