// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// fakeHTTPClient is a test HTTP client that can be configured to return specific responses
type fakeHTTPClient struct {
	response *http.Response
	err      error
}

func (c *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.response, nil
}

// createMockServicesResponse creates a mock HTTP response for the services endpoint
func createMockServicesResponse(services []model.Service) *http.Response {
	response := model.ServicesResponse{
		Services: services,
	}
	_, _ = json.Marshal(response)

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       httptest.NewRecorder().Result().Body,
		Header:     make(http.Header),
	}
}

// newTestCollector creates a collector for testing with mock dependencies
func newTestCollector(t *testing.T) (*collector, workloadmetamock.Mock) {
	mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	c := &collector{
		id:             collectorID,
		catalog:        workloadmeta.NodeAgent,
		store:          mockWmeta,
		serviceRetries: make(map[int32]uint),
		ignoredPids:    make(pidSet),
		sysProbeClient: &http.Client{},
		startTime:      time.Now(),
		startupTimeout: 30 * time.Second,
	}
	return c, mockWmeta
}

func TestGetURL(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		pids     []int32
		expected string
	}{
		{
			name:     "endpoint without pids",
			endpoint: "services",
			pids:     []int32{},
			expected: "http://sysprobe/discovery/services",
		},
		{
			name:     "endpoint with pids",
			endpoint: "language",
			pids:     []int32{1, 2, 3},
			expected: "http://sysprobe/discovery/language?pids=1%2C2%2C3",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url := getDiscoveryURL(c.endpoint, c.pids)
			require.Equal(t, c.expected, url)
		})
	}
}

func TestCleanPidMap(t *testing.T) {
	t.Run("removes dead pids from maps", func(t *testing.T) {
		alivePids := make(pidSet)
		alivePids.add(123)
		alivePids.add(456)

		retries := map[int32]uint{
			123: 1,
			456: 2,
			789: 3, // dead pid
		}

		ignored := make(pidSet)
		ignored.add(123)
		ignored.add(999) // dead pid

		cleanPidMap(alivePids, retries)
		cleanPidMap(alivePids, ignored)

		// Check that dead pids were removed
		require.Equal(t, uint(1), retries[123])
		require.Equal(t, uint(2), retries[456])
		require.NotContains(t, retries, int32(789))

		require.True(t, ignored.has(123))
		require.False(t, ignored.has(999))
	})
}

func TestGetPidsToRequest(t *testing.T) {
	c, _ := newTestCollector(t)

	// Create a set of alive PIDs
	alivePids := make(pidSet)
	alivePids.add(123)
	alivePids.add(456)
	alivePids.add(789)

	// Add ignored  PID (simulating a PID that exceeded max retry attempts)
	c.ignoredPids.add(456)

	pids, pidsToService := c.getPidsToRequest(alivePids)

	// Only non-ignored PIDs should be returned for querying
	require.Len(t, pids, 2)
	require.Contains(t, pids, int32(123))
	require.Contains(t, pids, int32(789))
	require.NotContains(t, pids, int32(456))

	// The pidsToService map should have entries for all requested PIDs
	// initially set to nil (will be populated later with service data)
	require.Len(t, pidsToService, 2)
	require.Contains(t, pidsToService, int32(123))
	require.Contains(t, pidsToService, int32(789))

	// Initially nil, will be filled by service discovery
	require.Nil(t, pidsToService[123])
	require.Nil(t, pidsToService[789])
}
