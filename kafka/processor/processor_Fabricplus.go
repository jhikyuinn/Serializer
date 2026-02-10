package main

// import (
// 	"context"
// 	"encoding/json"
// 	"flag"
// 	"fmt"
// 	"log"
// 	"regexp"
// 	"strconv"
// 	"time"

// 	"weave-kafka/contribution"
// 	weavehttp "weave-kafka/http"

// 	// contribution "weave-kafka/contribution"

// 	types "github.com/Watchdog-Network/types"
// 	"github.com/hyperledger/fabric-protos-go/common"
// 	"github.com/hyperledger/fabric/protoutil"

// 	// "google.golang.org/protobuf/proto"
// 	"github.com/golang/protobuf/proto"

// 	"github.com/lovoo/goka"
// 	"github.com/lovoo/goka/codec"
// )

// type Blocks struct {
// 	block            []Block
// 	PeerContribution contribution.FabricChannel
// }

// type Block struct {
// 	ChannelID    string
// 	OutputStream goka.Stream

// 	Peers            []PeerInfo
// 	PeerContribution contribution.FabricChannel
// }

// type PeerInfo struct {
// 	ID   string
// 	Sent int
// }

// var (
// 	brokers1 string
// 	brokers2 []string

// 	topicabort  goka.Stream = "mychannel-abort"
// 	topiccommit goka.Stream = "mychannel-commit"
// 	group       goka.Group  = "mychannel-group"

// 	emitted = false

// 	tmc *goka.TopicManagerConfig
// )
// var (
// 	epsilonUrgent = 1.0
// 	epsilonLow    = 1.0
// )

// type Graph struct {
// 	edges map[int][]int
// 	users map[int]string
// }

// func NewGraph(size int) *Graph {
// 	return &Graph{edges: make(map[int][]int)}
// }

// // This codec allows marshalling (encode) and unmarshalling (decode) the block struct(or produce struct) to and from the group table
// type blockCodec struct{}

// func init() {
// 	tmc = goka.NewTopicManagerConfig()
// 	tmc.Table.Replication = 3
// 	tmc.Stream.Replication = 3
// }

// // Encodes types.StateData into []byte
// func (bc *blockCodec) Encode(value interface{}) ([]byte, error) {
// 	if _, isState := value.(*types.StateData); !isState {
// 		return nil, fmt.Errorf("codec requires value *types.StateData, got %T", value)
// 	}
// 	return json.Marshal(value)
// }

// // Decodes a types.StateData from []byte to it's go representation
// func (bc *blockCodec) Decode(data []byte) (interface{}, error) {
// 	var (
// 		c   types.StateData
// 		err error
// 	)
// 	err = json.Unmarshal(data, &c)
// 	if err != nil {
// 		return nil, fmt.Errorf("error unmarshaling types.StateData: %v", err)
// 	}
// 	return &c, nil
// }

// func (bs *Blocks) process(ctx goka.Context, msg interface{}) {

// 	// bs.PeerContribution.Record(ctx, msg)
// 	key := ctx.Offset()
// 	ctx.Loopback(strconv.Itoa(int(key)), msg)

// }

// func (bs *Blocks) loopProcess(ctx goka.Context, msg interface{}) {
// 	startelsaped := time.Now().UnixMilli()

// 	var deserializedBatch [][]byte
// 	err := json.Unmarshal(msg.([]byte), &deserializedBatch)
// 	if err != nil {
// 		log.Fatalf("Failed to unmarshal JSON: %v", err)
// 	}

// 	// protobuf 메시지로 변환
// 	var kafkadata []*common.Envelope
// 	for _, serializedEnv := range deserializedBatch {
// 		env := &common.Envelope{}
// 		// fmt.Println("❌", serializedEnv)
// 		err := proto.Unmarshal(serializedEnv, env)
// 		if err != nil {
// 			log.Fatalf("Failed to unmarshal envelope: %v", err)
// 		}
// 		kafkadata = append(kafkadata, env)
// 	}

// 	fmt.Println("[The length of Kafka incoming data]", len(kafkadata))
// 	if len(kafkadata) == 1 {
// 		hello, _ := json.Marshal(kafkadata)
// 		ctx.Emit(topiccommit, strconv.Itoa(int(ctx.Offset())), hello)
// 		return
// 	}
// 	fmt.Println(kafkadata)

// 	var Ordereddata, Abortdata []*common.Envelope

// 	Ordereddata, Abortdata = epsilonOrdering(kafkadata)

// 	marshalledOrdereddata, _ := json.Marshal(Ordereddata)
// 	marshalledAbortdata, _ := json.Marshal(Abortdata)

// 	ctx.Emit(topiccommit, strconv.Itoa(int(ctx.Offset())), marshalledOrdereddata)
// 	ctx.Emit(topicabort, strconv.Itoa(int(ctx.Offset())), marshalledAbortdata)

// 	fmt.Println("[Total Time]", time.Now().UnixMilli()-startelsaped)

// }

// func epsilonOrdering(msg []*common.Envelope) (tSerial []*common.Envelope, tAbort []*common.Envelope) {

// 	var NumactualorderTx = 4
// 	// 미리 만들어놓은 유저의 크기에 맞게.
// 	var Numdataitem = 10

// 	Conflictgraph := make([][]int, NumactualorderTx)
// 	Readset := make([][]int, NumactualorderTx)
// 	Writeset := make([][]int, NumactualorderTx)
// 	Userset := make([]string, NumactualorderTx)

// 	for i := 0; i < NumactualorderTx; i++ {
// 		Conflictgraph[i] = make([]int, NumactualorderTx)
// 		Readset[i] = make([]int, Numdataitem)
// 		Writeset[i] = make([]int, Numdataitem)
// 	}

// 	re := regexp.MustCompile(`User(\d+)`)
// 	txs := regexp.MustCompile(`Query`)

// 	for i := 0; i < NumactualorderTx; i++ {
// 		payload, _ := protoutil.UnmarshalTransaction(msg[i].Payload)
// 		matches := re.FindAllStringSubmatch(payload.String(), -1)

// 		seen := make(map[string]bool, len(matches))
// 		for _, match := range matches {
// 			if len(match) > 1 {
// 				seen[match[1]] = true
// 			}
// 		}

// 		var firstKey int
// 		for key := range seen {
// 			firstKey = idxToInt(key)
// 			break
// 		}
// 		Userset[i] = fmt.Sprintf("User%d", firstKey)

// 		if txs.MatchString(payload.String()) {
// 			// Readset과 Writeset을 설정
// 			for key := range seen {
// 				index := idxToInt(key) - 1
// 				Readset[i][index] = 1
// 			}
// 		} else {
// 			// Readset과 Writeset을 설정
// 			for key := range seen {
// 				index := idxToInt(key) - 1
// 				Readset[i][index] = 1
// 				Writeset[i][index] = 1
// 			}
// 		}

// 	}

// 	fmt.Println(Readset)
// 	fmt.Println(Writeset)

// 	for i := 0; i < NumactualorderTx; i++ {
// 		for j := 0; j < Numdataitem; j++ {
// 			if Readset[i][j] == 1 {
// 				for k := 0; k < NumactualorderTx; k++ {
// 					if Writeset[k][j] == 1 && k != i {
// 						Conflictgraph[k][i] = 1
// 					}
// 				}
// 			}
// 		}
// 	}

// 	fmt.Print(Conflictgraph)

// 	// for _, index := range order {
// 	// 	tSerial = append(tSerial, msg[index-1])
// 	// }

// 	// for _, index := range aborted {
// 	// 	tAbort = append(tAbort, msg[index-1])
// 	// }

// 	return tSerial, tAbort
// }

// func idxToInt(s string) int {
// 	num, err := strconv.Atoi(s)
// 	if err != nil {
// 		return 0
// 	}
// 	return num
// }

// // Write a processor that consumes data from Kafka
// func (bs *Blocks) runProcessor() {
// 	channel := "mychannel-incoming"
// 	// for {
// 	// 	select {
// 	// 	case channel := <-bs.PeerContribution.ChannelTrigger:
// 	fmt.Println("New Channel:", channel)
// 	tm, err := goka.NewTopicManager(brokers2, goka.DefaultConfig(), tmc)
// 	if err != nil {
// 		log.Fatalf("Error creating topic manager: %v", err)
// 	}
// 	defer tm.Close()
// 	err = tm.EnsureStreamExists(string(channel), 3)
// 	if err != nil {
// 		log.Printf("Error creating kafka topic %s: %v", topiccommit, err)
// 	}
// 	err = tm.EnsureStreamExists(string("mychannel-commit"), 3)
// 	if err != nil {
// 		log.Printf("Error creating kafka topic %s: %v", topiccommit, err)
// 	}

// 	func() {
// 		initiating := make(chan struct{})
// 		var newBlock Block

// 		topicStream := goka.Stream(channel)
// 		newBlock.ChannelID = channel
// 		newBlock.OutputStream = goka.Stream("mychannel-abort")

// 		bs.block = append(bs.block, newBlock)
// 		group := goka.Group(channel + "-group")
// 		g := goka.DefineGroup(group,
// 			goka.Input(topicStream, new(codec.Bytes), bs.process), // function for receiving messages(stream) from Kafka
// 			goka.Loop(new(codec.Bytes), bs.loopProcess),           // re-key using status
// 			goka.Output(topiccommit, new(codec.Bytes)),
// 			goka.Output(topicabort, new(codec.Bytes)),
// 			goka.Persist(new(blockCodec)), // required for stateful-based data(table) processing
// 		)
// 		p, err := goka.NewProcessor(brokers2,
// 			g,
// 			goka.WithTopicManagerBuilder(goka.TopicManagerBuilderWithTopicManagerConfig(tmc)),
// 			goka.WithConsumerGroupBuilder(goka.DefaultConsumerGroupBuilder),
// 		)
// 		if err != nil {
// 			panic(err)
// 		}

// 		close(initiating)

// 		if err = p.Run(context.Background()); err != nil {
// 			log.Printf("Error running processor: %v", err)
// 		}
// 	}()
// }

// // Writing a view to query the user table
// func (bs *Blocks) runView(initialized chan struct{}) {
// 	<-initialized

// 	channel := <-bs.PeerContribution.ChannelTrigger
// 	group := goka.Group(channel + "-group")
// 	view, err := goka.NewView(brokers2,
// 		goka.GroupTable(group),
// 		new(blockCodec),
// 	)
// 	if err != nil {
// 		panic(err)
// 	}

// 	view.Run(context.Background())
// }
// func main() {

// 	// When this example is run the first time, wait for creation of all internal topics (this is done
// 	// by goka.NewProcessor)
// 	initialized := make(chan struct{})

// 	kafka := flag.String("broker", "117.16.244.33", "")
// 	flag.Parse()

// 	brokers1 = "" + *kafka + ":9091, " + *kafka + ":9092, " + *kafka + ":9093" + ""
// 	brokers2 = []string{*kafka + ":9091", *kafka + ":9092", *kafka + ":9093"}

// 	go weavehttp.Http3Listen()

// 	bs := Blocks{
// 		PeerContribution: contribution.FabricChannel{
// 			ChannelTrigger: make(chan string),
// 		},
// 	}

// 	go bs.PeerContribution.Start(brokers1)

// 	go bs.runProcessor()

// 	bs.runView(initialized)
// }
