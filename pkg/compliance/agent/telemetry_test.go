// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func createRandomContainers(store *workloadmeta.MockStore, n int) {
	for i := 0; i < n; i++ {
		store.SetEntity(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   strconv.FormatInt(int64(i), 10),
			},
			State: workloadmeta.ContainerState{Running: true},
		})
	}
}

func TestReportContainersCount(t *testing.T) {
	mockSender := mocksender.NewMockSender("foo")
	mockSender.SetupAcceptAll()

	fakeStore := workloadmeta.NewMockStore()
	telemetry := &telemetry{
		containers: &common.ContainersTelemetry{
			Sender:        mockSender,
			MetadataStore: fakeStore,
		},
	}

	runningContainersCount := 10
	createRandomContainers(fakeStore, runningContainersCount)

	// Create a non-running container. It should not appear in the result
	fakeStore.SetEntity(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   strconv.FormatInt(int64(runningContainersCount), 10),
		},
		State: workloadmeta.ContainerState{Running: false},
	})

	telemetry.reportContainers()
	mockSender.AssertNumberOfCalls(t, "Gauge", runningContainersCount)
	for i := 0; i < runningContainersCount; i++ {
		mockSender.AssertCalled(t, "Gauge", containersCountMetricName, 1.0, "", []string{"container_id:" + strconv.Itoa(i)})
	}
}
