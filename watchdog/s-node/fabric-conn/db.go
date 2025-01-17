package __

import (
	"context"
	"fmt"
	"time"
	mp "validator/mempool"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const addrDB = "mongodb://localhost:27017"

func Insert(data *mp.Fabric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(addrDB).SetAuth(options.Credential{
		Username: "inlab",
		Password: "1234",
	})

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		panic(err)
	}
	fmt.Println("Connected")

	collection := client.Database("ledger").Collection("tx")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := collection.InsertOne(ctx, bson.D{
		{"BlkNum", data.Num},
		{"PrevHash", data.PreviousHash},
		{"CurHash", data.DataHash},
		// {"Signature", data.DataHash},
		// {"Consensus", false},
	})
	fmt.Println(res.InsertedID)
}

func Read(num uint64) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(addrDB).SetAuth(options.Credential{
		Username: "inlab",
		Password: "1234",
	})

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		panic(err)
	}
	fmt.Println("Connected")

	collection := client.Database("ledger").Collection("tx")
	_, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findone_result := collection.FindOne(context.TODO(), bson.M{"BlkNum": num})
	var bson_obj bson.M
	if err2 := findone_result.Decode(&bson_obj); err2 != nil {
		return "none"
	}

	s := fmt.Sprint(bson_obj["CurHash"])
	return s
}
