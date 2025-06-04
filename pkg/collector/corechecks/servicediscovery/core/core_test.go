// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compcore "github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type mockTimeProvider struct {
	now time.Time
}

func (m *mockTimeProvider) Now() time.Time {
	return m.now
}

func TestServiceInfoToModelService(t *testing.T) {
	tests := []struct {
		name     string
		info     *ServiceInfo
		pid      int32
		expected *model.Service
	}{
		{
			name:     "nil service info",
			info:     nil,
			pid:      123,
			expected: nil,
		},
		{
			name: "valid service info",
			info: &ServiceInfo{
				Service: model.Service{
					Ports: []uint16{8080, 8081},
					RSS:   1024,
				},
			},
			pid: 123,
			expected: &model.Service{
				PID:   123,
				Ports: []uint16{8080, 8081},
				RSS:   1024,
				Type:  string(servicetype.Detect([]uint16{8080, 8081})),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out model.Service
			result := tt.info.ToModelService(tt.pid, &out)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTagsPriority(t *testing.T) {
	cases := []struct {
		name                string
		tags                []string
		expectedTagName     string
		expectedServiceName string
	}{
		{
			"nil tag list",
			nil,
			"",
			"",
		},
		{
			"empty tag list",
			[]string{},
			"",
			"",
		},
		{
			"no useful tags",
			[]string{"foo:bar"},
			"",
			"",
		},
		{
			"malformed tag",
			[]string{"foobar"},
			"",
			"",
		},
		{
			"service tag",
			[]string{"service:foo"},
			"service",
			"foo",
		},
		{
			"app tag",
			[]string{"app:foo"},
			"app",
			"foo",
		},
		{
			"short_image tag",
			[]string{"short_image:foo"},
			"short_image",
			"foo",
		},
		{
			"kube_container_name tag",
			[]string{"kube_container_name:foo"},
			"kube_container_name",
			"foo",
		},
		{
			"kube_deployment tag",
			[]string{"kube_deployment:foo"},
			"kube_deployment",
			"foo",
		},
		{
			"kube_service tag",
			[]string{"kube_service:foo"},
			"kube_service",
			"foo",
		},
		{
			"multiple tags",
			[]string{
				"foo:bar",
				"baz:biz",
				"service:my_service",
				"malformed",
			},
			"service",
			"my_service",
		},
		{
			"empty value",
			[]string{
				"service:",
				"app:foo",
			},
			"app",
			"foo",
		},
		{
			"multiple tags with priority",
			[]string{
				"foo:bar",
				"short_image:my_image",
				"baz:biz",
				"service:my_service",
				"malformed",
			},
			"service",
			"my_service",
		},
		{
			"all priority tags",
			[]string{
				"kube_service:my_kube_service",
				"kube_deployment:my_kube_deployment",
				"kube_container_name:my_kube_container_name",
				"short_iamge:my_short_image",
				"app:my_app",
				"service:my_service",
			},
			"service",
			"my_service",
		},
		{
			"multiple tags",
			[]string{
				"service:foo",
				"service:bar",
				"other:tag",
			},
			"service",
			"bar",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tagName, name := GetServiceNameFromContainerTags(c.tags)
			require.Equalf(t, c.expectedServiceName, name, "got wrong service name from container tags")
			require.Equalf(t, c.expectedTagName, tagName, "got wrong tag name for service naming")
		})
	}
}

func TestDiscoveryGetServices(t *testing.T) {
	mockTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tp := &mockTimeProvider{now: mockTime}

	mockWMeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		compcore.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	discovery := &Discovery{
		Config: &DiscoveryConfig{
			CPUUsageUpdateDelay: time.Second,
			NetworkStatsPeriod:  time.Second,
		},
		Cache:             make(map[int32]*ServiceInfo),
		PotentialServices: make(PidSet),
		RunningServices:   make(PidSet),
		IgnorePids:        make(PidSet),
		TimeProvider:      tp,
		WMeta:             mockWMeta,
	}

	// Get the current process PID
	pid := int32(os.Getpid())

	// Test getService callback
	getService := func(_ any, pid int32) *model.Service {
		if pid == int32(os.Getpid()) {
			// Create a service with the actual process info
			service := &model.Service{
				PID:   int(pid),
				Ports: []uint16{8080},
			}
			discovery.Cache[pid] = &ServiceInfo{
				Service: *service,
			}
			return service
		}
		return nil
	}

	t.Run("new service", func(t *testing.T) {
		// First call - service should be added to potential services
		resp, err := discovery.GetServices(DefaultParams(), []int32{pid}, nil, getService)
		require.NoError(t, err)
		assert.Empty(t, resp.StartedServices)
		assert.Contains(t, discovery.PotentialServices, pid)

		// Second call - service should be moved to running services
		resp, err = discovery.GetServices(DefaultParams(), []int32{pid}, nil, getService)
		require.NoError(t, err)
		assert.Len(t, resp.StartedServices, 1)
		assert.Contains(t, discovery.RunningServices, pid)
		assert.NotContains(t, discovery.PotentialServices, pid)

		// Verify RSS was read correctly
		require.Greater(t, resp.StartedServices[0].RSS, uint64(0), "RSS should be greater than 0")
	})

	t.Run("service heartbeat", func(t *testing.T) {
		// Move time forward
		tp.now = mockTime.Add(HeartbeatTime)

		resp, err := discovery.GetServices(DefaultParams(), []int32{pid}, nil, getService)
		require.NoError(t, err)
		require.Len(t, resp.HeartbeatServices, 1)
		assert.Equal(t, int64(tp.now.Unix()), resp.HeartbeatServices[0].LastHeartbeat)
		require.Greater(t, resp.HeartbeatServices[0].RSS, uint64(0), "RSS should be greater than 0")
	})

	t.Run("service stopped", func(t *testing.T) {
		// Call with empty pids list - service should be marked as stopped
		resp, err := discovery.GetServices(DefaultParams(), []int32{}, nil, getService)
		require.NoError(t, err)
		assert.Len(t, resp.StoppedServices, 1)
		assert.NotContains(t, discovery.RunningServices, pid)
	})
}

func TestPidSet(t *testing.T) {
	set := make(PidSet)

	t.Run("empty set", func(t *testing.T) {
		assert.False(t, set.Has(123))
	})

	t.Run("add and remove", func(t *testing.T) {
		set.Add(123)
		assert.True(t, set.Has(123))

		set.Remove(123)
		assert.False(t, set.Has(123))
	})
}

func TestDiscoveryCleanup(t *testing.T) {
	discovery := &Discovery{
		Cache:             make(map[int32]*ServiceInfo),
		PotentialServices: make(PidSet),
		RunningServices:   make(PidSet),
		IgnorePids:        make(PidSet),
	}

	// Add some test data
	discovery.Cache[123] = &ServiceInfo{}
	discovery.PotentialServices.Add(123)
	discovery.RunningServices.Add(123)
	discovery.IgnorePids.Add(123)

	t.Run("clean cache", func(t *testing.T) {
		alivePids := make(PidSet)
		alivePids.Add(456) // Different from our test pid

		discovery.cleanCache(alivePids)
		assert.NotContains(t, discovery.Cache, int32(123))
	})

	t.Run("clean pid sets", func(t *testing.T) {
		alivePids := make(PidSet)
		alivePids.Add(456) // Different from our test pid

		discovery.cleanPidSets(alivePids, discovery.PotentialServices, discovery.RunningServices, discovery.IgnorePids)
		assert.NotContains(t, discovery.PotentialServices, int32(123))
		assert.NotContains(t, discovery.RunningServices, int32(123))
		assert.NotContains(t, discovery.IgnorePids, int32(123))
	})
}
