// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptime

import (
	"fmt"
	"testing"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func uptimeSampler() (uint64, error) {
	return 555, nil
}

func TestUptimeCheckLinux(t *testing.T) {
	uptime = uptimeSampler
	defer func() { uptime = host.Uptime }()

	// we have to init the mocked sender here before fileHandleCheck.Configure(...)
	// (and append it to the aggregator, which is automatically done in NewMockSender)
	// because the FinalizeCheckServiceTag is called in Configure.
	// Hopefully, the check ID is an empty string while running unit tests;
	mockSender := mocksender.NewMockSender("")
	mockSender.On("FinalizeCheckServiceTag").Return()

	uptimeCheck := new(Check)
	uptimeCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	// reset the check ID for the sake of correctness
	mocksender.SetSender(mockSender, uptimeCheck.ID())

	mockSender.On("Gauge", "system.uptime", 555.0, "", []string(nil)).Return().Times(1)
	mockSender.On("Commit").Return().Times(1)

	uptimeCheck.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestUptimeCheckErrorPath(t *testing.T) {
	uptime = func() (uint64, error) {
		return 0, fmt.Errorf("uptime unavailable")
	}
	defer func() { uptime = host.Uptime }()

	uptimeCheck := new(Check)
	mock := mocksender.NewMockSender(uptimeCheck.ID())
	mock.On("FinalizeCheckServiceTag").Return()
	uptimeCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	mocksender.SetSender(mock, uptimeCheck.ID())

	err := uptimeCheck.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "uptime unavailable")
	// Gauge should NOT be called when uptime retrieval fails
	mock.AssertNotCalled(t, "Gauge")
	mock.AssertNotCalled(t, "Commit")
}
