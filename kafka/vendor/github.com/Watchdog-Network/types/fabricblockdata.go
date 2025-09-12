package types

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
