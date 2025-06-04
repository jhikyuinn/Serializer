package main

import (
	"bytes"
	cryptorand "crypto/rand"
	"fmt"
	"math/rand"
	mathRand "math/rand"

	ltcode "github.com/MCNL-HGU/mp2btp/fec/lt"
)

func main() {

	lt := ltcode.CreateLTCode(1024, 2048, 0.8, 0.9)

	fmt.Printf(" LT Symbol Size               : %d \n", lt.GetSymSize())
	fmt.Printf(" LT Number of Source Symbols  : %d \n", lt.GetNumSrcSyms())
	fmt.Printf(" LT Number of Encoded Symbols : %d \n", lt.GetNumEncSyms())

	seedNumber := rand.Uint32()
	fmt.Printf(" LT Seed Number               : %d \n", seedNumber)

	// Generate source block
	srcBlock := make([]byte, lt.GetSrcBlockSize())
	_, err := cryptorand.Read(srcBlock)
	if err != nil {
		fmt.Println("Error generating random numbers:", err)
		return
	}
	// lt.PrintBlock("Source Block", srcBlock, int(lt.GetSrcBlockSize()))

	// Create symbol map
	symbolMap := make([]byte, lt.GetEncBlockSize())
	for i := 0; i < int(lt.GetEncBlockSize()); i++ {
		symbolMap[i] = 1
	}

	// LT encode
	encBlock := lt.Encode(srcBlock, seedNumber)
	// lt.PrintBlock("Encoded Block", encBlock, int(lt.GetEncBlockSize()))

	// Loss simulation
	for i := 0; i < int(32); i++ {
		lostSymIdx := mathRand.Intn(int(lt.GetNumEncSyms()))
		startSymIdx := int(lostSymIdx) * int(lt.GetSymSize())
		endSymIdx := startSymIdx + int(lt.GetSymSize()) - 1
		for j := startSymIdx; j <= endSymIdx; j++ {
			encBlock[j] = 0
		}
		symbolMap[lostSymIdx] = 0
		fmt.Printf("Lost symbols: %d (encBlock[%d-%d]) \n", lostSymIdx, startSymIdx, endSymIdx)
	}

	// LT decode
	decBlock := lt.Decode(encBlock, symbolMap, seedNumber)
	// lt.PrintBlock("Decoded Block", decBlock, int(lt.GetSrcBlockSize()))

	// Verification
	areEqual := bytes.Equal(srcBlock, decBlock)
	if areEqual {
		fmt.Printf("Source block and Decoded block are equal! \n ")
	} else {
		fmt.Printf("Error: Source block and Decoded block are not equal! \n")
	}

	// failNum := 0
	// for i := 0; i < int(lt.srcBlockSize); i++ {
	// 	if srcBlock[i] != decBlock[i] {
	// 		failNum++
	// 		fmt.Printf("%d, ", i/int(lt.symSize))
	// 	}
	// }
	// fmt.Printf("\nDecoding failure symbols  :  %d \n", failNum)

}
