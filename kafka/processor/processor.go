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
		return
	}

	var Ordereddata, Abortdata, Mediumdata, Lowdata []*common.Envelope

	for _, data := range kafkadata {
		payload, _ := protoutil.UnmarshalTransaction(data.Payload)
		payloadStr := payload.String()

		if strings.Contains(payloadStr, `Query`) {
			Ordereddata = append(Ordereddata, data)
		} else if strings.Contains(payloadStr, `"Urgency\":true`) {
			Mediumdata = append(Mediumdata, data)
		} else {
			Lowdata = append(Lowdata, data)
		}
	}

	// ( 긴급한 트랜잭션을 먼저 정렬하고 Low를 따로 정렬하면 Low단에서 문제 발생하지 않나? )
	if len(Mediumdata) > 0 {
		SerialMediumTx, AbortMediumTx := epsilonOrdering(true, Mediumdata)
		Ordereddata = append(Ordereddata, reverseArray(SerialMediumTx)...)
		Abortdata = append(Abortdata, reverseArray(AbortMediumTx)...)
	}

	// 낮은 우선순위 트랜잭션 처리
	if len(Lowdata) > 0 {
		SerialLowTx, AbortLowTx := epsilonOrdering(false, Lowdata)
		Ordereddata = append(Ordereddata, reverseArray(SerialLowTx)...)
		Abortdata = append(Abortdata, reverseArray(AbortLowTx)...)
	}

	marshalledOrdereddata, _ := json.Marshal(Ordereddata)
	marshalledAbortdata, _ := json.Marshal(Abortdata)

	ctx.Emit(topiccommit, strconv.Itoa(int(ctx.Offset())), marshalledOrdereddata)
	ctx.Emit(topicabort, strconv.Itoa(int(ctx.Offset())), marshalledAbortdata)

	fmt.Println("[Total Time]", time.Now().UnixMilli()-startelsaped)

}

func epsilonOrdering(urgency bool, msg []*common.Envelope) (tSerial []*common.Envelope, tAbort []*common.Envelope) {

	var NumactualorderTx = 0
	// 미리 만들어놓은 유저의 크기에 맞게.
	var Numdataitem = 4

	// 급한 트랜잭션은 epsilon urgent값을 이용, 급하지 않다면 epsilon low 값을 이용
	if urgency {
		NumactualorderTx = int(math.Ceil(epsilonUrgent * float64(len(msg))))
	} else {
		NumactualorderTx = int(math.Ceil(epsilonLow * float64(len(msg))))
	}

	Conflictgraph := make([][]int, NumactualorderTx)
	Readset := make([][]int, NumactualorderTx)
	Writeset := make([][]int, NumactualorderTx)
	Userset := make([]string, NumactualorderTx)

	for i := 0; i < NumactualorderTx; i++ {
		Conflictgraph[i] = make([]int, NumactualorderTx)
		Readset[i] = make([]int, Numdataitem)
		Writeset[i] = make([]int, Numdataitem)
	}

	re := regexp.MustCompile(`User(\d+)`)

	for i := 0; i < NumactualorderTx; i++ {
		payload, _ := protoutil.UnmarshalTransaction(msg[i].Payload)
		matches := re.FindAllStringSubmatch(payload.String(), -1)

		seen := make(map[string]bool, len(matches))
		for _, match := range matches {
			if len(match) > 1 {
				seen[match[1]] = true
			}
		}

		// seen 맵에서 첫 번째 값을 얻음
		var firstKey int
		for key := range seen {
			firstKey = idxToInt(key)
			break
		}
		Userset[i] = fmt.Sprintf("User%d", firstKey)

		// Readset과 Writeset을 설정
		for key := range seen {
			index := idxToInt(key) - 1
			Readset[i][index] = 1
			Writeset[i][index] = 1
		}
	}

	for i := 0; i < NumactualorderTx; i++ {
		for j := 0; j < Numdataitem; j++ {
			if Readset[i][j] == 1 {
				for k := 0; k < NumactualorderTx; k++ {
					if Writeset[k][j] == 1 && k != i {
						Conflictgraph[k][i] = 1
					}
				}
			}
		}
	}

	order, aborted := transactionScheduler(Userset, Conflictgraph)

	for _, index := range order {
		tSerial = append(tSerial, msg[index-1])
	}

	for _, index := range aborted {
		tAbort = append(tAbort, msg[index-1])
	}

	return tSerial, tAbort
}

func idxToInt(s string) int {
	num, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return num
}

func reverseArray(arr []*common.Envelope) []*common.Envelope {
	reversed := make([]*common.Envelope, len(arr))
	copy(reversed, arr)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed
}

func (g *Graph) AddEdge(from, to int) {
	if len(g.edges[from]) == 0 {
		g.edges[from] = make([]int, 0, 10)
	}
	g.edges[from] = append(g.edges[from], to)
}

func (g *Graph) RemoveNode(node int) {
	delete(g.edges, node)
	for from, edges := range g.edges {
		// 새로운 슬라이스로 필터링된 값을 저장
		filteredEdges := edges[:0]
		for _, to := range edges {
			if to != node {
				filteredEdges = append(filteredEdges, to)
			}
		}
		g.edges[from] = filteredEdges
	}
}

func hasCycleUtil(graph *Graph, node int, visited, recStack map[int]bool) bool {
	if recStack[node] {
		// 이미 순환을 찾았으면 바로 종료
		return true
	}
	if visited[node] {
		return false
	}

	visited[node] = true
	recStack[node] = true

	for _, neighbor := range graph.edges[node] {
		if hasCycleUtil(graph, neighbor, visited, recStack) {
			return true
		}
	}

	recStack[node] = false
	return false
}

func detectCycle(graph *Graph) bool {
	visited := make(map[int]bool, len(graph.edges))
	recStack := make(map[int]bool, len(graph.edges))

	for node := range graph.edges {
		if !visited[node] && hasCycleUtil(graph, node, visited, recStack) {
			return true
		}
	}
	return false
}

// 사용자의 트랜잭션 실패율을 최소화하도록 순서를 정렬
func sortNodesByAbortInfo(size int, abortInfo interface{}) []int {
	nodes := make([]int, 0, size)
	for node := 1; node <= size; node++ {
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {

		valueI := getAbortCount(abortInfo, fmt.Sprintf("User%d", nodes[i]))
		valueJ := getAbortCount(abortInfo, fmt.Sprintf("User%d", nodes[j]))

		return valueI < valueJ
	})
	return nodes
}

// watchdog으로 부터 데이터를 받아서 정리
func getAbortCount(abortInfo interface{}, user string) int32 {
	abortInfoMap, ok := abortInfo.(bson.M)
	if !ok {
		return 0
	}
	if val, exists := abortInfoMap[user]; exists {
		if intValue, ok := val.(int32); ok {
			return intValue
		}
	}
	return 0
}

func transactionScheduler(userinfo []string, matrix [][]int) ([]int, []int) {

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
	abortListMap := make(map[int]bool)
	abortInfo := UserabortInfoinWnode()

	userToTransactions := make(map[int][]int)

	for i, user := range userinfo {
		userID := user[len(user)-1] - '0'
		userToTransactions[int(userID)] = append(userToTransactions[int(userID)], i+1)
	}

	sortedNodes := sortNodesByAbortInfo(size, abortInfo)
	for _, user := range sortedNodes {
		transactions, exists := userToTransactions[user]
		if !exists {
			continue
		}

		for _, tx := range transactions {
			if abortListMap[tx] {
				continue
			}

			if detectCycle(graph) {
				abortListMap[tx] = true
				graph.RemoveNode(tx)
				abortList = append(abortList, tx)
				fmt.Printf("Removed transaction %d (User: User%d)\n", tx, user)
				break
			} else {
				fmt.Printf("Transaction %d (User: User%d) does not cause a cycle. Skipping.\n", tx, user)
			}
		}
	}

	successList := []int{}
	for i := 1; i <= size; i++ {
		if !abortListMap[i] {
			successList = append(successList, i)
		}
	}

	fmt.Println("SUC", successList)
	fmt.Println("ABO", abortList)

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
		return nil
	}
	if abortCount, exists := result["NumofAbortTransaction"]; exists {
		return abortCount
	}

	return nil
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
