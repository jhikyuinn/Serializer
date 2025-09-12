package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

const address = "localhost:4242"

func main() {
	// TLS 설정
	tlsConfig := generateTLSConfig()

	// QUIC 리스너 생성
	listener, err := quic.ListenAddr(address, tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	fmt.Printf("Listening on %s...\n", address)

	// 세션 대기
	session, err := listener.Accept(nil)
	if err != nil {
		log.Fatalf("Failed to accept session: %v", err)
	}

	// 스트림 대기
	stream, err := session.AcceptStream(nil)
	if err != nil {
		log.Fatalf("Failed to accept stream: %v", err)
	}
	defer stream.Close()

	// 데이터 수신
	file, err := os.Create("received_data.bin")
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	fmt.Println("Receiving data...")
	totalReceived := 0
	buf := make([]byte, 1024*1024) // 1MB 버퍼
	for {
		n, err := stream.Read(buf)
		totalReceived += n
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				log.Fatalf("Failed to write to file: %v", writeErr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Failed to read stream: %v", err)
		}
	}
	fmt.Printf("Data received: %d bytes\n", totalReceived)
}

// TLS 설정 생성
func generateTLSConfig() *tls.Config {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1년 유효기간

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		log.Fatalf("Failed to generate serial number: %v", err)
	}

	certTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Example QUIC Server"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		log.Fatalf("Failed to marshal private key: %v", err)
	}
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyPEM})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEMBytes)
	if err != nil {
		log.Fatalf("Failed to create TLS certificate: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
