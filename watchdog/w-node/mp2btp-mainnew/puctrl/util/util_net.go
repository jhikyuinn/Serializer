package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"strings"
)

// Generate TLS Configuration
func GenerateTLSConfig(hostID uint32) *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}

	host := strconv.FormatUint(uint64(hostID), 10)

	return &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"puctrl"},
		ServerName:         host,
		MaxVersion:         tls.VersionTLS13,
		MinVersion:         tls.VersionTLS10,
	}
}

func Ip2int(addr string) uint32 {
	ip := net.ParseIP(addr)

	if ip == nil {
		Log("Mp2SessionManager.Ip2int(): Wrong IP address format")

		return 0
	}
	ip = ip.To4()

	return binary.BigEndian.Uint32(ip)
}

func Int2ip(ipInt uint32) string {
	ipByte := make([]byte, 4)

	binary.BigEndian.PutUint32(ipByte, ipInt)

	ip := net.IP(ipByte)

	return ip.String()
}

func Int2IpArray(ipArray []uint32) []string {
	result := make([]string, len(ipArray))
	for i, ip := range ipArray {
		ipBytes := []byte{
			byte(ip >> 24),
			byte(ip >> 16),
			byte(ip >> 8),
			byte(ip),
		}
		result[i] = net.IP(ipBytes).String()
	}
	return result
}

func GetIP(addr string) string {
	parts := strings.Split(addr, ":")
	if len(parts) >= 2 {
		lastColon := strings.LastIndex(addr, ":")
		return addr[:lastColon]
	}
	return addr // return all if no colon found (only IP part is present)
}
