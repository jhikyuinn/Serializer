package weavehttp

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

	_ "net/http/pprof"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Channels struct {
	ID []string `json:"id"`
}

type binds []string

func (b binds) String() string {
	return strings.Join(b, ",")
}

func (b *binds) Set(v string) error {
	*b = strings.Split(v, ",")
	return nil
}

// Size is needed by the /demo/upload handler to determine the size of the uploaded file
type Size interface {
	Size() int64
}

// See https://en.wikipedia.org/wiki/Lehmer_random_number_generator
func generatePRData(l int) []byte {
	res := make([]byte, l)
	seed := uint64(1)
	for i := 0; i < l; i++ {
		seed = seed * 48271 % 2147483647
		res[i] = byte(seed)
	}
	return res
}

var certPath string

func init() {
	//docker
	// _, _, _, ok := runtime.Caller(0)
	//direct
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Failed to get current frame")
	}
	// direct
	certPath = path.Dir(filename)
}

// GetCertificatePaths returns the paths to certificate and key
func GetCertificatePaths() (string, string) {
	// docker
	// return path.Join("./src/cert.pem"), path.Join("./src/priv.key")
	// direct
	return path.Join(certPath, "./testdata/cert.pem"), path.Join(certPath, "./testdata/priv.key")

}

func setupHandler(www string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// fmt.Println(str.S)

		// Small 40x40 png
		w.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
			0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x28,
			0x01, 0x03, 0x00, 0x00, 0x00, 0xb6, 0x30, 0x2a, 0x2e, 0x00, 0x00, 0x00,
			0x03, 0x50, 0x4c, 0x54, 0x45, 0x5a, 0xc3, 0x5a, 0xad, 0x38, 0xaa, 0xdb,
			0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41, 0x54, 0x78, 0x01, 0x63, 0x18,
			0x61, 0x00, 0x00, 0x00, 0xf0, 0x00, 0x01, 0xe2, 0xb8, 0x75, 0x22, 0x00,
			0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
		})
	})

	mux.HandleFunc("/fabric/channels", func(w http.ResponseWriter, r *http.Request) {
		MongoURL := "mongodb://127.0.0.1:27017"
		client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(MongoURL))
		if err != nil {
			fmt.Println(err)
		}

		collection := client.Database("fabric").Collection("contribution")

		_, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		cursor, _ := collection.Find(context.TODO(), bson.M{})

		var channels Channels

		for cursor.Next(context.TODO()) {
			var elem bson.M
			err := cursor.Decode(&elem)
			if err != nil {
				fmt.Println(err)
			}
			channels.ID = append(channels.ID, elem["ChannelId"].(string))
		}

		jsonData, _ := json.Marshal(channels)
		w.Write(jsonData)
	})

	mux.HandleFunc("/weave/blocks", func(w http.ResponseWriter, r *http.Request) {
		// LOGIC: getting the number of fabric blocks
	})

	mux.HandleFunc("/weave/tx", func(w http.ResponseWriter, r *http.Request) {
		// LOGIC: getting the number of fabric transactions
	})

	mux.HandleFunc("/weave/peers", func(w http.ResponseWriter, r *http.Request) {
		// LOGIC: getting the number of joined peers and running peers
	})

	// LOGIC: getting peers' contribution
	// Peer ID, amount of chunks, latest latency, and average latency
	mux.HandleFunc("/weave/contribution", func(w http.ResponseWriter, r *http.Request) {
		MongoURL := "mongodb://127.0.0.1:27017"
		client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(MongoURL))
		if err != nil {
			fmt.Println(err)
		}

		collection := client.Database("fabric").Collection("contribution")

		_, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		// findOne := collection.FindOne(context.TODO(), bson.M{"ChannelId": "mychannel"})
		cursor, _ := collection.Find(context.TODO(), bson.M{})

		for cursor.Next(context.TODO()) {
			var elem bson.M
			err := cursor.Decode(&elem)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println(elem["ChannelId"].(string))

		}
	})

	return mux
}

// 8000이 열려있지 않다보니, 8080으로 접근시 Quic오류가 발생.
func Http3Listen() {
	// defer profile.Start().Stop()
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	// runtime.SetBlockProfileRate(1)

	bs := binds{}
	flag.Var(&bs, "bind", "bind to")
	www := flag.String("www", "", "www data")
	tcp := flag.Bool("tcp", false, "also listen on TCP")
	flag.Parse()

	if len(bs) == 0 {
		bs = binds{":8082"}
	}

	handler := setupHandler(*www)
	quicConf := &quic.Config{}

	bCap := bs[0]
	fmt.Println(bCap)
	var err error
	if *tcp {
		certFile, keyFile := GetCertificatePaths()
		err = http3.ListenAndServe(bCap, certFile, keyFile, handler)
	} else {
		server := http3.Server{
			Handler:    handler,
			Addr:       bCap,
			QuicConfig: quicConf,
		}
		err = server.ListenAndServeTLS(GetCertificatePaths())
	}
	if err != nil {
		fmt.Println(err)
	}
}
