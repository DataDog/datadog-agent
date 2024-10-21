// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package generic

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
)

// MockContainerAccessor is a dummy ContainerLister for tests
type MockContainerAccessor struct {
	containers []*workloadmeta.Container
}

// ListRunning returns the mocked containers
func (l *MockContainerAccessor) ListRunning() []*workloadmeta.Container {
	return l.containers
}

// CreateTestProcessor returns a ready-to-use Processor
func CreateTestProcessor(listerContainers []*workloadmeta.Container,
	metricsContainers map[string]mock.ContainerEntry,
	metricsAdapter MetricsAdapter,
	containerFilter ContainerFilter,
	tagger tagger.Component,
) (*mocksender.MockSender, *Processor, ContainerAccessor) {
	mockProvider := mock.NewMetricsProvider()
	mockCollector := mock.NewCollector("testCollector")
	for _, runtime := range provider.AllLinuxRuntimes {
		mockProvider.RegisterConcreteCollector(provider.NewRuntimeMetadata(string(runtime), ""), mockCollector)
	}
	for cID, entry := range metricsContainers {
		mockCollector.SetContainerEntry(cID, entry)
	}

	mockAccessor := MockContainerAccessor{
		containers: listerContainers,
	}

	mockedSender := mocksender.NewMockSender("generic-container")
	mockedSender.SetupAcceptAll()

	p := NewProcessor(mockProvider, &mockAccessor, metricsAdapter, containerFilter, tagger)

	return mockedSender, &p, &mockAccessor
}

// MockSendMetric is a dummy function that can be used as a senderFunc
func MockSendMetric(senderFunc func(string, float64, string, []string), metricName string, value *float64, tags []string) {
	if value != nil {
		senderFunc(metricName, *value, "", tags)
	}
}

// CreateContainerMeta returns a dummy workloadmeta.Container
func CreateContainerMeta(runtime, cID string) *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   cID,
		},
		Runtime: workloadmeta.ContainerRuntime(runtime),
		State: workloadmeta.ContainerState{
			Running: true,
			// Put the creation date in the past as, on Windows, the timer resolution may generate a 0 elapsed.
			StartedAt: time.Now().Add(-2 * time.Second),
		},
	}
}
