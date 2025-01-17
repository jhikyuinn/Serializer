package main

// func hasConflict(index int, writeSet, readSet []int) bool {
// 	if writeSet[index] == readSet[index] {
// 		return true
// 	}
// 	return false
// }

// func TXCategorizing(ctx goka.Context, msg interface{}) (tSerial []*common.Envelope) {
//
//
// 	fmt.Println(msg)
// 	if strings.Contains(msg.(string), `"Urgency\":true`) {
// 		transactions = append(transactions, *transaction)
// 	} else {
// 		transaction := &Transaction{
// 			id:       len(transactions),
// 			readSet:  []int{1},
// 			writeSet: []int{2},
// 			urgent:   false,
// 			readOnly: false,
// 			msg:      msg.(string),
// 		}
// 		transactions = append(transactions, *transaction)
// 	}

// if len(transactions) > n {
// 	var tMedium, tLow []Transaction

// 	for i, ti := range transactions {
// 		if keyVersionInReadSet(i, ti.readSet, state) {
// 			ctx.Emit(topicabort, strconv.Itoa(int(ctx.Offset())), ti.msg)
// 		} else if ti.urgent {
// 			tMedium = append(tMedium, ti)
// 		} else {
// 			tLow = append(tLow, ti)
// 		}
// 	}
// 	for _, ti := range transactions {
// 		if keyVersionInReadSet(ti.readSet, state) {
// 			ctx.Emit(topicabort, strconv.Itoa(int(ctx.Offset())), ti.msg)
// 		} else if ti.urgent {
// 			tMedium = append(tMedium, ti)
// 		} else {
// 			tLow = append(tLow, ti)
// 		}
// 	}
// 	transactions = transactions[:0]

// 	// Ordering urgent priority transactions
// 	fmt.Println("URGENT")
// 	tAbort, tSerial, state = epsilonOrdering(tMedium, tAbort, tSerial, state, epsilonUrgent)

// 	// Step 2: Process low-priority transactions
// 	for i, ti := range tLow {
// 		if keyVersionInReadSet(i, ti.readSet, state) {
// 			tAbort = append(tAbort, ti)
// 		} else {
// 			tSerial = append(tSerial, ti)
// 		}
// 	}

// 	for _, ti := range tLow {
// 		if keyVersionInReadSet(ti.readSet, state) {
// 			tAbort = append(tAbort, ti)
// 		} else {
// 			tSerial = append(tSerial, ti)
// 		}
// 	}

// 	// Ordering low priority transactions
// 	fmt.Println("LOW")
// 	tAbort, tSerial, state = epsilonOrdering(tLow, tAbort, tSerial, state, epsilonLow)

// 	fmt.Println(tSerial)
// 	fmt.Println(time.Now().UnixMilli() - timestate[0])
// 	timestate = timestate[:0]
// 	return tAbort, tSerial
// }
// 	return
// }

// func epsilonOrdering(T []Transaction, tAbort, tSerial []Transaction, state []int, epsilon float64) ([]Transaction, []Transaction, []int) {
// Step 1: 초기화
// k := int(math.Ceil(epsilon * float64(len(T)))) // k = ⌈ε⋅|T|⌉
// tEpsilon := []Transaction{}
// M := make([][]int, k)
// for i := 0; i < k; i++ {
// 	M[i] = make([]int, k)
// }
// // Step 2: 앱실론에 맞게 구역 정하기
// for i := 0; i < k; i++ {
// 	tEpsilon = append(tEpsilon, T[i])
// }

// // Step 3: 트랜잭션을 비교해서 표 채우기
// for row := 0; row < k; row++ {
// 	for column := row + 1; column < k; column++ {
// 		if row != column && hasConflict(tEpsilon[row].writeSet, tEpsilon[column].readSet) {
// 			M[row][column] = 1
// 		}
// 	}
// }

// // Step 4: 순환구조 제거
// tEpsilon, tAbort, M = cycleRemoval(M, tEpsilon)

// // Step 5: 트랜잭션 위상정렬로 정렬
// tSerial, _ = transactionSchedule(M, tEpsilon)

// // Step 6: 남은 트랜잭션 배열에 추가
// for i := k; i < len(T); i++ {
// 	if keyVersionInReadSet(T[i].readSet, state) {
// 		tAbort = append(tAbort, T[i])
// 	} else {
// 		tSerial = append(tSerial, T[i])
// 	}
// }

// 	return tAbort, tSerial, state
// }

// 충돌이 있는지 확인
// func hasConflict(index int, writeSet, readSet []int) bool {
// 	if writeSet[index] == readSet[index] {
// 		return true
// 	}
// 	return false
// }

// func hasConflict(writeSet, readSet []int) bool {
// 	for _, writeKey := range writeSet {
// 		for _, readKey := range readSet {
// 			if writeKey == readKey {
// 				return true
// 			}
// 		}
// 	}
// 	return false
// }

// 키 버전 확인
// func keyVersionInReadSet(index int, readSet []int, state int) bool {
// 	if readSet[index] < state {
// 		return true
// 	}
// 	return false
// }

// func keyVersionInReadSet(readSet []int, state []int) bool {
// 	for _, key := range readSet {
// 		for _, stateKey := range state {
// 			// fmt.Println(key, stateKey)
// 			if key < stateKey {
// 				return true
// 			}
// 		}
// 	}
// 	return false
// }

// func cycleRemoval(Mnew [][]int, Tepsilon []Transaction) ([]Transaction, []Transaction, [][]int) {
// 	// Step 1: 초기화
// 	L := generateTransactionPairs(Mnew, Tepsilon)
// 	var D, Dtemp []Transaction
// 	var Tabort []Transaction

// 	for len(L) > 0 {
// 		pair := L[0]
// 		L = L[1:] // Exclude the selected pair from L

// 		// Step 3: 추가하면서 초기화
// 		Tinit := pair[0]
// 		Dtemp = append(Dtemp, Tinit)

// 		// Step 4: 사이클 파악
// 		D = cycle(Tinit, pair[0], pair[1], Mnew)

// 		// Step 5: abort function을 줄이도록
// 		Tstar := argMinAbort(D)

// 		// Step 6: 거절당한 트랜잭션 처리
// 		Tabort = append(Tabort, Tstar)
// 		Tepsilon = removeTransaction(Tepsilon, Tstar)

// 		// Step 7: 삭제
// 		Mnew = deleteRowColumn(Mnew, Tstar.id)
// 	}

// 	return Tepsilon, Tabort, Mnew
// }

// func generateTransactionPairs(M [][]int, Tepsilon []Transaction) [][2]Transaction {
// 	var pairs [][2]Transaction
// 	for i := 0; i < len(M); i++ {
// 		for j := 0; j < len(M[i]); j++ {
// 			if M[i][j] == 1 {
// 				pairs = append(pairs, [2]Transaction{Tepsilon[i], Tepsilon[j]})
// 			}
// 		}
// 	}
// 	return pairs
// }

// func cycle(Tinit, Trow, Tcol Transaction, Mnew [][]int) []Transaction {
// 	var Dtemp []Transaction
// 	Dtemp = append(Dtemp, Tcol) // Add Tcol to Dtemp
// 	Trow = Tcol
// 	fmt.Println(len(Mnew[Trow.id])) // Update Trow as Tcol

// 	byteTrow, _ := json.Marshal(Trow)
// 	byteTinit, _ := json.Marshal(Tinit)
// 	// Step 1: 사이클 체크
// 	if bytes.Equal(byteTrow, byteTinit) {
// 		return Dtemp
// 	}

// 	// Step 2: For each transaction pair in the corresponding row of Mnew with value 1
// 	for col := 0; col < len(Mnew[Trow.id]); col++ {
// 		if Mnew[Trow.id][col] == 1 {
// 			Dtemp = append(Dtemp, cycle(Tinit, Trow, Transaction{id: col}, Mnew)...)
// 		}
// 	}

// 	return Dtemp
// }

// // watchdog에서 유저의 abort transaction에 대해 합의한 내용을 불러와서 최소화가 되도록 함,
// func argMinAbort(D []Transaction) Transaction {
// 	minTransaction := D[0]
// 	for _, t := range D {
// 		if t.id < minTransaction.id {
// 			minTransaction = t
// 		}
// 	}
// 	return minTransaction
// }

// func removeTransaction(Tepsilon []Transaction, T Transaction) []Transaction {
// 	for i, t := range Tepsilon {
// 		if t.id == T.id {
// 			return append(Tepsilon[:i], Tepsilon[i+1:]...)
// 		}
// 	}
// 	return Tepsilon
// }

// func deleteRowColumn(Mnew [][]int, id int) [][]int {
// 	Mnew = append(Mnew[:id], Mnew[id+1:]...)
// 	for i := range Mnew {
// 		Mnew[i] = append(Mnew[i][:id], Mnew[i][id+1:]...)
// 	}
// 	return Mnew
// }

// func transactionSchedule(M [][]int, Tepsilon []Transaction) ([]Transaction, error) {
// 	n := len(M)
// 	inDegree := make([]int, n)
// 	var result []Transaction
// 	queue := []Transaction{}

// 	// Step 1: 각각의 인디그리 계산
// 	for i := 0; i < n; i++ {
// 		for j := 0; j < n; j++ {
// 			if M[i][j] == 1 {
// 				inDegree[j]++
// 			}
// 		}
// 	}

// 	// Step 2: 0인 트랜잭션들 추가
// 	for i := 0; i < n; i++ {
// 		if inDegree[i] == 0 {
// 			queue = append(queue, Tepsilon[i])
// 		}
// 	}

// 	// Step 3: 위상정렬
// 	for len(queue) > 0 {
// 		t := queue[0]
// 		queue = queue[1:]

// 		// 추가
// 		result = append(result, t)

// 		// 업데이트
// 		for col := 0; col < n; col++ {
// 			if M[t.id][col] == 1 {
// 				inDegree[col]--

// 				if inDegree[col] == 0 {
// 					queue = append(queue, Tepsilon[col])
// 				}
// 			}
// 		}
// 	}

// 	// Step 4: 계속 확인
// 	for i := 0; i < n; i++ {
// 		if inDegree[i] > 0 {
// 			return nil, fmt.Errorf("cycle detected, no valid schedule possible")
// 		}
// 	}

// 	return result, nil
// }
