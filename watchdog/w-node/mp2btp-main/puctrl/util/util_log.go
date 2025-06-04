package util

import (
	"fmt"
	"time"
)

var flag bool

// FIXME: How to set verbose mode var? (range problem)
func Log(format string, args ...interface{}) {
	if flag {
		pre := "[" + time.Now().Format(time.StampMicro) + "] "
		fmt.Printf(pre+format+"\n", args...)
	}
}

func SetFlag(verbose_mode bool) {
	flag = verbose_mode
}
