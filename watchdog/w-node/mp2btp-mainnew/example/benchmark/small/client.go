package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"
	"github.com/quic-go/quic-go/qlog"
)

type bufferedWriterCloser struct {
	*bufio.Writer
	io.Closer
}

const size = 4 * 1024 // 4KB

func main() {

	myAddr := flag.String("m", "127.0.0.1:4000", "My address")
	childAddr := flag.String("c", "127.0.0.1:5000", "Child address")
	t := flag.Float64("t", 10.0, "Total time")
	logFlag := flag.Bool("l", false, "Quic log output")
	flag.Parse()

	quicConf := &quic.Config{}
	if *logFlag {
		quicConf.Tracer = qlog.NewTracer(func(p logging.Perspective, connectionID []byte) io.WriteCloser {
			filename := fmt.Sprintf("sender_%x.qlog", connectionID)
			f, err := os.Create(filename)
			if err != nil {
				panic(err)
			}
			return &bufferedWriterCloser{
				Writer: bufio.NewWriter(f),
				Closer: f,
			}
		})
	}

	// TLS configuration
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"mp2bs"},
		MaxVersion:         tls.VersionTLS13,
		MinVersion:         tls.VersionTLS10,
	}

	// Bind remote IP
	rAddr, _ := net.ResolveUDPAddr("udp", *childAddr)

	// Bind my IP
	lAddr, _ := net.ResolveUDPAddr("udp", *myAddr)

	// Create UDP connection
	udpConn, _ := net.ListenUDP("udp", lAddr)

	// Connect to Master listener
	quicSession, err := quic.Dial(udpConn, rAddr, *childAddr, tlsConf, nil)
	if err != nil {
		panic(err)
	}

	startTime := time.Now()

	quicStream, err := quicSession.OpenStreamSync(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("Connection complete(%f)\n", time.Since(startTime).Seconds())

	buf := make([]byte, size)
	if _, err = rand.Read(buf); err != nil {
		panic(err)
	}

	// 주기적으로 데이터를 전송하기 위한 ticker 설정
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	total := 0

	for {
		select {
		case <-ticker.C:
			// 4KB의 데이터를 스트림에 전송
			n, err := quicStream.Write(buf)
			if err != nil {
				fmt.Println("Failed to write data:", err)
				quicStream.Close()
				quicSession.CloseWithError(0, "CLOSE")
				return
			}

			total += n

			// 종료 조건 확인
			elapsedTime := time.Since(startTime)
			if elapsedTime.Seconds() >= *t {
				quicStream.Close()
				quicSession.CloseWithError(0, "CLOSE")
				fmt.Printf("Complete!! (%d bytes)\n", total)
				return
			}
		}
	}
}
