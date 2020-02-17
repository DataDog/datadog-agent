// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package system

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func uptimeSampler() (uint64, error) {
	return 555, nil
}

func TestUptimeCheckLinux(t *testing.T) {
	// we have to init the mocked sender here before fileHandleCheck.Configure(...)
	// (and append it to the aggregator, which is automatically done in NewMockSender)
	// because the FinalizeCheckServiceTag is called in Configure.
	// Hopefully, the check ID is an empty string while running unit tests;
	mock := mocksender.NewMockSender("")
	mock.On("FinalizeCheckServiceTag").Return()

	uptime = uptimeSampler
	uptimeCheck := new(UptimeCheck)
	uptimeCheck.Configure(nil, nil, "test")

	// reset the check ID for the sake of correctness
	mocksender.SetSender(mock, uptimeCheck.ID())

	mock.On("Gauge", "system.uptime", 555.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	uptimeCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 1)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
