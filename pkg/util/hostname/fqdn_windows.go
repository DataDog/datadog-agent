// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"bytes"
	"math"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func getSystemFQDN() (string, error) {
	hn, err := os.Hostname()
	if err != nil {
		return "", err
	}

	he, err := windows.GetHostByName(hn)
	if err != nil {
		return "", err
	}

	ptr := (*[math.MaxUint16]byte)(unsafe.Pointer(he.Name))
	size := bytes.IndexByte(ptr[:], 0)
	name := make([]byte, size)
	copy(name, ptr[:])

	return string(name), nil
}
