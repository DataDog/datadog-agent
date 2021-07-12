// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package util

import (
	"C"
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
	namestring := C.GoString((*C.char)(unsafe.Pointer(he.Name)))

	return namestring, nil
}
