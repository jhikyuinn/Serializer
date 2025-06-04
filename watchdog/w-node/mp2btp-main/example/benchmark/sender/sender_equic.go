package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"time"
	// equic "github.com/MCNL-HGU/Enhanced-quic"
)

const size = 65536

func main() {

	var err error

	childAddr := flag.String("c", "127.0.0.1:7000", "Child address")
	t := flag.Float64("t", 10.0, "Total time")
	flag.Parse()

	conf := &equic.Config{
		VERBOSE_MODE: false,
		NIC_NAMES:    []string{"eth", "eth"},
		NIC_ADDRS:    []string{"127.0.0.1:5000", "127.0.0.1:6000"},
		// NIC_NAMES: []string{"eth"},
		// NIC_ADDRS: []string{"192.168.10.3:5000"},
	}

	// Connect to Master listener
	session, err := equic.Dial(context.Background(), *childAddr, conf)
	if err != nil {
		panic(err)
	}

	startTime := time.Now()

	stream := session.OpenStreamSync(context.Background())

	fmt.Printf("Connection complete(%f)\n", time.Since(startTime).Seconds())

	buf := make([]byte, size)
	total := 0
	n := 0

	if n, err = rand.Read(buf); err != nil {
		panic(err)
	}

	for {
		total = total + n

		stream.Write(buf[:n])

		elapsedTime := time.Since(startTime)

		if elapsedTime.Seconds() >= *t {
			break
		}
	}

	fmt.Printf("Complete!! (%d bytes)\n", total)
}
