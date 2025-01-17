package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"weave-kafka/contribution"
	weavehttp "weave-kafka/http"

	// contribution "weave-kafka/contribution"

	types "github.com/Watchdog-Network/types"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/lovoo/goka"
	"github.com/lovoo/goka/codec"
)

type Blocks struct {
	block            []Block
	PeerContribution contribution.FabricChannel
}

type Block struct {
	ChannelID    string
	OutputStream goka.Stream

	Peers            []PeerInfo
	PeerContribution contribution.FabricChannel
}

type PeerInfo struct {
	ID   string
	Sent int
}

var (
	brokers1 string
	brokers2 []string

	topicabort  goka.Stream = "mychannel-abort"
	topiccommit goka.Stream = "mychannel-commit"
	group       goka.Group  = "mychannel-group"

	emitted = false

	tmc *goka.TopicManagerConfig
)
var (
	epsilonUrgent = 1.0
	epsilonLow    = 1.0
)

type Graph struct {
	edges map[int][]int
	users map[int]string
}

func NewGraph(size int) *Graph {
	return &Graph{edges: make(map[int][]int)}
}

// This codec allows marshalling (encode) and unmarshalling (decode) the block struct(or produce struct) to and from the group table
type blockCodec struct{}

func init() {
	tmc = goka.NewTopicManagerConfig()
	tmc.Table.Replication = 3
	tmc.Stream.Replication = 3
}

// Encodes types.StateData into []byte
func (bc *blockCodec) Encode(value interface{}) ([]byte, error) {
	if _, isState := value.(*types.StateData); !isState {
		return nil, fmt.Errorf("codec requires value *types.StateData, got %T", value)
	}
	return json.Marshal(value)
}

// Decodes a types.StateData from []byte to it's go representation
func (bc *blockCodec) Decode(data []byte) (interface{}, error) {
	var (
		c   types.StateData
		err error
	)
	err = json.Unmarshal(data, &c)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling types.StateData: %v", err)
	}
	return &c, nil
}

func (bs *Blocks) process(ctx goka.Context, msg interface{}) {

	// bs.PeerContribution.Record(ctx, msg)

	key := ctx.Offset()
	ctx.Loopback(strconv.Itoa(int(key)), msg)
}

func (bs *Blocks) loopProcess(ctx goka.Context, msg interface{}) {
	startelsaped := time.Now().UnixMilli()

	var kafkadata []*common.Envelope
	err := json.Unmarshal(msg.([]byte), &kafkadata)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("[The length of Kafka incoming data]", len(kafkadata))
	if len(kafkadata) == 1 {
		hello, _ := json.Marshal(kafkadata)
		ctx.Emit(topiccommit, strconv.Itoa(int(ctx.Offset())), hello)
	} else {

		var Ordereddata []*common.Envelope
		var Abortdata []*common.Envelope
		var Mediumdata []*common.Envelope
		var Lowdata []*common.Envelope

		for i := range kafkadata {
			payload, _ := protoutil.UnmarshalTransaction(kafkadata[i].Payload)
			if strings.Contains(payload.String(), `Queryy`) {
				Ordereddata = append(Ordereddata, kafkadata[i])
			} else if strings.Contains(payload.String(), `"Urgency\":true`) {
				// 급한 트랜잭션을 발행한 유저들은 우선순위를 두고.
				Mediumdata = append(Mediumdata, kafkadata[i])
			} else {
				// 이외의 트랜잭션들은 마지막으로.
				Lowdata = append(Lowdata, kafkadata[i])
			}
		}

		// ( 긴급한 트랜잭션을 먼저 정렬하고 Low를 따로 정렬하면 Low단에서 문제 발생하지 않나? )
		SerialMediumTx, AbortMediumTx := epsilonOrdering(true, Mediumdata)
		SerialLowTx, AbortLowTx := epsilonOrdering(false, Lowdata)

		// 정렬된 트랜잭션을 역으로 추가
		SerialMediumTx = reverseArray(SerialMediumTx)
		AbortMediumTx = reverseArray(AbortMediumTx)
		Ordereddata = append(Ordereddata, SerialMediumTx...)
		Abortdata = append(Abortdata, AbortMediumTx...)

		SerialLowTx = reverseArray(SerialLowTx)
		AbortLowTx = reverseArray(AbortLowTx)
		Ordereddata = append(Ordereddata, SerialLowTx...)
		Abortdata = append(Abortdata, AbortLowTx...)

		marshalledOrdereddata, _ := json.Marshal(Ordereddata)
		marshalledAbortdata, _ := json.Marshal(Abortdata)

		ctx.Emit(topiccommit, strconv.Itoa(int(ctx.Offset())), marshalledOrdereddata)
		ctx.Emit(topicabort, strconv.Itoa(int(ctx.Offset())), marshalledAbortdata)

		fmt.Println("[Total Time]", time.Now().UnixMilli()-startelsaped)

	}
}

func epsilonOrdering(urgency bool, msg []*common.Envelope) (tSerial []*common.Envelope, tAbort []*common.Envelope) {

	fmt.Println(len(msg))

	var NumactualorderTx = 0
	var Numdataitem = 10
	var index = 0

	// 급한 트랜잭션은 epsilon urgent값을 이용, 급하지 않다면 epsilon low 값을 이용
	if urgency {
		NumactualorderTx = int(math.Ceil(epsilonUrgent * float64(len(msg))))
	} else {
		NumactualorderTx = int(math.Ceil(epsilonLow * float64(len(msg))))
	}

	// 초기화
	Conflictgraph := make([][]int, NumactualorderTx)
	for i := 0; i < NumactualorderTx; i++ {
		Conflictgraph[i] = make([]int, NumactualorderTx)
	}

	// data에 대해 크기는 동일함.
	Readset := make([][]int, NumactualorderTx)
	Writeset := make([][]int, NumactualorderTx)
	for i := range Readset {
		Readset[i] = make([]int, Numdataitem)
		Writeset[i] = make([]int, Numdataitem)
	}

	// 트렌잭션 내용을 파악해서 그래프 만들기
	for i := 0; i < NumactualorderTx; i++ {
		payload, _ := protoutil.UnmarshalTransaction(msg[i].Payload)
		re := regexp.MustCompile(`User(\d+)`)
		seen := make(map[string]bool)

		// payload에서 모든 User 뒤 숫자 추출
		matches := re.FindAllStringSubmatch(payload.String(), -1)

		for _, match := range matches {
			if len(match) > 1 {
				idx := match[1]

				// 이미 처리한 숫자인지 확인
				if _, exists := seen[idx]; !exists {
					fmt.Println("Match:", idx) // 숫자 출력
					index = idxToInt(idx)      // 필요한 처리 수행

					// 숫자를 처리한 후, seen 맵에 추가
					seen[idx] = true
					fmt.Println("🚨", len(seen))
				}
			}
		}

		if len(seen) == 1 {
			fmt.Print(seen)
			userString := fmt.Sprintf("User%d", index)
			if strings.Contains(payload.String(), userString) {
				Readset[i][index-1] = 1
				Writeset[i][index-1] = 1
			}
		} else if len(seen) > 1 {
			fmt.Print(seen)
			userString := fmt.Sprintf("User%d", index)
			fmt.Println("🚨", userString, "🚨")
			if strings.Contains(payload.String(), userString) {
				Readset[i][index-1] = 1
				Writeset[i][index-1] = 1
			}
		}
	}

	//  write은 read에 영향을 미치니까 해당 부분은 1로 변경.
	for i := 0; i < len(Readset); i++ {
		for j := 0; j < len(Readset[i]); j++ {
			if Readset[i][j] == 1 {
				for k := 0; k < len(Readset); k++ {
					if Writeset[k][j] == 1 && k != i {
						Conflictgraph[k][i] = 1
					}
				}
			}
		}
	}

	order, aborted := transactionScheduler(Conflictgraph)

	for _, index := range order {
		tSerial = append(tSerial, msg[index-1])
	}

	for _, index := range aborted {
		tAbort = append(tAbort, msg[index-1])
	}

	return tSerial, tAbort
}

func idxToInt(s string) int {
	var num int
	fmt.Sscanf(s, "%d", &num)
	return num
}

func reverseArray(arr []*common.Envelope) []*common.Envelope {
	reversed := make([]*common.Envelope, len(arr)) // 원래 배열과 같은 크기의 배열 생성
	for i, v := range arr {
		reversed[len(arr)-1-i] = v // 뒤에서부터 값 추가
	}
	return reversed
}

func (g *Graph) AddEdge(from, to int) {
	g.edges[from] = append(g.edges[from], to)
}

func (g *Graph) RemoveNode(node int) {
	delete(g.edges, node)
	for from := range g.edges {
		newEdges := []int{}
		for _, to := range g.edges[from] {
			if to != node {
				newEdges = append(newEdges, to)
			}
		}
		g.edges[from] = newEdges
	}
}

func hasCycleUtil(graph *Graph, node int, visited, recStack map[int]bool) bool {
	visited[node] = true
	recStack[node] = true

	for _, neighbor := range graph.edges[node] {
		if !visited[neighbor] {
			if hasCycleUtil(graph, neighbor, visited, recStack) {
				return true
			}
		} else if recStack[neighbor] {
			return true
		}
	}

	recStack[node] = false
	return false
}

func detectCycle(graph *Graph, size int) bool {
	visited := make(map[int]bool)
	recStack := make(map[int]bool)

	for node := 1; node <= size; node++ {
		if !visited[node] {
			if hasCycleUtil(graph, node, visited, recStack) {
				return true
			}
		}
	}
	return false
}

func sortNodesByAbortInfo(graph *Graph, size int, abortInfo interface{}) []int {
	nodes := make([]int, 0, size)
	for node := 1; node <= size; node++ {
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		userI := graph.users[nodes[i]]
		userJ := graph.users[nodes[j]]

		valueI := getAbortCount(abortInfo, userI)
		valueJ := getAbortCount(abortInfo, userJ)

		return valueI > valueJ
	})

	return nodes
}

func getAbortCount(abortInfo interface{}, user string) int {
	abortInfoMap, ok := abortInfo.(map[string]interface{})
	if !ok {
		return 0
	}
	if val, exists := abortInfoMap[user]; exists {
		if intValue, ok := val.(int); ok {
			return intValue
		}
	}
	return 0
}

func transactionScheduler(matrix [][]int) ([]int, []int) {
	size := len(matrix)
	graph := NewGraph(size)

	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			if matrix[i][j] == 1 {
				graph.AddEdge(i+1, j+1)
			}
		}
	}

	abortList := []int{}

	abortInfo := UserabortInfoinWnode()
	fmt.Println("⭐️", abortInfo, "⭐️")

	for detectCycle(graph, size) {
		sortedNodes := sortNodesByAbortInfo(graph, size, abortInfo)

		for _, node := range sortedNodes {
			if !contains(abortList, node) {
				abortList = append(abortList, node)
				graph.RemoveNode(node)
				break
			}
		}
	}

	successList := []int{}
	for i := 1; i <= size; i++ {
		if !contains(abortList, i) {
			successList = append(successList, i)
		}
	}

	return successList, abortList
}

func UserabortInfoinWnode() interface{} {
	clientOptions := options.Client().ApplyURI("mongodb://127.0.0.1:27017")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Fatalf("Failed to disconnect MongoDB: %v", err)
		}
	}()
	database := client.Database("User")
	collection := database.Collection("abortTransaction")

	opts := options.FindOne().SetSort(bson.D{{"_id", -1}})

	var result bson.M
	err = collection.FindOne(ctx, bson.D{}, opts).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			fmt.Println("No documents found")
		} else {
			log.Fatalf("Failed to fetch latest document: %v", err)
		}
		return result["NumofAbortTransaction"]
	}
	return result["NumofAbortTransaction"]
}

func contains(list []int, value int) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// Write a processor that consumes data from Kafka
func (bs *Blocks) runProcessor() {
	channel := "mychannel-incoming"
	// for {
	// 	select {
	// 	case channel := <-bs.PeerContribution.ChannelTrigger:
	fmt.Println("New Channel:", channel)
	tm, err := goka.NewTopicManager(brokers2, goka.DefaultConfig(), tmc)
	if err != nil {
		log.Fatalf("Error creating topic manager: %v", err)
	}
	defer tm.Close()
	err = tm.EnsureStreamExists(string(channel), 3)
	if err != nil {
		log.Printf("Error creating kafka topic %s: %v", topiccommit, err)
	}
	err = tm.EnsureStreamExists(string("mychannel-commit"), 3)
	if err != nil {
		log.Printf("Error creating kafka topic %s: %v", topiccommit, err)
	}

	func() {
		initiating := make(chan struct{})
		var newBlock Block

		topicStream := goka.Stream(channel)
		newBlock.ChannelID = channel
		newBlock.OutputStream = goka.Stream("mychannel-abort")

		bs.block = append(bs.block, newBlock)
		group := goka.Group(channel + "-group")
		g := goka.DefineGroup(group,
			goka.Input(topicStream, new(codec.Bytes), bs.process), // function for receiving messages(stream) from Kafka
			goka.Loop(new(codec.Bytes), bs.loopProcess),           // re-key using status
			goka.Output(topiccommit, new(codec.Bytes)),
			goka.Output(topicabort, new(codec.Bytes)),
			goka.Persist(new(blockCodec)), // required for stateful-based data(table) processing
		)
		p, err := goka.NewProcessor(brokers2,
			g,
			goka.WithTopicManagerBuilder(goka.TopicManagerBuilderWithTopicManagerConfig(tmc)),
			goka.WithConsumerGroupBuilder(goka.DefaultConsumerGroupBuilder),
		)
		if err != nil {
			panic(err)
		}

		close(initiating)

		if err = p.Run(context.Background()); err != nil {
			log.Printf("Error running processor: %v", err)
		}
	}()
}

// Writing a view to query the user table
func (bs *Blocks) runView(initialized chan struct{}) {
	<-initialized

	channel := <-bs.PeerContribution.ChannelTrigger
	group := goka.Group(channel + "-group")
	view, err := goka.NewView(brokers2,
		goka.GroupTable(group),
		new(blockCodec),
	)
	if err != nil {
		panic(err)
	}

	view.Run(context.Background())
}
func main() {

	// When this example is run the first time, wait for creation of all internal topics (this is done
	// by goka.NewProcessor)
	initialized := make(chan struct{})

	kafka := flag.String("broker", "117.16.244.33", "")
	flag.Parse()

	brokers1 = "" + *kafka + ":9091, " + *kafka + ":9092, " + *kafka + ":9093" + ""
	brokers2 = []string{*kafka + ":9091", *kafka + ":9092", *kafka + ":9093"}

	go weavehttp.Http3Listen()

	bs := Blocks{
		PeerContribution: contribution.FabricChannel{
			ChannelTrigger: make(chan string),
		},
	}

	go bs.PeerContribution.Start(brokers1)

	go bs.runProcessor()

	bs.runView(initialized)
}
