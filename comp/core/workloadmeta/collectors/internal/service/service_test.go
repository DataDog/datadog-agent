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
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// startTestServer creates a system-probe test server that returns the specified response or error
func startTestServer(t *testing.T, response *model.ServicesResponse, shouldError bool) (string, *httptest.Server) {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discovery/services" {
			if shouldError {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			responseBytes, _ := json.Marshal(response)
			w.Write(responseBytes)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	socketPath := testutil.SystemProbeSocketPath(t, "service-collector")
	server, err := testutil.NewSystemProbeTestServer(handler, socketPath)
	require.NoError(t, err)
	require.NotNil(t, server)
	server.Start()
	t.Cleanup(server.Close)

	return socketPath, server
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

const (
	// Test PIDs with descriptive names
	pidNewService     = 123
	pidFreshService   = 456
	pidStaleService   = 789
	pidDeadService    = 999
	pidIgnoredService = 555

	// Base timestamp for consistent testing - January 1, 2024, 12:00:00 UTC
	baseTimestamp = 1704110400
)

var (
	// Reference time for testing
	baseTime = time.Unix(baseTimestamp, 0)
	// 15 minute heartbeat threshold
	heartbeatThreshold = 15 * time.Minute
)

// Service creation helpers for testing
func createWorkloadmetaService(pid int32, name string, heartbeatOffset time.Duration) *workloadmeta.Service {
	return &workloadmeta.Service{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindService,
			ID:   strconv.Itoa(int(pid)),
		},
		Name:                     name,
		GeneratedName:            name + "-generated",
		GeneratedNameSource:      "cmdline",
		AdditionalGeneratedNames: []string{name + "-alt1", name + "-alt2"},
		DDService:                "dd-" + name,
		DDServiceInjected:        true,
		Ports:                    []uint16{8080, 9090},
		APMInstrumentation:       "automatic",
		Language:                 "go",
		Type:                     "web_service",
		CommandLine:              []string{"/usr/bin/" + name, "--port=8080"},
		StartTimeMilli:           uint64(baseTime.Add(-1 * time.Hour).UnixMilli()),
		LastHeartbeat:            baseTime.Add(heartbeatOffset).Unix(),
	}
}

func createModelService(pid int32, name string) model.Service {
	return model.Service{
		PID:                      int(pid),
		GeneratedName:            name + "-model",
		GeneratedNameSource:      "process",
		AdditionalGeneratedNames: []string{name + "-model-alt"},
		TracerMetadata: []tracermetadata.TracerMetadata{
			{
				TracerLanguage: "python",
				TracerVersion:  "1.0.0",
				ServiceName:    name + "-service",
			},
		},
		DDService:          "dd-model-" + name,
		DDServiceInjected:  false,
		Ports:              []uint16{3000, 4000},
		APMInstrumentation: "manual",
		Language:           "python",
		Type:               "database",
		CommandLine:        []string{"/opt/" + name, "--config=/etc/config"},
		StartTimeMilli:     uint64(baseTime.Add(-30 * time.Minute).UnixMilli()),
		LastHeartbeat:      baseTime.Unix(),
	}
}

// Test verification helper functions
func verifyStoredServices(t *testing.T, mockWmeta workloadmetamock.Mock, expectedPids []int32) {
	allServices := mockWmeta.ListServices()
	storedPids := make([]int32, len(allServices))
	for i, service := range allServices {
		pid, _ := strconv.Atoi(service.GetID().ID)
		storedPids[i] = int32(pid)
	}
	require.ElementsMatch(t, expectedPids, storedPids, "unexpected services in store")
}

func verifyRemovedServices(t *testing.T, mockWmeta workloadmetamock.Mock, removedPids []int32) {
	for _, removedPid := range removedPids {
		service, err := mockWmeta.GetService(strconv.Itoa(int(removedPid)))
		require.Nil(t, service, "service PID %d should have been removed", removedPid)
		require.Error(t, err, "should get error when fetching removed service")
	}
}

func verifyServiceFields(t *testing.T, mockWmeta workloadmetamock.Mock, expectedServices map[int32]model.Service) {
	for pid, expectedModel := range expectedServices {
		service, err := mockWmeta.GetService(strconv.Itoa(int(pid)))
		require.NoError(t, err, "should be able to fetch service PID %d", pid)
		require.NotNil(t, service, "service PID %d should exist", pid)

		// Verify all core fields are correctly mapped from model.Service to workloadmeta.Service
		require.Equal(t, expectedModel.GeneratedName, service.GeneratedName, "GeneratedName mismatch for PID %d", pid)
		require.Equal(t, expectedModel.GeneratedNameSource, service.GeneratedNameSource, "GeneratedNameSource mismatch for PID %d", pid)
		require.ElementsMatch(t, expectedModel.AdditionalGeneratedNames, service.AdditionalGeneratedNames, "AdditionalGeneratedNames mismatch for PID %d", pid)
		require.ElementsMatch(t, expectedModel.TracerMetadata, service.TracerMetadata, "TracerMetadata mismatch for PID %d", pid)
		require.Equal(t, expectedModel.DDService, service.DDService, "DDService mismatch for PID %d", pid)
		require.Equal(t, expectedModel.DDServiceInjected, service.DDServiceInjected, "DDServiceInjected mismatch for PID %d", pid)
		require.ElementsMatch(t, expectedModel.Ports, service.Ports, "Ports mismatch for PID %d", pid)
		require.Equal(t, expectedModel.APMInstrumentation, service.APMInstrumentation, "APMInstrumentation mismatch for PID %d", pid)
		require.Equal(t, expectedModel.Language, service.Language, "Language mismatch for PID %d", pid)
		require.Equal(t, expectedModel.Type, service.Type, "Type mismatch for PID %d", pid)
		require.ElementsMatch(t, expectedModel.CommandLine, service.CommandLine, "CommandLine mismatch for PID %d", pid)
		require.Equal(t, expectedModel.StartTimeMilli, service.StartTimeMilli, "StartTimeMilli mismatch for PID %d", pid)
		require.Equal(t, expectedModel.LastHeartbeat, service.LastHeartbeat, "LastHeartbeat mismatch for PID %d", pid)
	}
}

func TestGetPidsToRequest_Basic(t *testing.T) {
	c, _ := newTestCollector(t)

	// Create a set of alive PIDs
	alivePids := make(pidSet)
	alivePids.add(pidNewService)
	alivePids.add(pidFreshService)
	alivePids.add(pidStaleService)

	// Add ignored PID (simulating a PID that exceeded max retry attempts)
	c.ignoredPids.add(pidFreshService)

	pids, pidsToService := c.getPidsToRequest(alivePids)

	// Only non-ignored PIDs should be returned for querying
	require.Len(t, pids, 2)
	require.Contains(t, pids, int32(pidNewService))
	require.Contains(t, pids, int32(pidStaleService))
	require.NotContains(t, pids, int32(pidFreshService))

	// The pidsToService map should have entries for all requested PIDs
	// initially set to nil (will be populated later with service data)
	require.Len(t, pidsToService, 2)
	require.Contains(t, pidsToService, int32(pidNewService))
	require.Contains(t, pidsToService, int32(pidStaleService))

	// Initially nil, will be filled by service discovery
	require.Nil(t, pidsToService[pidNewService])
	require.Nil(t, pidsToService[pidStaleService])
}

func TestServiceStoreLifetime(t *testing.T) {
	tests := []struct {
		name                string
		existingServices    []*workloadmeta.Service
		alivePids           []int32
		ignoredPids         []int32
		httpResponse        *model.ServicesResponse
		httpError           error
		expectedRequests    []int32
		expectStored        []int32
		expectRemoved       []int32
		expectFieldsUpdated map[int32]model.Service
	}{
		{
			name:             "new service discovered and stored",
			existingServices: []*workloadmeta.Service{},
			alivePids:        []int32{pidNewService},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{createModelService(pidNewService, "new-service")},
			},
			expectedRequests:    []int32{pidNewService},
			expectStored:        []int32{pidNewService},
			expectRemoved:       []int32{},
			expectFieldsUpdated: map[int32]model.Service{pidNewService: createModelService(pidNewService, "new-service")},
		},
		{
			name: "fresh service skipped, stale service updated",
			existingServices: []*workloadmeta.Service{
				createWorkloadmetaService(pidFreshService, "fresh", -5*time.Minute),  // fresh
				createWorkloadmetaService(pidStaleService, "stale", -20*time.Minute), // stale
			},
			alivePids: []int32{pidFreshService, pidStaleService},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{createModelService(pidStaleService, "updated-stale")},
			},
			expectedRequests:    []int32{pidStaleService}, // only stale service
			expectStored:        []int32{pidFreshService, pidStaleService},
			expectRemoved:       []int32{},
			expectFieldsUpdated: map[int32]model.Service{pidStaleService: createModelService(pidStaleService, "updated-stale")},
		},
		{
			name: "dead service removed from store",
			existingServices: []*workloadmeta.Service{
				createWorkloadmetaService(pidFreshService, "live", -5*time.Minute),
				createWorkloadmetaService(pidDeadService, "dead", -5*time.Minute),
			},
			alivePids:           []int32{pidFreshService}, // pidDeadService is dead
			httpResponse:        &model.ServicesResponse{Services: []model.Service{}},
			expectedRequests:    []int32{}, // pidFreshService is fresh, no requests needed
			expectStored:        []int32{pidFreshService},
			expectRemoved:       []int32{pidDeadService},
			expectFieldsUpdated: map[int32]model.Service{},
		},
		{
			name:        "ignored pids are skipped",
			alivePids:   []int32{pidNewService, pidIgnoredService},
			ignoredPids: []int32{pidIgnoredService},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{createModelService(pidNewService, "service-new")},
			},
			expectedRequests:    []int32{pidNewService}, // pidIgnoredService ignored
			expectStored:        []int32{pidNewService},
			expectRemoved:       []int32{},
			expectFieldsUpdated: map[int32]model.Service{pidNewService: createModelService(pidNewService, "service-new")},
		},
		{
			name: "comprehensive scenario - new, fresh, stale, dead services",
			existingServices: []*workloadmeta.Service{
				createWorkloadmetaService(pidFreshService, "fresh", -5*time.Minute),  // fresh - skip
				createWorkloadmetaService(pidStaleService, "stale", -20*time.Minute), // stale - update
				createWorkloadmetaService(pidDeadService, "dead", -10*time.Minute),   // dead - remove
			},
			alivePids: []int32{pidNewService, pidFreshService, pidStaleService}, // pidDeadService is dead
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{
					createModelService(pidNewService, "new-service"),
					createModelService(pidStaleService, "updated-stale"),
				},
			},
			expectedRequests: []int32{pidNewService, pidStaleService}, // new + stale
			expectStored:     []int32{pidNewService, pidFreshService, pidStaleService},
			expectRemoved:    []int32{pidDeadService},
			expectFieldsUpdated: map[int32]model.Service{
				pidNewService:   createModelService(pidNewService, "new-service"),
				pidStaleService: createModelService(pidStaleService, "updated-stale"),
			},
		},
		{
			name:                "http error handled gracefully",
			existingServices:    []*workloadmeta.Service{},
			alivePids:           []int32{pidNewService},
			httpError:           http.ErrNotSupported,
			expectedRequests:    []int32{pidNewService},
			expectStored:        []int32{}, // no services stored due to error
			expectRemoved:       []int32{},
			expectFieldsUpdated: map[int32]model.Service{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, mockWmeta := newTestCollector(t)

			// Set up ignored PIDs
			for _, pid := range tc.ignoredPids {
				c.ignoredPids.add(pid)
			}

			// Pre-populate store with existing services
			for _, service := range tc.existingServices {
				mockWmeta.Set(service)
			}

			// Mock alive PIDs
			alivePids := make(pidSet)
			for _, pid := range tc.alivePids {
				alivePids.add(pid)
			}

			// Create system-probe test server
			socketPath, _ := startTestServer(t, tc.httpResponse, tc.httpError != nil)

			// Override the collector's HTTP client to use the test socket
			c.sysProbeClient = sysprobeclient.Get(socketPath)

			// Capture which PIDs are requested (before running full update)
			requestedPids, _ := c.getPidsToRequest(alivePids)

			// Verify expected requests
			require.ElementsMatch(t, tc.expectedRequests, requestedPids, "unexpected PIDs requested")

			// Run the full update cycle
			c.updateServices()

			// Verify final state using helper functions
			verifyStoredServices(t, mockWmeta, tc.expectStored)
			verifyRemovedServices(t, mockWmeta, tc.expectRemoved)
			verifyServiceFields(t, mockWmeta, tc.expectFieldsUpdated)
		})
	}
}
