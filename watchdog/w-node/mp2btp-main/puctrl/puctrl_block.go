package puctrl

import (
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/MCNL-HGU/mp2btp/puctrl/packet"
	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

type Block struct {
	blockNumber           uint32
	blockSize             uint32
	receivedSegmentNumber int
	readSegmentNumber     int
	receivedBytes         uint32
	readBytes             uint32
	segmentList           []*Segment
	lock                  *sync.Mutex
	condNewData           *sync.Cond
	condSegment           *sync.Cond
	// expectedSegmentNumber int
}

func CreateBlockBuffer(number uint32, size uint32) *Block {
	b := Block{
		blockNumber:           number,
		blockSize:             size,
		receivedSegmentNumber: -1,
		readSegmentNumber:     0,
		receivedBytes:         0,
		readBytes:             0,
		segmentList:           make([]*Segment, 0),
	}

	b.lock = &sync.Mutex{}
	b.condNewData = sync.NewCond(b.lock)
	b.condSegment = sync.NewCond(b.lock)

	util.Log("Created Block Number=%d", b.blockNumber)

	return &b
}

func (b *Block) FillBlockFromFile(blockFileName string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	// Set block size
	stat, err := os.Stat(blockFileName)
	if err != nil {
		panic(err)
	}
	b.blockSize = uint32(stat.Size())

	// Open file and write to buffer
	fi, err := os.Open(blockFileName)
	if err != nil {
		panic(err)
	}

	segmentSize := uint32(0)
	numSegment := int(math.Ceil(float64(b.blockSize) / float64(Conf.SEGMENT_SIZE)))

	for idx := 0; idx < int(numSegment); idx++ {
		// Create a segment
		if b.blockSize-b.receivedBytes >= Conf.SEGMENT_SIZE {
			segmentSize = Conf.SEGMENT_SIZE
		} else {
			segmentSize = b.blockSize - b.receivedBytes
		}

		startDataNumber := b.receivedBytes / packet.PAYLOAD_SIZE
		numData := uint32(math.Ceil(float64(segmentSize) / float64(packet.PAYLOAD_SIZE)))
		endDataNumber := startDataNumber + numData - 1
		lastSegment := (b.receivedBytes+segmentSize == b.blockSize)

		segment, _ := CreateSegment(b.blockNumber, uint32(idx), startDataNumber, endDataNumber, segmentSize, 0, lastSegment)
		b.PushSegment(segment)

		// Fill segment
		readBytes := uint32(0)
		buf := make([]byte, packet.PAYLOAD_SIZE)
		for dataNumber := startDataNumber; dataNumber <= endDataNumber; dataNumber++ {
			_, err := fi.Read(buf) // Read data from block file
			if err != nil {
				panic(err)
			}

			// Push to segment
			pushLen, endOfSegment := segment.PushData(0, dataNumber, buf)
			readBytes += uint32(pushLen)
			if endOfSegment {
				break
			}
		}

		b.receivedBytes += readBytes
	}

	if b.receivedBytes != b.blockSize {
		panic(fmt.Sprintf("Block.FillBlockFromFile(): BlockNumber=%d, blockSize(%d) is not equal to filled size(%d)! ",
			b.blockNumber, b.blockSize, b.receivedBytes))
	}

	util.Log("Block.FillBlockFromFile(): BlockNumber=%d, blockSize=%d, numSegment=%d, receivedBytes=%d ",
		b.blockNumber, b.blockSize, numSegment, b.receivedBytes)
}

func (b *Block) PushSegment(segment *Segment) {
	// b.lock.Lock()
	// defer b.lock.Unlock()
	b.segmentList = append(b.segmentList, segment)
}

func (b *Block) WriteBlockData(sessionID uint32, dataNumber uint32, data []byte) bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	// Calculate segment number with data number
	startOffset := dataNumber * packet.PAYLOAD_SIZE
	segmentNumber := int(math.Floor(float64(startOffset) / float64(Conf.SEGMENT_SIZE)))

	if segmentNumber+1 <= b.GetLength() {
		pushLen, endOfSegment := b.segmentList[segmentNumber].PushData(sessionID, dataNumber, data)
		if pushLen == 0 {
			//util.Log("Block.WriteBlockData(): Fail to write block data! (dataNumber=%d)", dataNumber)
		} else if pushLen < 0 {
			util.Log("Block.WriteBlockData(): Block data duplicated! (dataNumber=%d)", dataNumber)
		} else {
			b.receivedBytes += uint32(pushLen)
		}
		b.condNewData.Signal()
		b.condSegment.Signal()
		return endOfSegment
	} else {
		util.Log("Block.WriteBlockData(): segmentNumber+1 (%d) is equal or greater than block length (%d) ", segmentNumber+1, b.GetLength())
		return false
	}
}

func (b *Block) WriteBlockFecData(sessionID uint32, fecDataNumber uint32, data []byte) bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	// Calculate segment number with data number
	startOffset := fecDataNumber * packet.PAYLOAD_SIZE
	segmentNumber := int(math.Floor(float64(startOffset) / float64(Conf.FEC_SEGMENT_SIZE)))

	if segmentNumber+1 <= b.GetLength() {
		// if FEC data is decoded succesfully, endOfSegment is true
		pushLen, endOfSegment := b.segmentList[segmentNumber].PushFecData(sessionID, fecDataNumber, data)
		if pushLen == 0 {
			//util.Log("Block.WriteBlockData(): Fail to write block data! (dataNumber=%d)", dataNumber)
		} else if pushLen < 0 {
			util.Log("Block.WriteBlockFecData(): Block FEC data duplicated! (dataNumber=%d)", fecDataNumber)
		}

		if endOfSegment {
			b.receivedBytes += uint32(b.segmentList[segmentNumber].segmentSize)
			b.condNewData.Signal()
		}
		//b.condSegment.Signal()
		return endOfSegment
	} else {
		util.Log("Block.WriteBlockFecData(): segmentNumber+1 (%d) is equal or greater than block length (%d) ", segmentNumber+1, b.GetLength())
		return false
	}
}

// ReadBlockData() is needed for Receive() called by application
func (b *Block) ReadBlockData(buf []byte) (int, bool) {
	b.lock.Lock()
	defer b.lock.Unlock()
	for b.receivedBytes-b.readBytes == 0 {
		b.condNewData.Wait() // Wait until new data is received
	}

	readLen := uint32(0)
	bufLen := uint32(len(buf))
	unreadDataLen := b.receivedBytes - b.readBytes
	if unreadDataLen >= bufLen {
		readLen = bufLen
	} else {
		readLen = unreadDataLen
	}

	// read data
	readBytes, endOfSegment := b.segmentList[b.readSegmentNumber].PopData(buf, b.readBytes, b.readBytes+readLen)
	b.readBytes += uint32(readBytes)
	if endOfSegment {
		b.readSegmentNumber++
	}

	return readBytes, endOfSegment
}

// ReadBlockDataByDataNumber() is needed for SendBlockData() called at sending peer
func (b *Block) ReadBlockDataByDataNumber(dataNumber uint32, buf []byte) (int, bool) {
	b.lock.Lock()
	defer b.lock.Unlock()

	startOffset := dataNumber * packet.PAYLOAD_SIZE
	endOffset := uint32(0)
	if b.blockSize-startOffset >= packet.PAYLOAD_SIZE {
		endOffset = startOffset + packet.PAYLOAD_SIZE
	} else {
		endOffset = b.blockSize
	}

	for b.receivedBytes < endOffset {
		b.condSegment.Wait() // Wait until the segment is received
	}

	// Calculate segment number with data number
	segmentNumber := int(math.Floor(float64(startOffset) / float64(Conf.SEGMENT_SIZE)))

	// read data (In this case, we don't update b.readBytes)
	readBytes, endOfSegment := b.segmentList[segmentNumber].PopData(buf, startOffset, endOffset)

	return readBytes, endOfSegment
}

// ReadBlockFecDataByDataNumber() is needed for SendBlockData() called at sending peer and PULL_MODE_MP_FEC
func (b *Block) ReadBlockFecDataByDataNumber(fecDataNumber uint32, buf []byte) (int, bool) {
	b.lock.Lock()
	defer b.lock.Unlock()

	startOffset := fecDataNumber * packet.PAYLOAD_SIZE
	endOffset := startOffset + packet.PAYLOAD_SIZE

	// Calculate segment number with data number
	segmentNumber := int(math.Floor(float64(startOffset) / float64(Conf.FEC_SEGMENT_SIZE)))

	// read data (In this case, we don't update b.readBytes)
	readBytes, endOfSegment := b.segmentList[segmentNumber].PopFecData(buf, startOffset, endOffset)

	return readBytes, endOfSegment
}

// FecEncodeBlockData() encodes the segment and returns the startFecDataNumber and the endFecDataNumber
func (b *Block) FecEncodeBlockData(dataNumber uint32, fecSeedNumber uint32) (uint32, uint32) {
	b.lock.Lock()
	defer b.lock.Unlock()

	// dataNumber is data number in source segment, not encoded segement
	startOffset := dataNumber * packet.PAYLOAD_SIZE
	endOffset := uint32(0)
	if b.blockSize-startOffset >= packet.PAYLOAD_SIZE {
		endOffset = startOffset + packet.PAYLOAD_SIZE
	} else {
		endOffset = b.blockSize
	}

	for b.receivedBytes < endOffset {
		//b.condSegment.Wait() // Wait until the segment is received
	}

	// Calculate segment number with data number
	segmentNumber := int(math.Floor(float64(startOffset) / float64(Conf.SEGMENT_SIZE)))

	// Encode segment
	startFecDataNumber, endFecDataNumber := b.segmentList[segmentNumber].FecEncodeData(fecSeedNumber)

	return startFecDataNumber, endFecDataNumber
}

func (b *Block) IsEmpty() bool {
	return (len(b.segmentList) == 0)
}

func (b *Block) GetLength() int {
	return len(b.segmentList)
}
