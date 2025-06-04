package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
	PuCtrl "github.com/MCNL-HGU/mp2btp/puctrl"
)

const blockNumber = uint32(10)

func extractAndParseJSON(data []byte) (string, error) {
	// JSON 시작 위치 탐색: 일반적으로 문자열은 '"' (34), JSON 객체는 '{' (123)
	jsonStart := bytes.IndexAny(data, "\"{")
	if jsonStart == -1 {
		return "", fmt.Errorf("no JSON start found")
	}

	// JSON 데이터 추출
	jsonBytes := data[jsonStart:]

	// Unmarshal 시도
	var name string
	err := json.Unmarshal(jsonBytes, &name)
	if err != nil {
		return "", fmt.Errorf("json unmarshal error: %w", err)
	}

	return name, nil
}

func main() {
	configFile := flag.String("c", "push_root.toml", "Config file name")
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
	fmt.Println("💛", config.PEER_ADDRS)
	nodeInfo := pu.GetNodeInfo(config.PEER_ADDRS)

	// Accept connection from parent and connect to children if non-leaf node
	pu.Listen()

	time.Sleep(1 * time.Second)

	// Connect to child
	fmt.Println("💛", nodeInfo)
	pu.Connect(nodeInfo, byte(PuCtrl.PUSH_SESSION))

	time.Sleep(2 * time.Second)

	my := "hello"
	b, _ := json.Marshal(my)
	fmt.Println(b)

	pu.SendAuditMsg(b)

	fmt.Println("💛FINISH")

	time.Sleep(2 * time.Second)
	rcvBuf := make([]byte, 8192)
	n, err := pu.ReceiveRaw(rcvBuf)
	if err != nil {
		panic(err)
	}

	jsonData, _ := extractAndParseJSON(rcvBuf[0:n])

	fmt.Println("🔥", jsonData)

	fmt.Printf("PEER-%d: FINISH!!! \n\n", *peerId)
}
