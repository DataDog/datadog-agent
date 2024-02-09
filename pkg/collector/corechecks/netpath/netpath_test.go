// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netpath

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestCPUCheckLinux(t *testing.T) {
	cpuCheck := new(Check)
	cpuCheck.Configure(integration.FakeConfigHash, nil, nil, "test")

	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "netpath.test_metric", float64(10), "", []string(nil)).Return().Times(1)

	m.On("Commit").Return().Times(1)

	cpuCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.GaugeType.String(), 1)
}
