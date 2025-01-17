package main

import (
	pb "auditchain/msg"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/golang/protobuf/proto"
	"github.com/herumi/bls-eth-go-binary/bls"
)

func main() {
	a := &pb.AuditMsg{BlkNum: 1}

	var sBSPs []*pb.StandbyBSP = make([]*pb.StandbyBSP, 5)
	sBSPs[0] = &pb.StandbyBSP{Id: 0}
	sBSPs[1] = &pb.StandbyBSP{Id: 1}
	sBSPs[2] = &pb.StandbyBSP{Id: 2}
	sBSPs[3] = &pb.StandbyBSP{Id: 3}
	sBSPs[4] = &pb.StandbyBSP{Id: 4}
	for i := 0; i < 5; i++ {
		sBSPs[4] = &pb.StandbyBSP{Id: uint32(rand.Intn(1000))}
	}

	var hAuditors []*pb.HonestAuditor = make([]*pb.HonestAuditor, 100)
	var temp uint32
	for i := 0; i < 100; i++ {
		temp = uint32(rand.Intn(1000))
		hAuditors[i] = &pb.HonestAuditor{Id: temp}
	}

	b := &pb.AuditMsg{
		BlkNum:         101,
		PrevHash:       "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		CurHash:        "61b4c705859f4158d38090c1e38e8fdc4f3d29db007f012766276aa498835cf6",
		AuditorID:      12,
		BSP:            212,
		StandbyBSPs:    sBSPs,
		Signature:      "cf37d3f31d7f701b04c013721cd45f46c6ebd6db6e8245d2a42be7c3575d933e",
		HonestAuditors: hAuditors,
		PhaseNum:       pb.AuditMsg_PREPARE,
	}

	fmt.Println("inlab package BYE function", a)
	fmt.Println("inlab package BYE function", b.PhaseNum)

	// PROTO 인코딩
	protoBytes, err := proto.Marshal(b)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(protoBytes))

	// JSON 인코딩
	jsonBytes, err := json.Marshal(b)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(jsonBytes))
	gg := bytes.NewBuffer(jsonBytes).String()
	fmt.Println(gg)

	bls.Init(bls.BLS12_381)
	bls.SetETHmode(bls.EthModeDraft07)
	var sec1 bls.SecretKey
	sec1.SetByCSPRNG()
	pub1 := sec1.GetPublicKey()
	fmt.Printf("t6: %T\n", pub1)
	fmt.Println(pub1)
	aaa, err := json.Marshal(pub1)
	fmt.Println(len(aaa))
	json.Unmarshal(aaa, pub1)
	fmt.Println(pub1)
}
