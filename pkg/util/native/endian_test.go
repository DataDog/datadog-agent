package native

import (
	"encoding/binary"
	"runtime"
	"testing"
	"unsafe"
)

func TestNativeEndian(t *testing.T) {
	if rt := getRuntimeEndian(); Endian != rt {
		t.Fatalf("%s: runtime endianness %T != compile time endianness %T", runtime.GOARCH, Endian, rt)
	}
}

func getRuntimeEndian() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	} else {
		return binary.BigEndian
	}
}
