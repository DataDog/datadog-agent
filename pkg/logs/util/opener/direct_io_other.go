// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package opener

import (
	"errors"
	"os"
)

var errDirectIOUnsupported = errors.New("O_DIRECT is only supported on Linux")

func openDirect(string) (*os.File, error) {
	return nil, errDirectIOUnsupported
}

func directIOAlignments(*os.File) (int, int, error) {
	return directIOAlignment, directIOAlignment, nil
}

// allocDirectIOBuffer may use the heap: openDirect never succeeds off Linux.
func allocDirectIOBuffer(size int) ([]byte, error) {
	return make([]byte, size), nil
}

func freeDirectIOBuffer([]byte) {}
