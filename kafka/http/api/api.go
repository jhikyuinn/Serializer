package api

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"weave-kafka/http/testdata"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type AuthSuccess struct {
}

var address string

func GetBlocks() {
	insecure := flag.Bool("insecure", false, "skip certificate verification")

	flag.Parse()
	// urls := flag.Args()

	pool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatal(err)
	}
	testdata.AddRootCA(pool)

	var qconf quic.Config

	roundTripper := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{
			RootCAs:            pool,
			InsecureSkipVerify: *insecure,
			// KeyLogWriter:       keyLog,
		},
		QuicConfig: &qconf,
	}
	defer roundTripper.Close()
	hclient := &http.Client{
		Transport: roundTripper,
	}

	jsonBody := []byte(`{"name":"tester"}`)
	bodyReader := bytes.NewReader(jsonBody)

	req, _ := http.NewRequest("POST", "https:/localhost:8000/test", bodyReader)
	req.Header.Add("Content-Type", "application/json")

	resp, err := hclient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err == nil {
		str := string(respBody)
		fmt.Println(str)
	}
}
