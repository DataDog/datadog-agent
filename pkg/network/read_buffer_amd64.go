
// +build windows

// AMD64 specific implementations for allocating and freeing readbuffers. AMD64
// is the only supported architecture, but the AMD64 implementation will not
// compiled on i386 because the signature for malloc changes based on bus size.
package network

// #include <stdlib.h>
import  "C"

import (
	"unsafe"
)

// AllocateReadBuffer allocates a read buffer with malloc. Buffer's must be freed with FreeReadBuffer.
func AllocateReadBuffer() (*_readbuffer, error) {
	sizeOfReadBuffer := unsafe.Sizeof(_readbuffer{})
	return (*_readbuffer)(C.malloc(C.ulonglong(sizeOfReadBuffer))), nil
}

// FreeReadBuffer frees a read buffer allocated by AllocateReadBuffer
func FreeReadBuffer(buf * _readbuffer) {
	C.free(unsafe.Pointer(buf))
}