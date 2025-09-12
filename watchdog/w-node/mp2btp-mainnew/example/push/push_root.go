package main

import (
	"flag"
	"fmt"

	"github.com/BurntSushi/toml"
	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
)

const blockNumber = uint32(10)

func main() {
	fileName := flag.String("b", "../block", "File name you want to send")
	maxBlockNumber := flag.Int("n", 1, "Maximum Block number (> 0))")
	configFile := flag.String("c", "push_root.toml", "Config file name")
	//topAddr := flag.String("t", "127.0.0.1:5000", "Top node")
	peerId := flag.Int("i", 0, "Peer ID")
	//testFlag := flag.Bool("t", false, "TEST mode")
	// rate := flag.Float64("r", 100, "Throughput(unit: mbit)")
	// time := flag.Float64("e", 10, "Running time(unit: sec)")
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

	// Connect to child
	pu.Connect(nodeInfo, byte(PuCtrl.PUSH_SESSION))

	// Send block
	for blockNumber := 0; blockNumber < *maxBlockNumber; blockNumber++ {
		fmt.Printf("PEER-%d: File name=%s_%d \n", *peerId, *fileName, blockNumber)
		fmt.Printf("PEER-%d: Send a block(number=%d) \n", *peerId, blockNumber)
		pu.Send(*fileName, uint32(blockNumber))
	}

	pu.Close(uint32(*maxBlockNumber - 1))

	// if !*testFlag {
	// 	// Send block
	// 	// We assume that the requested block is "block" in this test application
	// 	fmt.Printf("PEER-%d: File name=%s_%d \n", *peerId, *fileName, *blockNumber)
	// 	fmt.Printf("PEER-%d: Send a block(number=%d) \n", *peerId, *blockNumber)
	// 	mp2.Mp2SendBlock(*fileName, uint32(*blockNumber))
	// } else {
	// 	// TP TEST (default rate: 100mbit, time: 10sec)
	// 	// We calculate the size of the block so that it can run for as long as the entered time
	// 	fmt.Printf("PEER-%d: Throughput test\n", *peerId)
	// 	// mp2.Mp2Test(*rate, *time) // Mp2Test(unit: mbit, unit: sec)
	// }

	// mp2.Mp2Close()

	fmt.Printf("PEER-%d: FINISH!!! \n\n", *peerId)
}
