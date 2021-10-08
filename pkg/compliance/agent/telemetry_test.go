// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/collectors"

	"github.com/stretchr/testify/assert"
)

type dummyCollector struct{ ctrCount int }

func (dc *dummyCollector) Detect() error                                 { return nil }
func (dc *dummyCollector) UpdateMetrics(c []*containers.Container) error { return nil }
func (dc *dummyCollector) List() ([]*containers.Container, error) {
	ctrList := []*containers.Container{}
	for i := 0; i < dc.ctrCount; i++ {
		ctrList = append(ctrList, &containers.Container{ID: strconv.Itoa(i)})
	}
	return ctrList, nil
}

type dummyDetector struct{ ctrCount int }

func (dd *dummyDetector) GetPreferred() (collectors.Collector, string, error) {
	return &dummyCollector{ctrCount: dd.ctrCount}, "dummy", nil
}

func newDummyDetector(ctrCount int) collectors.DetectorInterface {
	return &dummyDetector{ctrCount: ctrCount}
}

func TestReportContainersCount(t *testing.T) {
	mockSender := mocksender.NewMockSender("foo")
	mockSender.SetupAcceptAll()

	telemetry := &telemetry{sender: mockSender}

	containersCount := 10
	telemetry.detector = newDummyDetector(containersCount)
	assert.NoError(t, telemetry.reportContainers())
	mockSender.AssertNumberOfCalls(t, "Gauge", containersCount)
	for i := 0; i < containersCount; i++ {
		mockSender.AssertCalled(t, "Gauge", containersCountMetricName, 1.0, "", []string{"container_id:" + strconv.Itoa(i)})
	}
}
