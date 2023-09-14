// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package filehandles

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

func TestFhCheckWindows(t *testing.T) {
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.SetQueryReturnValue("\\\\.\\Process(_Total)\\Handle Count", 0.006848775103963421)

	fileHandleCheck := new(fhCheck)
	mock := mocksender.NewMockSender(fileHandleCheck.ID())
	fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	mock.On("Gauge", "system.fs.file_handles.in_use", 0.006848775103963421, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 1)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
