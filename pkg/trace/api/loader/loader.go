// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package loader contains initialization logic shared with the trace-loader process
package loader

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// GetListenerFromFD creates a new net.Listener from a file descriptor
//
// Under the hood the file descriptor will be dupped to be used by the Go runtime
// The file descriptor from the string will be closed
func GetListenerFromFD(fdStr string, name string) (net.Listener, error) {
	fd, err := strconv.Atoi(fdStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse file descriptor %v: %v", fdStr, err)
	}

	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		return nil, fmt.Errorf("invalid file descriptor %v", fdStr)
	}

	defer f.Close()

	listener, flerr := net.FileListener(f)
	if flerr != nil {
		return nil, fmt.Errorf("could not create file listener for %v: %v", fdStr, flerr)
	}
	return listener, nil
}
