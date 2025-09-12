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

const addrDB = "mongodb://localhost:27017"

// func BlkInsert(cMsg *pb.AuditMsg) {
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	clientOptions := options.Client().ApplyURI(addrDB)

// 	client, err := mongo.Connect(ctx, clientOptions)
// 	if err != nil {
// 		panic(err)
// 	}

// 	collection := client.Database("ledger").Collection("blk")
// 	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()
// 	res, err := collection.InsertOne(ctx, bson.D{
// 		{"BlkNum", cMsg.BlkNum},
// 		{"PrevHash", cMsg.PrevHash},
// 		{"CurHash", cMsg.CurHash},
// 		{"LeaderID", cMsg.LeaderID},
// 		{"MerkleRootHash", cMsg.MerkleRootHash},
// 		{"Signature", cMsg.Signature},
// 		{"HonestAuditors", cMsg.HonestAuditors},
// 		{"AuditMsg_PREPARE", cMsg.PhaseNum},
// 		{"TxList", cMsg.TxList},
// 		{"Committee", cMsg.HonestAuditors},
// 	})
// 	if err != nil {
// 		fmt.Println(err)
// 		return
// 	} else {
// 		fmt.Println(res.InsertedID)
// 	}

// 	// Disconnected
// 	err = client.Disconnect(context.TODO())
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// }

// func BlkHashRead(num uint32) string {
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	clientOptions := options.Client().ApplyURI(addrDB).SetAuth(options.Credential{
// 		Username: "inlab",
// 		Password: "1234",
// 	})

// 	client, err := mongo.Connect(ctx, clientOptions)
// 	if err != nil {
// 		panic(err)
// 	}
// 	collection := client.Database("ledger").Collection("blk")
// 	_, cancel = context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()

// 	findone_result := collection.FindOne(context.TODO(), bson.M{"BlkNum": num})
// 	var bson_obj bson.M
// 	if err2 := findone_result.Decode(&bson_obj); err2 != nil {
// 		return "none"
// 	}
// 	return bson_obj["CurHash"].(string)
// }

func BlockHashCalculator(s *pb.AuditMsg) []byte {
	var b bytes.Buffer
	gob.NewEncoder(&b).Encode(s)
	return b.Bytes()
}

func UserAbortInfoInsert(cMsg *pb.AuditMsg) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(addrDB)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		panic(err)
	}

	collection := client.Database("User").Collection("abortTransaction")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := collection.InsertOne(ctx, bson.D{
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

	// Disconnected
	err = client.Disconnect(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
}
