// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build windows

package api

import (
	"net"

	"github.com/Microsoft/go-winio"
)

func listenPipe(path string, secdec string, bufferSize int) (net.Listener, error) {
	return winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: secdec,
		InputBufferSize:    int32(bufferSize),
	})
}
