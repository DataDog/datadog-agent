// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/stretchr/testify/assert"
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
		sender:        mockSender,
		metadataStore: fakeStore,
	}

	containersCount := 10
	createRandomContainers(fakeStore, containersCount)

	assert.NoError(t, telemetry.reportContainers())
	mockSender.AssertNumberOfCalls(t, "Gauge", containersCount)
	for i := 0; i < containersCount; i++ {
		mockSender.AssertCalled(t, "Gauge", containersCountMetricName, 1.0, "", []string{"container_id:" + strconv.Itoa(i)})
	}
}
