package __

import (
	pb "auditchain/msg"
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const addrDB = "mongodb://127.0.0.1:27017"

func BlockHashCalculator(s *pb.AuditMsg) []byte {
	var b bytes.Buffer
	gob.NewEncoder(&b).Encode(s)
	return b.Bytes()
}

func BlkInsert(cMsg *pb.AuditMsg) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(addrDB)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		panic(err)
	}

	collection := client.Database("ledger").Collection("blk")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := collection.InsertOne(ctx, bson.D{
		{"PrevHash", cMsg.PrevHash},
		{"CurHash", cMsg.CurHash},
		{"LeaderID", cMsg.LeaderID},
		{"MerkleRootHash", cMsg.MerkleRootHash},
		{"Signature", cMsg.Signature},
		{"NumofAbortTransaction", cMsg.Aborttransactionnum},
		{"Committee", cMsg.HonestAuditors},
	})
	if err != nil {
		fmt.Println(err)
		return
	} else {
		fmt.Println(res.InsertedID)
	}

	err = client.Disconnect(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
}
