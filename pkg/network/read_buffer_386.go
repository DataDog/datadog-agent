
// +build windows

package network

import (
	"fmt"
)

// AllocateReadBuffer is not supported on i386.
func AllocateReadBuffer() (*_readbuffer, error) {
	return nil, fmt.Errorf("unsupported on 386")
}

// FreeReadBuffer is not supported on i386.
func FreeReadBuffer(buf * _readbuffer) {
	// this code is unreachable because AllocateReadBuffer will not succeed on 386
}