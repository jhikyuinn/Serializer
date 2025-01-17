package contribution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	types "github.com/Watchdog-Network/types"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/lovoo/goka"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type contributionCodec struct{}

type FabricChannelState struct {
	FabricChannels []FabricChannel
}

type FabricChannel struct {
	Type           int               `bson:"type" json:"type"`
	ChannelId      string            `bson:"channel" json:"channel"`
	BlockNumber    int               `bson:"block" json:"block"`
	Transactions   int               `bson:"tx" json:"tx"`
	Members        int               `bson:"members" json:"members"`
	FabricPeers    []PeerAttribution `bson:"attr" json:"attr"`
	ChannelTrigger chan string
}

type PeerAttribution struct {
	Id   string
	Sent int
}

var (
	tmc *goka.TopicManagerConfig

	mutex sync.Mutex
)

func init() {
	tmc = goka.NewTopicManagerConfig()
	tmc.Table.Replication = 3
	tmc.Stream.Replication = 3
}

// Encodes types.StateData into []byte
func (cc *contributionCodec) Encode(value interface{}) ([]byte, error) {
	if _, isState := value.(*FabricChannel); !isState {
		return nil, fmt.Errorf("codec requires value FabricChannel, got %T", value)
	}
	return json.Marshal(value)
}

// Decodes a types.StateData from []byte to it's go representation
func (cc *contributionCodec) Decode(data []byte) (interface{}, error) {
	var (
		c   FabricChannel
		err error
	)
	err = json.Unmarshal(data, &c)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling FabricChannel: %v", err)
	}
	return &c, nil
}

func (fc *FabricChannel) getContribution() FabricChannel {
	return *fc
}

func (fc *FabricChannel) Start(brokers string) {
	go fc.consumer(brokers)
}

func (fc *FabricChannel) Record(ctx goka.Context, msg interface{}) {

	mutex.Lock()
	defer mutex.Unlock()

	MongoURL := "mongodb://localhost:27017"
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(MongoURL))
	if err != nil {
		fmt.Println(err)
	}
	defer client.Disconnect(context.TODO())

	collection := client.Database("fabric").Collection("contribution")

	var recvInfo FabricChannel

	// if the channel is already exist, find that the peer was joined
	isChannelExist, r := fc.IsChannelExist(context.TODO(), msg.(string), collection)
	fmt.Println(msg.(string))
	if isChannelExist {
		// appending peer contribution
		if r["FabricPeers"] != nil {
			for _, j := range r["FabricPeers"].(primitive.A) {
				if strings.Contains(msg.(*types.SymbolBundleData).Id, j.(primitive.M)["id"].(string)) {
					s := int(j.(primitive.M)["sent"].(int32)) + len(msg.(*types.SymbolBundleData).Symbols)

					newPeer := PeerAttribution{
						Id:   msg.(*types.SymbolBundleData).Id,
						Sent: s,
					}
					recvInfo = FabricChannel{
						Type:        4,
						ChannelId:   msg.(*types.SymbolBundleData).Channel,
						FabricPeers: []PeerAttribution{newPeer},
					}

					fc.updateChannel(context.TODO(), recvInfo, collection)
					return
				}
			}
		}

		// if the peer is just joined, push the peer information
		newPeer := PeerAttribution{
			// Id:   msg.(*types.SymbolBundleData).Id,
			// Sent: len(msg.(*types.SymbolBundleData).Symbols),
		}
		recvInfo = FabricChannel{
			Type: 3,
			// ChannelId:   msg.(*types.SymbolBundleData).Channel,
			FabricPeers: []PeerAttribution{newPeer},
		}
		fc.updateChannel(context.TODO(), recvInfo, collection)
		return
	} else {
		// if the channel is not exist, incert the peer information
		newPeer := PeerAttribution{
			// Id:   msg.(*types.SymbolBundleData).Id,
			Sent: 1,
		}
		recvInfo = FabricChannel{
			Type: 3,
			// ChannelId:   msg.(*types.SymbolBundleData).Channel,
			FabricPeers: []PeerAttribution{newPeer},
		}

		fc.insertChannel(context.TODO(), recvInfo, collection)
	}
}

func (fc *FabricChannel) consumer(brokers string) {
	probrokers := brokers
	contConsumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": probrokers,
		"group.id":          "contributionGroup",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	contConsumer.SubscribeTopics([]string{"contribution-topic"}, nil)
	defer contConsumer.Close()

	for {
		msg, err := contConsumer.ReadMessage(-1)
		if err == nil {
			recvInfo := FabricChannel{}
			json.Unmarshal(msg.Value, &recvInfo)
			recvInfo.FabricPeers = []PeerAttribution{}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			MongoURL := "mongodb://localhost:27017"
			client, err := mongo.Connect(ctx, options.Client().ApplyURI(MongoURL))
			if err != nil {
				fmt.Println(err)
			}

			collection := client.Database("fabric").Collection("contribution")
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			isChannelExist, _ := fc.IsChannelExist(ctx, recvInfo.ChannelId, collection)
			if isChannelExist {
				fc.updateChannel(ctx, recvInfo, collection)
			} else {
				go func() {
					fmt.Println("New joined channel:", recvInfo.ChannelId)
					go fc.setChannelId(recvInfo.ChannelId)
				}()
				fc.insertChannel(ctx, recvInfo, collection)
				continue
			}
		}
	}
}

func (fc *FabricChannel) setChannelId(channelId string) {
	fc.ChannelTrigger <- channelId
}

func (fc *FabricChannel) IsChannelExist(ctx context.Context, channelId string, collection *mongo.Collection) (bool, bson.M) {
	opts := options.FindOne().SetSort(bson.D{{Key: "ChannelId", Value: 1}})
	var r bson.M
	err := collection.FindOne(context.TODO(), bson.D{{Key: "ChannelId", Value: channelId}}, opts).Decode(&r)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		} else {
			fmt.Println(err)
		}
		return false, nil
	}

	return true, r
}

func (fc *FabricChannel) insertChannel(ctx context.Context, recvInfo FabricChannel, collection *mongo.Collection) {
	res, err := collection.InsertOne(ctx, bson.D{
		{Key: "ChannelId", Value: recvInfo.ChannelId},
		{Key: "BlockNumber", Value: recvInfo.BlockNumber},
		{Key: "Transactions", Value: recvInfo.Transactions},
		{Key: "Members", Value: recvInfo.Members},
		{Key: "FabricPeers", Value: recvInfo.FabricPeers},
	})
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Successfully inserted to private DB", res.InsertedID)

}

func (fc *FabricChannel) updateChannel(ctx context.Context, recvInfo FabricChannel, collection *mongo.Collection) {
	f := collection.FindOne(context.TODO(), bson.M{"ChannelId": recvInfo.ChannelId})
	var bsonObj bson.M
	if err := f.Decode(&bsonObj); err != nil {
		fmt.Println(err)
		return
	}

	var update bson.M
	filter := bson.M{"ChannelId": recvInfo.ChannelId}
	if recvInfo.Type == 1 {
		update = bson.M{
			"$set": bson.M{
				"BlockNumber":  recvInfo.BlockNumber,
				"Transactions": int(bsonObj["Transactions"].(int32)) + recvInfo.Transactions,
			},
		}
	} else if recvInfo.Type == 2 {
		update = bson.M{
			"$set": bson.M{
				"Members": recvInfo.Members,
			},
		}
	} else if recvInfo.Type == 3 {
		update = bson.M{
			"$push": bson.M{
				"FabricPeers": recvInfo.FabricPeers[0],
			},
		}
	} else if recvInfo.Type == 4 {
		filter = bson.M{
			"ChannelId":      recvInfo.ChannelId,
			"FabricPeers.id": recvInfo.FabricPeers[0].Id,
		}
		update = bson.M{
			"$set": bson.M{
				"FabricPeers.$.sent": recvInfo.FabricPeers[0].Sent,
			},
		}
	}

	collection.UpdateOne(ctx, filter, update)
}
