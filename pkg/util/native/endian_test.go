// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package native

import (
	"encoding/binary"
	"runtime"
	"testing"
	"unsafe"
)

func TestNativeEndian(t *testing.T) {
	if rt := getRuntimeEndian(); Endian != rt {
		t.Fatalf("%s: compile time endianness %T != runtime endianness %T", runtime.GOARCH, Endian, rt)
	}
}

func getRuntimeEndian() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}
