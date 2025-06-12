// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package wlm

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
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
	self := int32(os.Getpid())

	// Create test processes with service information
	processes := []struct {
		pid         int32
		containerID string
		service     *workloadmeta.Service
	}{
		{
			pid:         self,
			containerID: "container1",
			service: &workloadmeta.Service{
				GeneratedName:       "web-service",
				GeneratedNameSource: "process_name",
				DDService:           "web",
				ContainerID:         "container1",
				ContainerTags:       []string{"service:web", "env:prod", "app:nginx"},
				Ports:               []uint16{80, 443},
				Language:            "go",
				Type:                "web_service",
			},
		},
		{
			pid:         1002,
			containerID: "container2",
			service: &workloadmeta.Service{
				GeneratedName:       "db-service",
				GeneratedNameSource: "process_name",
				DDService:           "postgres",
				ContainerID:         "container2",
				ContainerTags:       []string{"service:db", "env:prod", "app:postgres"},
				Ports:               []uint16{5432},
				Language:            "c",
				Type:                "db",
			},
		},
	}

	// Add processes to workloadmeta
	for _, p := range processes {
		process := &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(p.pid)),
			},
			Pid:         p.pid,
			ContainerID: p.containerID,
			Service:     p.service,
		}
		mockWmeta.Set(process)
	}

	// Create DiscoveryWLM instance
	discovery, err := NewDiscoveryWLM(mockWmeta, mockTagger)
	require.NoError(t, err)

	// Get services - call twice to test the caching behavior
	_, err = discovery.DiscoverServices()
	require.NoError(t, err)
	resp, err := discovery.DiscoverServices()
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify results
	require.Len(t, resp.StartedServices, len(processes))

	// Check each process's service info
	for _, p := range processes {
		var found *model.Service
		for i := range resp.StartedServices {
			if resp.StartedServices[i].PID == int(p.pid) {
				found = &resp.StartedServices[i]
				break
			}
		}
		require.NotNil(t, found, "Service not found for process %d", p.pid)
		assert.Equal(t, int(p.pid), found.PID)
		assert.Equal(t, p.service.GeneratedName, found.GeneratedName)
		assert.Equal(t, p.service.DDService, found.DDService)
		assert.Equal(t, p.service.ContainerID, found.ContainerID)
		assert.ElementsMatch(t, p.service.ContainerTags, found.ContainerTags)
		assert.ElementsMatch(t, p.service.Ports, found.Ports)
		assert.Equal(t, p.service.Language, found.Language)

		if p.pid == self {
			assert.NotEqual(t, uint64(0), found.RSS)
		}
	}
}
