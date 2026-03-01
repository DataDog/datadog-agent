// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build freebsd || darwin

package filehandles

import (
	"fmt"
	"testing"

	"github.com/blabber/go-freebsd-sysctl/sysctl"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func GetInt64(_ string) (value int64, err error) {
	value = 65534
	err = nil
	return
}

func TestFhCheckFreeBSD(t *testing.T) {
	getInt64 = GetInt64

	// we have to init the mocked sender here before fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, ...)
	// (and append it to the aggregator, which is automatically done in NewMockSender)
	// because the FinalizeCheckServiceTag is called in Configure.
	// Hopefully, the check ID is an empty string while running unit tests;
	mock := mocksender.NewMockSender("")

	fileHandleCheck := new(fhCheck)
	fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	// reset the check ID for the sake of correctness
	mocksender.SetSender(mock, fileHandleCheck.ID())

	mock.On("Gauge", "system.fs.file_handles.used", float64(65534), "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.max", float64(65534), "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestFhCheckOpenFilesError(t *testing.T) {
	callCount := 0
	getInt64 = func(oid string) (int64, error) {
		callCount++
		if oid == openfilesOID {
			return 0, fmt.Errorf("sysctl error")
		}
		return 100, nil
	}
	defer func() { getInt64 = sysctl.GetInt64 }()

	fileHandleCheck := new(fhCheck)
	mock := mocksender.NewMockSender(fileHandleCheck.ID())
	fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	mocksender.SetSender(mock, fileHandleCheck.ID())

	err := fileHandleCheck.Run()
	assert.Error(t, err)
	mock.AssertNotCalled(t, "Gauge")
	mock.AssertNotCalled(t, "Commit")
}

func TestFhCheckMaxFilesError(t *testing.T) {
	getInt64 = func(oid string) (int64, error) {
		if oid == "kern.maxfiles" {
			return 0, fmt.Errorf("sysctl error")
		}
		return 65534, nil
	}
	defer func() { getInt64 = sysctl.GetInt64 }()

	fileHandleCheck := new(fhCheck)
	mock := mocksender.NewMockSender(fileHandleCheck.ID())
	fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	mocksender.SetSender(mock, fileHandleCheck.ID())

	err := fileHandleCheck.Run()
	assert.Error(t, err)
	mock.AssertNotCalled(t, "Commit")
}
