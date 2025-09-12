package gofountain

import (
	"encoding/json"
)

// A HLFEncodedBlock represents Hyperledger fabric block that store transaction
// information in the ledger.
type HLFEncodedBlock struct {
	BlockHeader   BlockHeader
	BlockData     BlockData
	BlockMetadata BlockMetadata
	// How many padding bytes this block has at the end.
	Padding []int
}

// The BlockHeader consist of block number, copy of the previous block hash and
// the current block hash.
type BlockHeader struct {
	Number       uint64
	PreviousHash []byte
	DataHash     []byte
}

// The BlockData fields of a block are the essentials segment that contents the
// transaction details sorted in the byte array.
type BlockData struct {
	Data [][]byte
}

// The BlockMetadata fields contain the created time of the block, certificate
// details and signature of the block writer.
type BlockMetadata struct {
	Metadata [][]byte
}

// EqualizeParsedBlockLentghs adds padding to parsed blocks to make them devided by number of source symbols.
// Returns a slice of serialized parsedblock with padding.
func EqualizeParsedBlockLengths(numSourceSymbols int, symbolAlignmentSize int, numEncodedSourceSymbols int, parsedBlock HLFEncodedBlock) []byte {

	marshalledBlock, _ := json.Marshal(parsedBlock)

	var padding []int
	if len(marshalledBlock)%numSourceSymbols != 0 {
		if ((numSourceSymbols-(len(marshalledBlock)%numSourceSymbols))/2)%2 == 0 {
			padding = make([]int, ((numSourceSymbols-(len(marshalledBlock)%numSourceSymbols))/2)+2)
		} else {
			padding = make([]int, ((numSourceSymbols-(len(marshalledBlock)%numSourceSymbols))/2)+1)
		}
	}

	parsedBlock.Padding = padding
	marshalledBlock, _ = json.Marshal(parsedBlock)

	return marshalledBlock
}
