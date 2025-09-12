package puctrl

import (
	"sync"
	"time"
)

var EncDelayTot = float64(0.0)
var DecDelayTot = float64(0.0)

type Stats struct {
	blockNumber          uint32
	segmentIndex         uint32
	segmentRequestTime   time.Time
	segmentResponseTime  float64
	fecEncodingStartTime time.Time
	fecEncodingDelay     float64
	fecDecodingStartTime time.Time
	fecDecodingDelay     float64
	segmentSize          uint32
	pullMode             byte
	receivedBytes        map[uint32](uint32) // including FEC data
	receivedNumData      map[uint32](uint32) // including FEC data
	duplicatedNumData    map[uint32](uint32) // including FEC data
	mutex                sync.Mutex
}

func CreateStats(blockNo uint32, segmentIdx uint32) *Stats {
	s := Stats{
		blockNumber:       blockNo,
		segmentIndex:      segmentIdx,
		receivedBytes:     make(map[uint32]uint32, 0),
		receivedNumData:   make(map[uint32]uint32, 0),
		duplicatedNumData: make(map[uint32]uint32, 0),
		mutex:             sync.Mutex{},
	}

	return &s
}

func (s *Stats) SetSegmentResponseTime(finish bool) {
	if !finish {
		s.segmentRequestTime = time.Now()
	} else {
		elapsedTime := time.Since(s.segmentRequestTime)
		s.segmentResponseTime = elapsedTime.Seconds()
	}
}

func (s *Stats) SetSegmentSize(size uint32) {
	s.segmentSize = size
}

func (s *Stats) SetPullMode(mode byte) {
	s.pullMode = mode
}

func (s *Stats) IncreaseReceivedBytes(sessoinID uint32, receivedBytes uint32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.receivedBytes[sessoinID] += receivedBytes
}

func (s *Stats) IncreaseReceivedNumData(sessoinID uint32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.receivedNumData[sessoinID] += 1
}

func (s *Stats) IncreaseDuplicatedNumData(sessoinID uint32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.duplicatedNumData[sessoinID] += 1
}

func (s *Stats) SetFecEncodingDelay(finish bool) {
	if !finish {
		s.fecEncodingStartTime = time.Now()
	} else {
		elapsedTime := time.Since(s.fecEncodingStartTime)
		s.fecEncodingDelay = elapsedTime.Seconds()
	}
}

func (s *Stats) SetFecDecodingDelay(finish bool) {
	if !finish {
		s.fecDecodingStartTime = time.Now()
	} else {
		elapsedTime := time.Since(s.fecDecodingStartTime)
		s.fecDecodingDelay = elapsedTime.Seconds()
	}
}
