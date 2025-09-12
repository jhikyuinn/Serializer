// package main

// import (
// 	"bytes"
// 	"encoding/json"
// 	"flag"
// 	"fmt"
// 	"os"
// 	"time"

// 	"github.com/BurntSushi/toml"
// 	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
// )

// const size = 32768

// type recvResult struct {
// 	n   int
// 	err error
// 	buf []byte
// }

// func receiveWorker(pu *PuCtrl.PuCtrl, resultCh chan<- recvResult) {
// 	for {
// 		buf := make([]byte, 8192)
// 		n, err := pu.ReceiveRaw(buf)
// 		if err != nil || n <= 0 {
// 			resultCh <- recvResult{n: n, err: err}
// 			return
// 		}
// 		resultCh <- recvResult{n: n, buf: buf}
// 	}
// }

// func extractAndParseJSON(data []byte) (string, error) {
// 	// JSON 시작 위치 탐색: 일반적으로 문자열은 '"' (34), JSON 객체는 '{' (123)
// 	jsonStart := bytes.IndexAny(data, "\"{")
// 	if jsonStart == -1 {
// 		return "", fmt.Errorf("no JSON start found")
// 	}

// 	// JSON 데이터 추출
// 	jsonBytes := data[jsonStart:]

// 	// Unmarshal 시도
// 	var name string
// 	err := json.Unmarshal(jsonBytes, &name)
// 	if err != nil {
// 		return "", fmt.Errorf("json unmarshal error: %w", err)
// 	}

// 	return name, nil
// }

// func main() {

// 	configFile := flag.String("c", "push_child.toml", "Config file name")
// 	peerId := flag.Int("i", 1, "Peer ID")
// 	flag.Parse()

// 	fmt.Printf("PEER-%d: Config file name=%s \n\n\n", *peerId, *configFile)

// 	// Parse the config file
// 	var Cfg PuCtrl.Config
// 	if _, err := toml.DecodeFile(*configFile, &Cfg); err != nil {
// 		panic(err)
// 	}

// 	// Create Push/Pull Control
// 	pu := PuCtrl.CreatePuCtrl(*configFile, uint32(*peerId))

// 	// Accept connection from parent and connect to children if non-leaf node
// 	pu.Listen()

// 	s, _ := os.Create(fmt.Sprintf("log%d.txt", *peerId))
// 	defer s.Close()

// 	startTime := time.Now()
// 	recvBytes := 0
// 	count := 0

// 	resultCh := make(chan recvResult)

// 	time.Sleep(10 * time.Second)
// 	for i := 0; i < 1; i++ {
// 		go receiveWorker(pu, resultCh)
// 	}

// 	res := <-resultCh

// 	fmt.Println("🔥", recvBytes)

// 	resres := "hoho"

// 	sndBuf, err := json.Marshal(resres)
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = pu.SendAuditMsg(sndBuf)
// 	if err != nil {
// 		panic(err)
// 	}

// 	recvBytes += res.n
// 	count++

// 	elapsedTime := time.Since(startTime)
// 	throughput := (float64(recvBytes) * 8.0) / elapsedTime.Seconds() / (1000 * 1000)
// 	logStr := fmt.Sprintf("Seconds=%f, Throughput=%f, ReceivedSize=%d\n", elapsedTime.Seconds(), throughput, recvBytes)
// 	s.Write([]byte(logStr))
// 	fmt.Println("🔥🔥🔥", logStr)

// 	jsonData, _ := extractAndParseJSON(res.buf[:res.n])

// 	fmt.Println("🔥🔥🔥🔥🔥", jsonData)
// 	fmt.Printf("PEER-%d: FINISH!!! \n\n", *peerId)

// }

// // Progress Bar
// type PB struct {
// 	percent   int64
// 	current   int64
// 	total     int64
// 	rate      string
// 	character string
// }

// func (pb *PB) SetOption(start, total int64) {
// 	pb.current = start
// 	pb.total = total
// 	pb.character = "#"
// 	pb.percent = pb.getPercent()

// 	for i := 0; i < int(pb.percent); i += 2 {
// 		pb.rate += pb.character
// 	}
// }

// func (pb *PB) getPercent() int64 {
// 	return int64((float32(pb.current) / float32(pb.total)) * 100)
// }

// func (pb *PB) Show(current int64, fileName string, time float64, throughput float64, modeStatus string) {
// 	pb.current = current
// 	last := pb.percent
// 	pb.percent = pb.getPercent()

// 	// If there is a difference between the last percent and the current percent
// 	if pb.percent != last && pb.percent%4 == 0 {
// 		for i := 0; i < int(pb.percent-last); i += 2 {
// 			pb.rate += pb.character
// 		}
// 	}

// 	// Loading information
// 	fmt.Printf("\033[F\033[F\033[F[Status] %s \n", modeStatus)
// 	fmt.Printf("%s [%-25s] %3d%% %8d/%d  %.2fMbps in %.3fs \n", fileName, pb.rate, pb.percent, pb.current, pb.total, throughput, time)
// }

// func (pb *PB) Finish() {
// 	fmt.Println()
// }

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
)

const size = 32768

func main() {
	// var pb PB
	// var blockNumber uint32
	// var blockSize uint32

	configFile := flag.String("c", "push_child.toml", "Config file name")
	peerId := flag.Int("i", 1, "Peer ID")
	flag.Parse()

	fmt.Printf("PEER-%d: Config file name=%s \n\n\n", *peerId, *configFile)

	// Parse the config file
	var Cfg PuCtrl.Config
	if _, err := toml.DecodeFile(*configFile, &Cfg); err != nil {
		panic(err)
	}

	// Create Push/Pull Control
	pu := PuCtrl.CreatePuCtrl(*configFile, uint32(*peerId))

	// Accept connection from parent and connect to children if non-leaf node
	pu.Listen()

	// Log file
	s, _ := os.Create(fmt.Sprintf("log%d.txt", *peerId))
	defer s.Close()

	startTime := time.Now()
	recvBytes := 0
	throughput := 0.0
	count := 0
	prevBlockNumber := uint32(9999999)
	var fo *os.File

	for {
		// Read block data from PuCtrl
		buf := make([]byte, size)
		blockNumber, blockSize, n, err := pu.Receive(buf)
		if err != nil {
			panic(err)
		}

		if n <= 0 {
			break
		}

		if count == 0 {
			startTime = time.Now()
		}

		recvBytes += n
		count++

		// Calculate throughput
		elapsedTime := time.Since(startTime)
		throughput = (float64(recvBytes) * 8.0) / float64(elapsedTime.Seconds()) / (1000 * 1000)
		if count%1000 == 0 {
			log := fmt.Sprintf("Seconds=%f, Throughput=%f, ReceivedSize=%d\n", elapsedTime.Seconds(), throughput, recvBytes)
			s.Write([]byte(log))
		}

		// Create new block file
		if blockNumber != prevBlockNumber {
			filename := fmt.Sprintf("peer_%d_block_%d", *peerId, blockNumber)
			fo, err = os.Create(filename)
			if err != nil {
				panic(err)
			}
			prevBlockNumber = blockNumber
		}

		// Write block data into file
		_, err = fo.Write(buf[:n])
		if err != nil {
			fmt.Printf("PEER-%d: Write error\n", *peerId)
			panic(err)
		}

		if blockSize == uint32(recvBytes) {
			fmt.Printf("PEER-%d: FINISH!!! BlockNumber=%d \n\n", *peerId, blockNumber)
			fo.Close()
			recvBytes = 0
			blockSize = 0
			count = 0
		}

		// modeStatus = mp2.Mp2GetPeerStatus()
		// pb.Show(int64(total), filename, float64(elapsedTime.Seconds()), throughput, modeStatus)
	}

	// TODO
	// mp2.Mp2Close()

	fmt.Printf("PEER-%d: FINISH!!! \n\n", *peerId)

	time.Sleep(30 * time.Second)
}

// Progress Bar
type PB struct {
	percent   int64
	current   int64
	total     int64
	rate      string
	character string
}

func (pb *PB) SetOption(start, total int64) {
	pb.current = start
	pb.total = total
	pb.character = "#"
	pb.percent = pb.getPercent()

	for i := 0; i < int(pb.percent); i += 2 {
		pb.rate += pb.character
	}
}

func (pb *PB) getPercent() int64 {
	return int64((float32(pb.current) / float32(pb.total)) * 100)
}

func (pb *PB) Show(current int64, fileName string, time float64, throughput float64, modeStatus string) {
	pb.current = current
	last := pb.percent
	pb.percent = pb.getPercent()

	// If there is a difference between the last percent and the current percent
	if pb.percent != last && pb.percent%4 == 0 {
		for i := 0; i < int(pb.percent-last); i += 2 {
			pb.rate += pb.character
		}
	}

	// Loading information
	fmt.Printf("\033[F\033[F\033[F[Status] %s \n", modeStatus)
	fmt.Printf("%s [%-25s] %3d%% %8d/%d  %.2fMbps in %.3fs \n", fileName, pb.rate, pb.percent, pb.current, pb.total, throughput, time)
}

func (pb *PB) Finish() {
	fmt.Println()
}
