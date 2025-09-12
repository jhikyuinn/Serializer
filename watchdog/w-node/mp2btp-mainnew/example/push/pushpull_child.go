package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
	"golang.org/x/exp/rand"
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

	rand.Seed(uint64(time.Now().UnixNano()))
	startTime := time.Now()
	recvBytes := 0
	randomMB := rand.Intn(49) + 1 // 1 ~ 49 MB 범위에서 선택
	stopBytes := randomMB * 1024 * 1024
	throughput := 0.0
	count := 0
	blockNumber := uint32(0)
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

		// Hard coding for testing
		if *peerId == Cfg.FAULT_PEER && recvBytes >= stopBytes {
			fmt.Printf("PEER-%d: Stop because recvBytes(%d) > stopBytes(%d)\n", *peerId, recvBytes, stopBytes)
			fo.Close()
			s.Close()
			os.Exit(0)
		}

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
			break
		}

		// modeStatus = mp2.Mp2GetPeerStatus()
		// pb.Show(int64(total), filename, float64(elapsedTime.Seconds()), throughput, modeStatus)
	}

	time.Sleep(30 * time.Second)
	pu.Close(blockNumber)

	fmt.Printf("PEER-%d: FINISH!!! \n\n", *peerId)
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
