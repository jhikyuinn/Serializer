package weavehttp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"
	"os"

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

var certPath string

func init() {
    _, filename, _, ok := runtime.Caller(0)
    if !ok {
        panic("Failed to get current frame")
    }
    certPath = path.Dir(filename)
}

func GetCertificatePaths() (string, string) {
    if IsDocker() {
        return path.Join("./src/cert.pem"), path.Join("./src/priv.key")
    }
    return path.Join(certPath, "./testdata/cert.pem"), path.Join(certPath, "./testdata/priv.key")
}

func IsDocker() bool {
    if _, err := os.Stat("/.dockerenv"); err == nil {
        return true
    }
    return false
}

func setupHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

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

	// Current not used.
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

func Http3Listen(bindAddr string, enableTCP bool) {
	go func() {
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			log.Printf("pprof server error: %v", err)
		}
	}()

	if bindAddr == "" {
		bindAddr = ":8082"
	}

	handler := setupHandler()
	quicConf := &quic.Config{}

	certFile, keyFile := GetCertificatePaths()

	var err error
	if enableTCP {
		log.Printf("Starting HTTP/3 (TCP) server on %s", bindAddr)
		err = http3.ListenAndServe(bindAddr, certFile, keyFile, handler)
	} else {
		log.Printf("Starting HTTP/3 (QUIC) server on %s", bindAddr)
		server := http3.Server{
			Handler:    handler,
			Addr:       bindAddr,
			QuicConfig: quicConf,
		}
		err = server.ListenAndServeTLS(certFile, keyFile)
	}

	if err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
