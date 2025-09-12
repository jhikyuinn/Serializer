package gofountain

import (
	"errors"
	"math/rand"
	"os"
	"strings"
	"time"
)

// Encode encodes the source message using number of source symbols, symbol alignment size, and number of encoded source symbols.
// The output is a slice of encoded source symbols that was generated
func Encode(message []byte, symbols int, alignment int, encBlocks int) []LTBlock {
	c := NewRaptorCodec(symbols, alignment)
	ids := make([]int64, encBlocks)

	seedMode := os.Getenv("TIME_VARIED_SEED")

	var random *rand.Rand
	if strings.Contains(seedMode, "true") {
		seed := time.Now().UnixNano() / 1e6
		random = rand.New(rand.NewSource(seed))
	} else {
		random = rand.New(rand.NewSource(8923489))
	}

	for i := range ids {
		ids[i] = int64(random.Intn(60000))
	}

	messageCopy := make([]byte, len(message))
	copy(messageCopy, message)

	encodedBlocks := EncodeLTBlocks(messageCopy, ids, c)

	return encodedBlocks
}

// Decode receives enough encoded source symbols to decode the original message(source message)
// The output is a original message and index of the encoded source symbol that have been decoded successfully.
func Decode(encodedSymbols []LTBlock, symbols int, alignment int, encBlock int, messageSize int, hash [32]byte) ([]byte, int, error) {
	c := NewRaptorCodec(symbols, alignment)
	decoder := newRaptorDecoder(c.(*raptorCodec), messageSize)

	for i, symbol := range encodedSymbols {
		decoder.AddBlocks([]LTBlock{symbol})
		if decoder.matrix.determined() {
			out := decoder.Decode()
			return out, i + 1, nil
		}
	}

	return nil, -1, errors.New("not decoded")
}
