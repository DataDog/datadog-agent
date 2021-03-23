// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"bufio"
	"fmt"

	"github.com/h2non/filetype"
)

var (
	datadogType = filetype.NewType("dog", "datadog/capture")
	// DATADOG0F1FF0000 in HEX; F1 for different versions, 00 to terminate header
	datadogHeader = []byte{0xD4, 0x74, 0xD0, 0x60, 0xF1, 0xFF, 0x00, 0x00}
)

func init() {
	// Register the new matcher and its type
	filetype.AddMatcher(datadogType, datadogMatcher)
}

func datadogMatcher(buf []byte) bool {
	if len(buf) < len(datadogHeader) {
		return false
	}

	for i := 0; i < len(datadogHeader); i++ {
		if buf[i] != datadogHeader[i] {
			return false
		}
	}

	return true
}

func WriteHeader(w *bufio.Writer) error {

	//Write header
	if n, err := w.Write(datadogHeader); err != nil || n < len(datadogHeader) {
		if err != nil {
			return fmt.Errorf("Capture file header could not be fully written to buffer")
		}
		return err
	}

	return nil
}
