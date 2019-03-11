// +build windows

package common

// #include <stdio.h>
// #include <stdlib.h>
import "C"
import (
	"sync"
)

var lockStdFileDescriptorsSwapping sync.Mutex

// Capture code from https://github.com/zimmski/osutil
func Capture(call func()) (output []byte, err error) {
	call()
	return nil, nil

}
