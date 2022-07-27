// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package network

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"golang.org/x/sys/windows"
)

type TestDriverHandleFail struct {
	// store some state variables
	hasBeenCalled   bool
	lastReturnBytes uint32
	lastBufferSize  int
	lastError       error
}

func (tdh *TestDriverHandleFail) ReadFile(p []byte, bytesRead *uint32, ol *windows.Overlapped) error {
	fmt.Printf("Got ReadFile call")
	// check state in struct to see if we've been called before
	if tdh.hasBeenCalled {
		if tdh.lastReturnBytes == 0 && tdh.lastError == windows.ERROR_MORE_DATA {
			// last time we returned empty but more...if caller does that twice in a row it's bad
			if len(p) <= tdh.lastBufferSize {
				panic(fmt.Errorf("Consecutive calls"))
			}
		}
	}
	return nil
}

func (tdh *TestDriverHandleFail) GetWindowsHandle() windows.Handle {
	return windows.Handle(0)
}

func (tdh *TestDriverHandleFail) DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error) {
	fmt.Printf("Got test ioctl call")
	if ioControlCode != 0 {
		return fmt.Errorf("wrong ioctl code")
	}
	return nil
}

func (tdh *TestDriverHandleFail) CancelIoEx(ol *windows.Overlapped) error {
	return nil
}

func (tdh *TestDriverHandleFail) GetStatsForHandle() (map[string]map[string]int64, error) {
	return nil, nil
}
func (tdh *TestDriverHandleFail) Close() error {
	return nil
}

func NewFailHandle(flags uint32, handleType driver.HandleType) (driver.Handle, error) {
	return &TestDriverHandleFail{}, nil
}

func TestSetFlowFiltersFail(t *testing.T) {
	return
}
