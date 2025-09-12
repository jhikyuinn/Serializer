package lt

/*
#cgo LDFLAGS: -lm
#include <lt.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type LT struct {
	symSize        uint32
	numSrcSyms     uint32
	numEncSyms     uint32
	numSymsForDec  uint32
	srcBlockSize   uint32
	encBlockSize   uint32
	codeRate       float32
	codeRateForDec float32
}

func CreateLTCode(symSize uint32, numSrcSyms uint32, codeRate float32, codeRateForDec float32) *LT {
	lt := LT{
		symSize:        symSize,
		numSrcSyms:     numSrcSyms,
		codeRate:       codeRate,
		codeRateForDec: codeRateForDec, // TODO: to be determined as appropriated value
	}
	lt.numEncSyms = uint32(float32(lt.numSrcSyms) / lt.codeRate)
	lt.numSymsForDec = uint32(float32(lt.numSrcSyms) / lt.codeRateForDec)
	lt.srcBlockSize = lt.symSize * lt.numSrcSyms
	lt.encBlockSize = lt.symSize * lt.numEncSyms

	return &lt
}

func (lt *LT) GetSymSize() uint32 {
	return lt.symSize
}

func (lt *LT) GetNumSrcSyms() uint32 {
	return lt.numSrcSyms
}

func (lt *LT) GetNumEncSyms() uint32 {
	return lt.numEncSyms
}

func (lt *LT) GetNumSymsForDec() uint32 {
	return lt.numSymsForDec
}

func (lt *LT) GetSrcBlockSize() uint32 {
	return lt.srcBlockSize
}

func (lt *LT) GetEncBlockSize() uint32 {
	return lt.encBlockSize
}

func (lt *LT) Encode(data []byte, seedNumber uint32) []byte {
	srcData := (*C.char)(unsafe.Pointer(&data[0]))

	// LT encoding - it reurns 1-D redundant data array only
	encBlock := C.lt_encode((*C.char)(srcData), C.int(lt.numSrcSyms), C.int(lt.numEncSyms), C.int(lt.symSize), C.int(seedNumber))

	if encBlock == nil {
		fmt.Printf("LT.LTEncode(): LT encoding failed!!! symSize=%d, numSrcSyms=%d, , numEncSyms=%d \n", lt.symSize, lt.numSrcSyms, lt.numEncSyms)
		return nil
	}

	return C.GoBytes(unsafe.Pointer(encBlock), C.int(lt.symSize*lt.numEncSyms)) // 4, 32
}

func (lt *LT) Decode(data []byte, symbolMap []byte, seedNumber uint32) []byte {
	encBlock := (*C.char)(unsafe.Pointer(&data[0]))
	nonError := (*C.char)(unsafe.Pointer(&symbolMap[0]))

	// LT decoding
	decBlock := C.lt_decode((*C.char)(encBlock), C.int(lt.numEncSyms), C.int(lt.numSrcSyms), C.int(lt.symSize), (*C.char)(nonError), C.int(seedNumber))

	if decBlock == nil {
		fmt.Printf("LT.LTDecode(): LT decoding failed!!! symSize=%d, numSrcSyms=%d, , numEncSyms=%d \n", lt.symSize, lt.numSrcSyms, lt.numEncSyms)
		return nil
	}

	return C.GoBytes(unsafe.Pointer(decBlock), C.int(lt.symSize*lt.numSrcSyms))
}

func (lt *LT) PrintBlock(blockName string, data []byte, len int) {
	fmt.Printf("%s: \n", blockName)
	fmt.Printf("%05d: ", 0)
	for i := 0; i < len; i++ {
		fmt.Printf("%02x, ", 0xff&data[i])
		if (i+1)%int(lt.symSize) == 0 {
			fmt.Printf("\n%05d: ", (i+1)/32)
		}
	}
	fmt.Printf("\n")
}
