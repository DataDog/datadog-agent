// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package winproc

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

func TestWinprocCheckWindows(t *testing.T) {
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.SetQueryReturnValue("\\\\.\\System\\Processor Queue Length", 2.0)
	pdhtest.SetQueryReturnValue("\\\\.\\System\\Processes", 32.0)

	winprocCheck := new(processChk)
	mock := mocksender.NewMockSender(winprocCheck.ID())
	winprocCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	mock.On("Gauge", "system.proc.queue_length", 2.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.proc.count", 32.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	winprocCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
