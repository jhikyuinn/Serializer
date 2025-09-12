package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

const addr = "203.252.112.32:4242"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: receiver <output_file>")
		return
	}
	outputFileName := os.Args[1]

	// QUIC 세션 설정
	quicConfig := &quic.Config{
		EnableDatagrams: true,
	}

	// qlog 활성화
	os.Setenv("QLOGDIR", "./qlog")

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "handong.edu",
		NextProtos:         []string{"quic-file-transfer"},
		// MaxVersion:         tls.VersionTLS13,
		// MinVersion:         tls.VersionTLS10,
	}

	// 서버와 연결
	session, err := quic.DialAddr(context.Background(), addr, tlsConf, quicConfig)
	if err != nil {
		fmt.Println("Failed to dial:", err)
		return
	}

	stream, err := session.AcceptStream(context.Background())
	if err != nil {
		fmt.Println("Failed to accept stream:", err)
		return
	}
	defer stream.Close()

	// 파일 수신
	outputFile, err := os.Create(outputFileName)
	if err != nil {
		fmt.Println("Failed to create file:", err)
		return
	}
	defer outputFile.Close()

	start := time.Now()
	n, err := io.Copy(outputFile, stream)
	if err != nil {
		fmt.Println("Failed to receive file:", err)
		return
	}
	elapsed := time.Since(start)

	// 수신된 파일의 Throughput 계산
	throughput := float64(n) / elapsed.Seconds() / 1024 / 1024
	fmt.Printf("Received %d bytes in %s (%.2f MB/s)\n", n, elapsed, throughput)
}

// func generateTLSConfig() *tls.Config {
// 	key, err := rsa.GenerateKey(rand.Reader, 1024)
// 	if err != nil {
// 		panic(err)
// 	}
// 	template := x509.Certificate{SerialNumber: big.NewInt(1)}
// 	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
// 	if err != nil {
// 		panic(err)
// 	}
// 	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
// 	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

// 	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return &tls.Config{
// 		Certificates: []tls.Certificate{tlsCert},
// 		NextProtos:   []string{"mp2bs"},
// 		// MaxVersion:   tls.VersionTLS13,
// 		// MinVersion:   tls.VersionTLS13,
// 	}
// }
