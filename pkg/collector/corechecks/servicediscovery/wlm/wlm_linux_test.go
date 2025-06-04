// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package wlm

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDiscoveryWLM(t *testing.T) {
	// Setup mocks
	mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	self := os.Getpid()

	// Create test containers
	containers := []struct {
		id     string
		pid    int
		tags   []string
		labels []string
	}{
		{
			id:     "container1",
			pid:    self,
			tags:   []string{"service:web", "env:prod"},
			labels: []string{"app:nginx"},
		},
		{
			id:     "container2",
			pid:    1002,
			tags:   []string{"service:db", "env:prod"},
			labels: []string{"app:postgres"},
		},
	}

	// Add containers to workloadmeta
	for _, c := range containers {
		container := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   c.id,
			},
			PID:           c.pid,
			CollectorTags: c.labels,
			State:         workloadmeta.ContainerState{Running: true},
		}
		mockWmeta.Set(container)

		// Set tagger tags
		mockTagger.SetTags(types.NewEntityID(types.ContainerID, c.id), "fake", nil, nil, c.tags, nil)
	}

	// Create DiscoveryWLM instance
	discovery, err := NewDiscoveryWLM(mockWmeta, mockTagger)
	require.NoError(t, err)

	// Get services
	_, err = discovery.DiscoverServices()
	require.NoError(t, err)
	resp, err := discovery.DiscoverServices()
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify results
	require.Len(t, resp.StartedServices, len(containers))

	// Check each container's service info
	for _, c := range containers {
		var found *model.Service
		for i := range resp.StartedServices {
			if resp.StartedServices[i].PID == c.pid {
				found = &resp.StartedServices[i]
				break
			}
		}
		require.NotNil(t, found, "Service not found for container %s", c.id)
		assert.Equal(t, c.pid, found.PID)
		assert.Equal(t, c.id, found.ContainerID)
		assert.ElementsMatch(t, append(c.tags, c.labels...), found.ContainerTags)

		if c.pid == self {
			assert.NotEqual(t, 0, found.RSS)
		}
	}
}
