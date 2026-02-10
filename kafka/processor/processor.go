package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"weave-kafka/contribution"
	weavehttp "weave-kafka/http"

	types "github.com/Watchdog-Network/types"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protoutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/golang/protobuf/proto"

	"github.com/lovoo/goka"
	"github.com/lovoo/goka/codec"
)

const (
	TopicAbort  goka.Stream = "mychannel-abort"
	TopicCommit goka.Stream = "mychannel-commit"
	Group       goka.Group  = "mychannel-group"

	EpsilonUrgent = 1.0
	EpsilonLow    = 1.0
)

var (
	re = regexp.MustCompile(`User(\d+)`)
	tmc *goka.TopicManagerConfig

	brokers1 string
	brokers2 []string
)

type Blocks struct {
	List             []Block
	PeerContribution contribution.FabricChannel
}

type Block struct {
	ChannelID        string
	OutputStream     goka.Stream
	Peers            []PeerInfo
	PeerContribution contribution.FabricChannel
}

type PeerInfo struct {
	ID   string
	Sent int
}

type Graph struct {
	edges map[int][]int
	users map[int]string
}

func NewGraph(size int) *Graph { return &Graph{edges: make(map[int][]int)} }

func init() {
	tmc = goka.NewTopicManagerConfig()
	tmc.Table.Replication = 3
	tmc.Stream.Replication = 3
}

type blockCodec struct{}

func newBlockCodec() *blockCodec {
	return &blockCodec{}
}

// Encode encodes *types.StateData into []byte
func (bc *blockCodec) Encode(value interface{}) ([]byte, error) {
	state, ok := value.(*types.StateData)
	if !ok {
		return nil, fmt.Errorf("blockCodec: expected *types.StateData, got %T", value)
	}
	return json.Marshal(state)
}

// Decode decodes []byte into *types.StateData
func (bc *blockCodec) Decode(data []byte) (interface{}, error) {
	var c types.StateData
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("blockCodec: unmarshal error: %w", err)
	}
	return &c, nil
}

func (bs *Blocks) process(ctx goka.Context, msg interface{}) {

	// bs.PeerContribution.Record(ctx, msg)
	key := ctx.Offset()
	ctx.Loopback(strconv.Itoa(int(key)), msg)
}

type TxMeta struct {
	Index   int
	User    string
	Reads   []string
	Writes  []string
	Urgency bool
}

type TxData struct {
	Timestamp int64    `json:"timestamp"`
	Batch     [][]byte `json:"batch"`
}

type AbortPayload struct {
	Timestamp int64                `json:"timestamp"`
	Data      []*common.Envelope `json:"data"`
}

func (bs *Blocks) loopProcess(ctx goka.Context, msg interface{}) {
	startelsaped:=time.Now().UnixMilli()

	var transactionlist TxData
	err := json.Unmarshal(msg.([]byte), &transactionlist)
	if err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	
	var kafkadata []*common.Envelope
	for _, serializedEnv := range transactionlist.Batch {
		env := &common.Envelope{}
		err := proto.Unmarshal(serializedEnv, env)
		if err != nil {
			log.Fatalf("Failed to unmarshal envelope: %v", err)
		}
		kafkadata = append(kafkadata, env)
	}

	fmt.Println("[The length of Kafka incoming data]", len(kafkadata),transactionlist.Timestamp)
	if len(kafkadata) == 1 {
		data, _ := json.Marshal(kafkadata)
		ctx.Emit(TopicCommit, strconv.Itoa(int(ctx.Offset())), data)
		return
	}

	var (
		originalDataCap = len(kafkadata) 
		originalData    = make([]*common.Envelope, 0, originalDataCap)
		OrderedData		= make([]*common.Envelope, 0, originalDataCap)
		AbortData		= make([]*common.Envelope, 0, originalDataCap)
		txMetaList      = make([]TxMeta, 0, originalDataCap)
	)
	
	for i, data := range kafkadata {
		payload := protoutil.UnmarshalPayloadOrPanic(data.Payload)
		transaction, _ := protoutil.UnmarshalTransaction(payload.Data)
	
		for _, action := range transaction.Actions {
			meta := TxMeta{Index: i}
	
			chaincodeActionPayload, _ := protoutil.UnmarshalChaincodeActionPayload(action.Payload)
			responsePayload, _ := protoutil.UnmarshalProposalResponsePayload(chaincodeActionPayload.Action.ProposalResponsePayload)
			chaincodeAction, _ := protoutil.UnmarshalChaincodeAction(responsePayload.Extension)
	
			txRWSet := &rwsetutil.TxRwSet{}
			if err = txRWSet.FromProtoBytes(chaincodeAction.Results); err != nil {
				fmt.Println("RWSet unmarshal error:", err)
				continue
			}
	
			firstKeyFound := false
	
			for _, ns := range txRWSet.NsRwSets {
				for _, r := range ns.KvRwSet.Reads {
					if r.Key == "namespaces/fields/basic/Sequence" {
						continue
					}
					if !firstKeyFound {
						meta.User = r.Key
						firstKeyFound = true
					}
					meta.Reads = append(meta.Reads, r.Key)
				}
	
				for _, w := range ns.KvRwSet.Writes {
					if !firstKeyFound {
						meta.User = w.Key
						firstKeyFound = true
					}
					meta.Writes = append(meta.Writes, w.Key)
	
					var valMap map[string]interface{}
					if err := json.Unmarshal(w.Value, &valMap); err == nil {
						if userInfo, ok := valMap["UserInfo"].(map[string]interface{}); ok {
							if urgency, ok := userInfo["Urgency"].(bool); ok && urgency {
								meta.Urgency = true
							}
						}
					}
				}
			}
	
			txMetaList = append(txMetaList, meta)
			originalData = append(originalData, data)
		}
	}

	var (
		urgentList       []TxMeta
		normalList       []TxMeta
		committedUrgent  []TxMeta
	)
	for _, meta := range txMetaList {
		switch {
		case len(meta.Reads) > 0 && len(meta.Writes) == 0:
			OrderedData = append(OrderedData, originalData[meta.Index])
		case meta.Urgency:
			urgentList = append(urgentList, meta)
		default:
			normalList = append(normalList, meta)
		}
	}

	if len(urgentList) > 0 {
		serialIdx, abortUrgent := epsilonOrdering(true, urgentList, EpsilonUrgent)

		for _, i := range serialIdx {
			committedUrgent = append(committedUrgent, txMetaList[i])
			OrderedData = append(OrderedData, originalData[i])
		}
		for _, i := range abortUrgent {
			AbortData = append(AbortData, originalData[i])
		}
		SaveAbortCount(urgentList, abortUrgent)
	}

	if len(normalList) > 0 {
		serialTX, abortBetween := checkbetweennormalandurgent(normalList, committedUrgent)

		for _, tx := range abortBetween {
			AbortData = append(AbortData, originalData[tx.Index])
		}

		serialIdx, abortNormal := epsilonOrdering(false, serialTX, EpsilonLow)

		for _, i := range serialIdx {
			OrderedData = append(OrderedData, originalData[serialTX[i].Index])
		}
		for _, i := range abortNormal {
			AbortData = append(AbortData, originalData[serialTX[i].Index])
		}
	}

	OrderedData = reverseArray(OrderedData)

	marshalledOrdereddata, _ := json.Marshal(OrderedData)

	abortPayload := &AbortPayload{
		Timestamp: time.Now().UnixMilli(),
		Data:      make([]*common.Envelope, 0, len(AbortData)),
	}

	abortPayload.Data = append(abortPayload.Data, AbortData...)
	marshalledAbortdata, err := json.Marshal(abortPayload)
	if err != nil {
		fmt.Printf("Failed to marshal abortPayload: %v\n", err)
	}

	ctx.Emit(TopicAbort, strconv.Itoa(int(ctx.Offset())), marshalledAbortdata)
	ctx.Emit(TopicCommit, strconv.Itoa(int(ctx.Offset())), marshalledOrdereddata)
	fmt.Println("[Total Ordering Time]", time.Now().UnixMilli()-startelsaped)
}


func reverseArray(arr []*common.Envelope) []*common.Envelope {
	reversed := make([]*common.Envelope, len(arr))
	copy(reversed, arr)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed
}

func checkbetweennormalandurgent(normalList []TxMeta, committedUrgent []TxMeta) (serialIdx, abortIdx []TxMeta) {
	committedUsers := make(map[string]struct{})
	for _, tx := range committedUrgent {
		committedUsers[tx.User] = struct{}{}
		for _, w := range tx.Writes {
			committedUsers[w] = struct{}{}
		}
	}

	for _, tx := range normalList {
		conflict := false

		if _, exists := committedUsers[tx.User]; exists {
			conflict = true
		}

		if !conflict {
			for _, w := range tx.Writes {
				if _, exists := committedUsers[w]; exists {
					conflict = true
					break
				}
			}
		}

		if conflict {
			abortIdx = append(abortIdx, tx)
		} else {
			serialIdx = append(serialIdx, tx)
		}
	}

	return serialIdx, abortIdx
}

func BuildRWSetGraph(txList []TxMeta, keyCount int) (ConflictGraph [][]int) {
	n := len(txList)
	ConflictGraph = make([][]int, n)
	Readset := make([][]int, n)
	Writeset := make([][]int, n)

	for i := 0; i < n; i++ {
		ConflictGraph[i] = make([]int, n)
		Readset[i] = make([]int, keyCount)
		Writeset[i] = make([]int, keyCount)

		for _, key := range txList[i].Reads {
			idx := idxToInt(key)
			Readset[i][idx] = 1
		}
		for _, key := range txList[i].Writes {
			idx := idxToInt(key)
			Writeset[i][idx] = 1
		}
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			for k := 0; k < keyCount; k++ {
				// // Write → Read
				// if Writeset[i][k] == 1 && Readset[j][k] == 1 {
				// 	ConflictGraph[i][j] = 1
				// }
				// Read → Write
				if Readset[i][k] == 1 && Writeset[j][k] == 1 {
					ConflictGraph[i][j] = 1
				}
				// Write → Write
				if Writeset[i][k] == 1 && Writeset[j][k] == 1 {
					ConflictGraph[i][j] = 1
				}
			}
		}
	}

	return ConflictGraph
}

func idxToInt(s string) int {
	s = strings.TrimPrefix(s, "User")
	num, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return num
}

// func epsilonOrdering(urgency bool, transactiongraph []TxMeta) (tSerial []*common.Envelope, tAbort []*common.Envelope) {
func epsilonOrdering(urgency bool, transactiongraph []TxMeta, epsilon float64) (tSerialIndex []int, tAbortIndex []int) {

	var Numkeyitem = 10000
	ConflictGraph := BuildRWSetGraph(transactiongraph, Numkeyitem)

	// var NumactualorderTx = int(math.Ceil(EpsilonUrgent * float64(len(msg))))

	// for i := range ConflictGraph {
	// 	fmt.Println("Tx", transactiongraph[i].Index, "Conflict:", ConflictGraph[i])
	// }
	var txindexList []int
	var userList []string
	for _, tx := range transactiongraph {
		txindexList = append(txindexList, tx.Index)
		userList = append(userList, tx.User)
	}
	order, aborted := transactionScheduler(txindexList, userList, ConflictGraph)

	return order, aborted

}

// 사용자의 트랜잭션 실패율을 최소화하도록 순서를 정렬
func sortNodesByAbortInfo(size int, abortInfo interface{}, txToUser map[int]int) []int {
	nodes := make([]int, 0, size)
	for node := 1; node <= size; node++ {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		userI, existsI := txToUser[nodes[i]]
		userJ, existsJ := txToUser[nodes[j]]

		if !existsI || !existsJ {
			return false
		}

		valueI := getAbortCount(abortInfo, fmt.Sprintf("User%d", userI))
		valueJ := getAbortCount(abortInfo, fmt.Sprintf("User%d", userJ))

		// fmt.Println(nodes[i], "→ User", userI, nodes[j], "→ User", userJ, ":", valueI, valueJ)

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

func checkConflict(graph *Graph, tx int) []int {
	conflictingTx := []int{}

	for node, neighbors := range graph.edges {
		if node == tx {
			continue
		}
		for _, neighbor := range neighbors {
			if neighbor == tx {
				conflictingTx = append(conflictingTx, node) // 충돌이 발생한 트랜잭션을 기록
			}
		}
	}
	return conflictingTx
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
		filteredEdges := edges[:0]
		for _, to := range edges {
			if to != node {
				filteredEdges = append(filteredEdges, to)
			}
		}
		g.edges[from] = filteredEdges
	}
}

func updateDependencies(graph *Graph, removedTx int) {

	for node, dependencies := range graph.edges {
		newDeps := []int{}
		for _, dep := range dependencies {
			if dep != removedTx {
				newDeps = append(newDeps, dep)
			}
		}
		graph.edges[node] = newDeps
	}
}

func transactionScheduler(txinfo []int, userinfo []string, matrix [][]int) ([]int, []int) {
	size := len(matrix)
	graph := NewGraph(size)
	for _, tx := range txinfo {
		graph.edges[tx] = []int{}
	}
	for i := 0; i < len(matrix); i++ {
		for j := 0; j < len(matrix); j++ {
			if matrix[i][j] == 1 {
				from := txinfo[i]
				to := txinfo[j]
				graph.AddEdge(from, to)
			}
		}
	}

	txToUser := make(map[int]string)
	for _, info := range userinfo {
		parts := strings.Fields(info)
		if len(parts) < 2 {
			continue
		}
		txID, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		user := parts[1]
		txToUser[txID] = user
	}

	successList := []int{}
	abortList := []int{}

	abortInfo := GetAbortCounts()
	fmt.Println(abortInfo)

	for len(graph.edges) > 0 {
		zeroInDegree := []int{}

		for node := range graph.edges {
			incoming := false
			for _, neighbors := range graph.edges {
				for _, to := range neighbors {
					if to == node {
						incoming = true
						break
					}
				}
				if incoming {
					break
				}
			}
			if !incoming {
				zeroInDegree = append(zeroInDegree, node)
			}
		}

		if len(zeroInDegree) == 0 {
			var nodeToAbort int
			for node := range graph.edges {
				nodeToAbort = node
				break
			}
			abortList = append(abortList, nodeToAbort)
			graph.RemoveNode(nodeToAbort)
			continue
		}

		for _, node := range zeroInDegree {
			successList = append(successList, node)
			graph.RemoveNode(node)
		}
	}
	fmt.Println("SUC", successList)
	fmt.Println("ABO", abortList)
	return successList, abortList
}

// func UserabortInfoinWnode() interface{} {
// 	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	client, err := mongo.Connect(ctx, clientOptions)
// 	if err != nil {
// 		log.Fatalf("Failed to connect to MongoDB: %v", err)
// 	}
// 	defer func() {
// 		if err := client.Disconnect(ctx); err != nil {
// 			log.Fatalf("Failed to disconnect MongoDB: %v", err)
// 		}
// 	}()
// 	database := client.Database("User")
// 	collection := database.Collection("abortTransaction")

// 	opts := options.FindOne().SetSort(bson.D{{"_id", -1}})

// 	var result bson.M
// 	err = collection.FindOne(ctx, bson.D{}, opts).Decode(&result)
// 	if err != nil {
// 		if err == mongo.ErrNoDocuments {
// 			fmt.Println("No documents found")
// 		} else {
// 			log.Fatalf("Failed to fetch latest document: %v", err)
// 		}
// 		return nil
// 	}
// 	if abortCount, exists := result["NumofAbortTransaction"]; exists {
// 		fmt.Println(abortCount)
// 		return abortCount
// 	}

// 	return nil
// }

func SaveAbortCount(txUsers []TxMeta, abortIDs []int) {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("MongoDB 연결 실패: %v", err)
	}
	defer client.Disconnect(ctx)

	collection := client.Database("User").Collection("abortTransaction")

	// abort count 집계
	abortCount := make(map[string]int)
	for _, abortID := range abortIDs {
		for _, tx := range txUsers {
			if tx.Index == abortID {
				abortCount[tx.User]++
			}
		}
	}

	// 유저별로 MongoDB에 누적 업데이트 (Upsert)
	for user, count := range abortCount {
		filter := bson.M{"user": user}
		update := bson.M{"$inc": bson.M{"abortCount": count}}
		opts := options.Update().SetUpsert(true)
		_, err := collection.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			log.Fatalf("MongoDB 업데이트 실패: %v", err)
		}
	}

}

// GetAbortCounts: 전체 유저별 누적 abortCount 조회
func GetAbortCounts() map[string]int {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("MongoDB 연결 실패: %v", err)
	}
	defer client.Disconnect(ctx)

	collection := client.Database("User").Collection("abortTransaction")

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Fatalf("MongoDB 조회 실패: %v", err)
	}
	defer cursor.Close(ctx)

	result := make(map[string]int)
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			log.Fatalf("Decode 실패: %v", err)
		}

		// nil 체크 후 타입 변환
		user, ok := doc["user"].(string)
		if !ok || user == "" {
			continue // user 필드 없으면 건너뜀
		}

		var count int
		switch v := doc["abortCount"].(type) {
		case int32:
			count = int(v)
		case int64:
			count = int(v)
		case int:
			count = v
		default:
			count = 0
		}

		result[user] = count
	}

	return result
}

// Write a processor that consumes data from Kafka
func (bs *Blocks) runProcessor() {
	channel := "mychannel-incoming"
	fmt.Println("Starting processor for channel:", channel)
	tm, err := goka.NewTopicManager(brokers2, goka.DefaultConfig(), tmc)
	if err != nil {
		log.Fatalf("Error creating topic manager: %v", err)
	}
	defer tm.Close()

	for _, topic := range []goka.Stream{goka.Stream(channel), TopicCommit, TopicAbort} {
		if err := tm.EnsureStreamExists(string(topic), 3); err != nil {
			log.Printf("Error creating kafka topic %s: %v", topic, err)
		}
	}

	newBlock := Block{
		ChannelID:    channel,
		OutputStream: TopicAbort,
	}
	bs.List = append(bs.List, newBlock)

	group := goka.Group(channel + "-group")
	g := goka.DefineGroup(
		group,
		goka.Input(goka.Stream(channel), new(codec.Bytes), bs.process),
		goka.Loop(new(codec.Bytes), bs.loopProcess),
		goka.Output(TopicCommit, new(codec.Bytes)),
		goka.Output(TopicAbort, new(codec.Bytes)),
		goka.Persist(new(blockCodec)),
	)

	p, err := goka.NewProcessor(
		brokers2,
		g,
		goka.WithTopicManagerBuilder(goka.TopicManagerBuilderWithTopicManagerConfig(tmc)),
		goka.WithConsumerGroupBuilder(goka.DefaultConsumerGroupBuilder),
	)
	if err != nil {
		fmt.Errorf("creating processor: %w", err)
	}

	p.Run(context.Background())
}

// Writing a view to query the user table
func (bs *Blocks) runView(initialized chan struct{}) error {
	<-initialized

	channel := <-bs.PeerContribution.ChannelTrigger
	group := goka.Group(channel + "-group")
	view, err := goka.NewView(brokers2, goka.GroupTable(group), new(blockCodec))
	if err != nil {
		return fmt.Errorf("creating view: %w", err)
	}
	return view.Run(context.Background())
}
func main() {

	kafka := flag.String("broker", "117.16.244.33", "Kafka broker")
	bind := flag.String("bind", ":8082", "server bind address")
    tcp := flag.Bool("tcp", false, "also listen on TCP")
	flag.Parse()

	brokers1 = "" + *kafka + ":9091, " + *kafka + ":9092, " + *kafka + ":9093" + ""
	brokers2 = []string{
		fmt.Sprintf("%s:9091", *kafka),
		fmt.Sprintf("%s:9092", *kafka),
		fmt.Sprintf("%s:9093", *kafka),
	}

	go weavehttp.Http3Listen(*bind,*tcp)

	bs := Blocks{
		PeerContribution: contribution.FabricChannel{
			ChannelTrigger: make(chan string),
		},
	}

	go bs.PeerContribution.Start(brokers1)

	go func() {
		bs.runProcessor()
	}()

	initialized := make(chan struct{})
	go func() {
		if err := bs.runView(initialized); err != nil {
			log.Fatalf("View error: %v", err)
		}
	}()

	select {} 
}
