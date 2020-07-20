package ebpf

import (
	"encoding/binary"
	"unsafe"
)

// GetHostByteOrder guesses the hosts byte order
func GetHostByteOrder() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	}

	return binary.BigEndian
}

// ByteOrder holds the hosts byte order
var ByteOrder binary.ByteOrder

func init() {
	ByteOrder = GetHostByteOrder()
}
