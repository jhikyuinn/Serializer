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
	maxBlockNumber := flag.Int("n", 1, "Maximum Block number (> 0))")
	configFile := flag.String("c", "pull_receiver0.toml", "Config file name")
	peerId := flag.Int("i", 0, "Peer ID")
	flag.Parse()

	fmt.Printf("PEER-%d: Config file name=%s \n\n\n", *peerId, *configFile)

	// Parse the config file
	var config PuCtrl.Config
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		panic(err)
	}

	// Create Push/Pull Control
	pu := PuCtrl.CreatePuCtrl(*configFile, uint32(*peerId))

	// Covert PeerAddr to NodeInfo
	nodeInfo := pu.GetNodeInfo(config.PEER_ADDRS)

	// Accept connection from parent and connect to children if non-leaf node
	pu.Listen()

	// Connect to senders
	pu.Connect(nodeInfo, byte(PuCtrl.PULL_SESSION))

	// Log file
	s, _ := os.Create(fmt.Sprintf("log%d.txt", *peerId))
	defer s.Close()

	startTime := time.Now()
	recvBytes := 0
	throughput := 0.0
	count := 0
	prevBlockNumber := uint32(9999999)
	var fo *os.File

	for blockNumber := 0; blockNumber < *maxBlockNumber; blockNumber++ {

		// Find block
		pu.FindBlock(uint32(blockNumber))

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
				count++
			}

			recvBytes += n

			// Calculate throughput
			elapsedTime := time.Since(startTime)
			throughput = (float64(recvBytes) * 8.0) / float64(elapsedTime.Seconds()) / (1000 * 1000)
			if recvBytes >= count*1024*1024 || recvBytes == int(blockSize) {
				log := fmt.Sprintf("Seconds=%f, Throughput=%f, ReceivedSize=%d\n", elapsedTime.Seconds(), throughput, recvBytes)
				s.Write([]byte(log))
				count++
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
				fo.Close()
				break
			}

			// modeStatus = mp2.Mp2GetPeerStatus()
			// pb.Show(int64(total), filename, float64(elapsedTime.Seconds()), throughput, modeStatus)
		}
	}

	fmt.Printf("PEER-%d: FINISH!!! \n\n", *peerId)

	//pb.Finish()
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
