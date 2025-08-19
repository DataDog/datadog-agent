// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build freebsd || darwin

package filehandles

import (
	"testing"

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
