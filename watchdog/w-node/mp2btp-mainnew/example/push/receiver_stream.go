package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"time"

	quic "github.com/quic-go/quic-go"
)

const address = "0.0.0.0:4242" // 서버 주소

func main() {
	// TLS 설정 생성
	tlsConfig := generateTLSConfig()

	// QUIC 리스너 생성
	listener, err := quic.ListenAddr(address, tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to create QUIC listener: %v", err)
	}
	fmt.Printf("Server listening on %s\n", address)

	for {
		// 클라이언트 세션 수락
		session, err := listener.Accept(context.Background())
		if err != nil {
			log.Printf("Failed to accept session: %v", err)
			continue
		}
		fmt.Println("New session accepted")

		go handleSession(session)
	}
}

func handleSession(session quic.Connection) {
	defer session.CloseWithError(0, "Session closed")

	for {
		// 스트림 수신
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			log.Printf("Failed to accept stream: %v", err)
			break
		}

		// 데이터 읽기
		buffer := make([]byte, 1024)
		n, err := stream.Read(buffer)
		if err != nil {
			log.Printf("Failed to read from stream: %v", err)
			stream.Close()
			continue
		}
		fmt.Printf("Received: %s\n", string(buffer[:n]))

		// 스트림 닫기
		stream.Close()
	}
}

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
