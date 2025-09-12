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

const (
	address   = "203.252.112.31:4242" // 서버가 바인딩할 주소
	bufferLen = 1024                  // 수신 버퍼 크기
)

func main() {
	// TLS 구성을 설정 (간단한 인증을 위해 Self-Signed 인증서를 사용)
	tlsConfig := GenerateTLSConfig()

	// 0-RTT
	quicConf := &quic.Config{Allow0RTT: true}

	//conn net.PacketConn

	////listener, err := quic.ListenAddr(address, tlsConfig, quicConf)
	listener, err := quic.ListenAddrEarly(address, tlsConfig, quicConf)
	if err != nil {
		log.Fatalf("Failed to create QUIC listener: %v", err)
	}
	fmt.Printf("Server listening on %s\n", address)

	for {
		// 클라이언트의 세션 수락
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

	// 스트림 수신
	stream, err := session.AcceptStream(context.Background())
	if err != nil {
		log.Printf("Failed to accept stream: %v", err)
		return
	}

	used0RTT := session.ConnectionState().Used0RTT
	fmt.Printf("0-RTT: %t\n", used0RTT)

	// 데이터 읽기
	buffer := make([]byte, bufferLen)
	n, err := stream.Read(buffer)
	if err != nil {
		log.Printf("Failed to read from stream: %v", err)
		return
	}
	fmt.Printf("Received: %s\n", string(buffer[:n]))

	//time.Sleep(2 * time.Second)

	stream.Close()
}

// GenerateTLSConfig generates a TLS configuration with a self-signed certificate.
func GenerateTLSConfig() *tls.Config {
	// Generate an ECDSA private key
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	// Create a self-signed certificate
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year validity

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

	// Encode the certificate and private key to PEM format
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		log.Fatalf("Failed to marshal private key: %v", err)
	}
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyPEM})

	// Create a TLS certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEMBytes)
	if err != nil {
		log.Fatalf("Failed to create TLS certificate: %v", err)
	}

	return &tls.Config{
		Certificates:           []tls.Certificate{tlsCert},
		NextProtos:             []string{"quic-echo-example"}, // QUIC에 사용될 프로토콜 지정
		SessionTicketsDisabled: false,                         // 세션 티켓 활성화
	}
}

// tlsConfig := &tls.Config{
// 	InsecureSkipVerify: true,
// 	NextProtos:         []string{"quic-echo-example"},
// 	MaxVersion:         tls.VersionTLS13,
// 	MinVersion:         tls.VersionTLS10,
// 	ClientSessionCache: sessionCache, // 세션 캐시 설정
// }
