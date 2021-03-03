// +build windows

package network

// This file contains AMD64 specific implementations for allocating and freeing readbuffers. AMD64
// is the only supported architecture, but the AMD64 implementation will not
// compiled on i386 because the signature for malloc changes based on bus size.

// #include <stdlib.h>
import "C"

import (
	"unsafe"
)

func allocateReadBuffer() (*readbuffer, error) {
	sizeOfReadBuffer := unsafe.Sizeof(readbuffer{})
	return (*readbuffer)(C.malloc(C.ulonglong(sizeOfReadBuffer))), nil
}

func freeReadBuffer(buf *readbuffer) {
	C.free(unsafe.Pointer(buf))
}
