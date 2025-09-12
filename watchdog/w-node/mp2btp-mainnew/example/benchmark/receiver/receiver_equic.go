package main

import (
	"context"
	"fmt"
	"os"
	"time"

	equic "github.com/MCNL-HGU/Enhanced-quic"
)

const size = 65536

func main() {

	eConf := &equic.Config{
		VERBOSE_MODE: false,
		NIC_NAMES:    []string{"eth", "eth"},
		NIC_ADDRS:    []string{"127.0.0.1:7000", "127.0.0.1:8000"},
		// NIC_NAMES: []string{"eth"},
		// NIC_ADDRS: []string{"192.168.10.3:5000"},
	}

	listener, err := equic.ListenAddr(eConf)
	if err != nil {
		panic(err)
	}

	session := listener.Accept(context.Background())

	stream := session.AcceptStream(context.Background())

	s, _ := os.Create("throughput_equic")
	defer s.Close()

	total := 0
	count := 0
	throughput := 0.0

	recvBuf := make([]byte, size)

	startTime := time.Now()

	for {
		n, err := stream.Read(recvBuf)
		if err != nil {
			break
		}

		total += n
		count++

		elapsedTime := time.Since(startTime)
		throughput = (float64(total) * 8.0) / float64(elapsedTime.Seconds()) / (1000 * 1000)

		if count%1000 == 0 {
			fmt.Printf("Seconds=%f, Throughput=%f\n", elapsedTime.Seconds(), throughput)
			log := fmt.Sprintf("Seconds=%f, Throughput=%f\n", elapsedTime.Seconds(), throughput)
			s.Write([]byte(log))
		}
	}

	fmt.Printf("Complete!! (%d bytes)\n", total)
}
