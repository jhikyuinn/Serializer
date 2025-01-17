package __

import (
	"context"
	"fmt"
	"log"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Fabric struct {
	Num          int
	PreviousHash []byte
	DataHash     []byte
	Consensus    bool
}

var M = new(sync.Mutex)
var plist []int

func GetTXs(numTXs int64) []*Fabric {
	name := "watchdog"
	password := "1622"
	host := "141.223.65.114"
	port := "27017"
	connectionURI := "mongodb://" + name + ":" + password + "@" + host + ":" + port + "/"
	clientOptions := options.Client().ApplyURI(connectionURI)

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to MongoDB! ")
	collection := client.Database("test").Collection("hp2b")
	findOptions := options.Find()
	findOptions.SetLimit(numTXs)

	// Passing bson.D{{}} as the filter matches all documents in the collection
	cur, err := collection.Find(context.TODO(), bson.D{{"Consensus", false}}, findOptions)
	if err != nil {
		log.Fatal(err)
	}

	var results []*Fabric

	// Finding multiple documents returns a cursor
	// Iterating through the cursor allows us to decode documents one at a time
	for cur.Next(context.TODO()) {

		// create a value into which the single document can be decoded
		var elem Fabric
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}

		results = append(results, &elem)
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
	// Close the cursor once finished
	cur.Close(context.TODO())

	// Disconnect
	err = client.Disconnect(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connection to MongoDB closed.")

	return results
}

func UpdateConsensus(TXs [][]byte) {
	name := "watchdog"
	password := "1622"
	host := "141.223.65.114"
	port := "27017"
	connectionURI := "mongodb://" + name + ":" + password + "@" + host + ":" + port + "/"
	clientOptions := options.Client().ApplyURI(connectionURI)

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to MongoDB!")
	collection := client.Database("test").Collection("hp2b")

	// Update
	update := bson.D{{"$set", bson.D{{"Consensus", true}}}}
	for _, each := range TXs {
		updateFilter := bson.D{{"DataHash", each}}

		updateResult, err := collection.UpdateOne(context.TODO(), updateFilter, update)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Matched %v documents and updated %v documents.\n", updateResult.MatchedCount, updateResult.ModifiedCount)
	}

	// Disconnect
	err = client.Disconnect(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connection to MongoDB closed.")
}
