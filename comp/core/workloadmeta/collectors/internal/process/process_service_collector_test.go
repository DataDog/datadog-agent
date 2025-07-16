// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

const (
	pidNewService     = 123 // New service; to be discovered
	pidFreshService   = 456 // Fresh service; updated recently
	pidStaleService   = 789 // Stale service; need a refresh
	pidIgnoredService = 555 // Ignored service; ignored pid
	pidRecentService  = 999 // Recent service; new process, start time < 1 minute
)

var baseTime = time.Date(2025, 1, 12, 1, 0, 0, 0, time.UTC) // 12th of January 2025, 1am UTC

// startTestServer creates a system-probe test server that returns the specified response or error
func startTestServer(t *testing.T, response *model.ServicesEndpointResponse, shouldError bool) (string, *httptest.Server) {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discovery/services" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if shouldError {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		responseBytes, _ := json.Marshal(response)
		w.Write(responseBytes)
	})

	socketPath := testutil.SystemProbeSocketPath(t, "")
	server, err := testutil.NewSystemProbeTestServer(handler, socketPath)
	require.NoError(t, err)
	require.NotNil(t, server)
	server.Start()
	t.Cleanup(server.Close)

	return socketPath, server
}

func makeProcessMap(pids []int32, createTimes map[int32]time.Time) map[int32]*procutil.Process {
	procs := make(map[int32]*procutil.Process)
	for _, pid := range pids {
		createTime := baseTime.Add(-2 * time.Minute) // Default: process started 2 minutes before baseTime
		if t, exists := createTimes[pid]; exists {
			createTime = t
		}
		procs[pid] = &procutil.Process{
			Pid:   pid,
			Stats: &procutil.Stats{CreateTime: createTime.UnixMilli()},
		}
	}
	return procs
}

func makeModelService(pid int32, name string) model.Service {
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
		CommandLine:        []string{"python", "-m", "myservice"},
		StartTimeMilli:     uint64(baseTime.Add(-1 * time.Minute).UnixMilli()),
		LogFiles:           []string{"/var/log/" + name + ".log"},
	}
}

func makeProcessEntityService(pid int32, name string) *workloadmeta.Process {
	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(pid)),
		},
		Pid: pid,
		Service: &workloadmeta.Service{
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
			Type:               "database",
			LogFiles:           []string{"/var/log/" + name + ".log"},
		},
	}
}

func makeProcessEntityServiceProcessCollectionDisabled(pid int32, name string) *workloadmeta.Process {
	process := makeProcessEntityService(pid, name)
	// When process collection is disabled, additional process fields are populated by service collection
	process.Cmdline = []string{"python", "-m", "myservice"}
	process.CreationTime = baseTime.Add(-1 * time.Minute)
	return process
}

func assertStoredServices(t *testing.T, store workloadmetamock.Mock, expected []*workloadmeta.Process) {
	for _, expectedProcess := range expected {
		if expectedProcess == nil {
			continue
		}

		assert.EventuallyWithT(t, func(collectT *assert.CollectT) {
			entity, err := store.GetProcess(expectedProcess.Pid)
			assert.NoError(collectT, err)
			assert.NotNil(collectT, entity)
			assert.NotNil(collectT, entity.Service)

			// Verify all service fields match expected values
			assert.Equal(collectT, expectedProcess.Service.GeneratedName, entity.Service.GeneratedName)
			assert.Equal(collectT, expectedProcess.Service.GeneratedNameSource, entity.Service.GeneratedNameSource)
			assert.Equal(collectT, expectedProcess.Service.AdditionalGeneratedNames, entity.Service.AdditionalGeneratedNames)
			assert.Equal(collectT, expectedProcess.Service.TracerMetadata, entity.Service.TracerMetadata)
			assert.Equal(collectT, expectedProcess.Service.DDService, entity.Service.DDService)
			assert.Equal(collectT, expectedProcess.Service.DDServiceInjected, entity.Service.DDServiceInjected)
			assert.Equal(collectT, expectedProcess.Service.Ports, entity.Service.Ports)
			assert.Equal(collectT, expectedProcess.Service.APMInstrumentation, entity.Service.APMInstrumentation)
			assert.Equal(collectT, expectedProcess.Service.Type, entity.Service.Type)
			assert.Equal(collectT, expectedProcess.Service.LogFiles, entity.Service.LogFiles)
		}, 2*time.Second, 100*time.Millisecond)
	}
}

func assertProcessWithoutServices(t *testing.T, store workloadmetamock.Mock, pids []int32) {
	if len(pids) == 0 {
		return
	}

	// Verify that processes exist but have no service data
	assert.EventuallyWithT(t, func(collectT *assert.CollectT) {
		for _, pid := range pids {
			entity, err := store.GetProcess(pid)
			assert.NoError(collectT, err, "PID %d should exist in store", pid)
			assert.NotNil(collectT, entity, "PID %d should exist in store", pid)
			// Process should exist but have no service data
			assert.Nil(collectT, entity.Service, "PID %d should not have service data", pid)
		}
	}, 1*time.Second, 100*time.Millisecond)
}

func assertNoEntitiesForPids(t *testing.T, store workloadmetamock.Mock, pids []int32) {
	if len(pids) == 0 {
		return
	}

	assert.EventuallyWithT(t, func(collectT *assert.CollectT) {
		for _, pid := range pids {
			entity, err := store.GetProcess(pid)
			assert.Error(collectT, err, "PID %d should not exist in store", pid)
			assert.Nil(collectT, entity, "PID %d should exist in store", pid)
		}
	}, 1*time.Second, 100*time.Millisecond)
}

func assertProcessesExist(t *testing.T, store workloadmetamock.Mock, pids []int32) {
	if len(pids) == 0 {
		return
	}

	// Verify that processes exist (regardless of service data)
	assert.EventuallyWithT(t, func(collectT *assert.CollectT) {
		for _, pid := range pids {
			entity, err := store.GetProcess(pid)
			assert.NoError(collectT, err, "PID %d should exist in store", pid)
			assert.NotNil(collectT, entity, "PID %d should exist in store", pid)
		}
	}, 1*time.Second, 100*time.Millisecond)
}

func TestFilterPidsToRequest(t *testing.T) {
	c := setUpCollectorTest(t, nil, nil, nil)

	// Set up test time using baseTime
	c.mockClock.Set(baseTime)

	// Create a set of alive PIDs
	alivePids := make(core.PidSet)
	alivePids.Add(pidNewService)     // No cache entry (should be requested)
	alivePids.Add(pidFreshService)   // Fresh cache entry (should NOT be requested)
	alivePids.Add(pidStaleService)   // Stale cache entry (should be requested)
	alivePids.Add(pidIgnoredService) // Ignored PID (should NOT be requested)
	alivePids.Add(pidRecentService)

	// Set up pidHeartbeats cache
	c.collector.pidHeartbeats[pidFreshService] = baseTime.Add(-5 * time.Minute)  // Fresh (5 minutes ago)
	c.collector.pidHeartbeats[pidStaleService] = baseTime.Add(-20 * time.Minute) // Stale (20 minutes ago)

	// Create mock processes map
	procs := make(map[int32]*procutil.Process)
	procs[pidNewService] = &procutil.Process{
		Pid: pidNewService,
		Stats: &procutil.Stats{
			CreateTime: baseTime.Add(-2 * time.Minute).UnixMilli(), // Started 2 minutes ago
		},
	}
	procs[pidFreshService] = &procutil.Process{
		Pid: pidFreshService,
		Stats: &procutil.Stats{
			CreateTime: baseTime.Add(-2 * time.Minute).UnixMilli(), // Started 2 minutes ago
		},
	}
	procs[pidStaleService] = &procutil.Process{
		Pid: pidStaleService,
		Stats: &procutil.Stats{
			CreateTime: baseTime.Add(-2 * time.Minute).UnixMilli(), // Started 2 minutes ago
		},
	}
	procs[pidRecentService] = &procutil.Process{
		Pid: pidRecentService,
		Stats: &procutil.Stats{
			CreateTime: baseTime.Add(-30 * time.Second).UnixMilli(), // Started 30 seconds ago (should be filtered out)
		},
	}

	// Add ignored PID (simulating a PID that exceeded max retry attempts)
	c.collector.ignoredPids.Add(pidIgnoredService)

	pids, pidsToService := c.collector.filterPidsToRequest(alivePids, procs)

	require.Len(t, pids, 2)
	require.Contains(t, pids, int32(pidNewService))
	require.Contains(t, pids, int32(pidStaleService))
	require.NotContains(t, pids, int32(pidFreshService))   // Fresh, should not be requested
	require.NotContains(t, pids, int32(pidIgnoredService)) // Ignored, should not be requested
	require.NotContains(t, pids, int32(pidRecentService))  // too recent (< 1 minute)

	// The pidsToService map should have entries for all requested PIDs
	require.Len(t, pidsToService, 2)
	require.Contains(t, pidsToService, int32(pidNewService))
	require.Contains(t, pidsToService, int32(pidStaleService))

	// Initially nil, will be filled by service discovery
	require.Nil(t, pidsToService[pidNewService])
	require.Nil(t, pidsToService[pidStaleService])
}

func TestServiceStoreLifetimeProcessCollectionDisabled(t *testing.T) {
	const collectionInterval = 1 * time.Second

	tests := []struct {
		name               string
		shouldError        bool
		httpResponse       *model.ServicesEndpointResponse
		alivePids          []int32
		ignoredPids        []int32
		existingProcesses  []*workloadmeta.Process
		expectStored       []*workloadmeta.Process
		pidHeartbeats      map[int32]time.Time
		processCreateTimes map[int32]time.Time
		expectNoEntities   []int32
	}{
		{
			name:      "new service discovered",
			alivePids: []int32{pidNewService},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidNewService, "new-service")},
			},
			expectStored: []*workloadmeta.Process{makeProcessEntityServiceProcessCollectionDisabled(pidNewService, "new-service")},
		},
		{
			name:        "http error handled",
			alivePids:   []int32{pidNewService},
			shouldError: true,
		},
		{
			name:        "ignored pid skipped",
			alivePids:   []int32{pidIgnoredService},
			ignoredPids: []int32{pidIgnoredService},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidIgnoredService, "ignored-service")},
			},
		},
		{
			name:      "fresh vs stale services",
			alivePids: []int32{pidFreshService, pidStaleService},
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntityServiceProcessCollectionDisabled(pidFreshService, "fresh-existing"),
				makeProcessEntityServiceProcessCollectionDisabled(pidStaleService, "stale-existing"),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{
					makeModelService(pidStaleService, "stale-service"),
				},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityServiceProcessCollectionDisabled(pidFreshService, "fresh-existing"),
				makeProcessEntityServiceProcessCollectionDisabled(pidStaleService, "stale-service"),
			},
			pidHeartbeats: map[int32]time.Time{
				pidFreshService: baseTime.Add(-5 * time.Minute),
				pidStaleService: baseTime.Add(-20 * time.Minute),
			},
		},
		{
			name:      "young process ignored",
			alivePids: []int32{pidRecentService},
			processCreateTimes: map[int32]time.Time{
				pidRecentService: baseTime.Add(-30 * time.Second), // Process started 30 seconds ago (too young)
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidRecentService, "recent-service")},
			},
			expectNoEntities: []int32{pidRecentService}, // Process should exist but have no service data
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := setUpCollectorTest(t, nil, nil, nil)
			ctx := t.Context()

			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(map[int]string{}).AnyTimes()

			socketPath, _ := startTestServer(t, tc.httpResponse, tc.shouldError)
			c.collector.sysProbeClient = sysprobeclient.Get(socketPath)

			for _, pid := range tc.ignoredPids {
				c.collector.ignoredPids.Add(pid)
			}

			for _, process := range tc.existingProcesses {
				c.mockStore.Set(process)
			}

			c.mockClock.Set(baseTime)

			if tc.pidHeartbeats != nil {
				c.collector.pidHeartbeats = tc.pidHeartbeats
			}

			// TODO: we should use Start() instead of these lines below when configuration is sorted as Start() is currently
			// by default disabled
			// Only start collectServicesDefault and stream (not collectProcesses) since process collection is disabled
			c.collector.containerProvider = c.mockContainerProvider
			c.collector.store = c.mockStore
			go c.collector.collectServicesNoCache(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			// Mock processProbe.ProcessesByPID to be called directly by collectServicesDefault
			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(makeProcessMap(tc.alivePids, tc.processCreateTimes), nil).Maybe()

			// Trigger service collection
			c.mockClock.Add(collectionInterval)

			assertStoredServices(t, c.mockStore, tc.expectStored)
			assertNoEntitiesForPids(t, c.mockStore, tc.expectNoEntities)

			// When process collection is disabled, ignored PIDs and error cases don't create process entities
			// since they only get created when services are successfully discovered
		})
	}
}

func TestServiceStoreLifetime(t *testing.T) {
	const collectionInterval = 1 * time.Second

	tests := []struct {
		name                    string
		shouldError             bool
		httpResponse            *model.ServicesEndpointResponse
		alivePids               []int32
		ignoredPids             []int32
		existingProcesses       []*workloadmeta.Process
		expectStored            []*workloadmeta.Process
		pidHeartbeats           map[int32]time.Time
		processCreateTimes      map[int32]time.Time
		expectNoServiceDataPids []int32
	}{
		{
			name:      "new service discovered and stored",
			alivePids: []int32{pidNewService},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidNewService, "new-service")},
			},
			expectStored: []*workloadmeta.Process{makeProcessEntityService(pidNewService, "new-service")},
		},
		{
			name:        "http error handled gracefully",
			alivePids:   []int32{pidNewService},
			shouldError: true,
			// expectStored is nil - no services should be stored when HTTP error occurs
		},
		{
			name:        "ignored pid is skipped",
			alivePids:   []int32{pidIgnoredService},
			ignoredPids: []int32{pidIgnoredService},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidIgnoredService, "ignored-service")},
			},
			// No expectStored since the PID should be ignored and no service should be stored
		},
		{
			name:      "fresh service not updated, stale service updated",
			alivePids: []int32{pidFreshService, pidStaleService},
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntityService(pidFreshService, "fresh-existing"), // Recent
				makeProcessEntityService(pidStaleService, "stale-existing"), // Stale (> 15min)
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{
					makeModelService(pidStaleService, "stale-service"), // Only stale service should be requested
				},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityService(pidFreshService, "fresh-existing"), // Fresh service unchanged
				makeProcessEntityService(pidStaleService, "stale-service"),  // Stale service updated
			},
			pidHeartbeats: map[int32]time.Time{
				pidFreshService: baseTime.Add(-5 * time.Minute),  // Fresh (5 minutes ago)
				pidStaleService: baseTime.Add(-20 * time.Minute), // Stale (20 minutes ago)
			},
		},
		{
			name:      "young process ignored",
			alivePids: []int32{pidRecentService},
			processCreateTimes: map[int32]time.Time{
				pidRecentService: baseTime.Add(-30 * time.Second), // Process started 30 seconds ago (too young)
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidRecentService, "recent-service")},
			},
			expectNoServiceDataPids: []int32{pidRecentService}, // Process should exist but have no service data
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Collector setup
			c := setUpCollectorTest(t, nil, nil, nil)
			ctx := t.Context()
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(map[int]string{}).AnyTimes()

			// Create test server & override collector client
			socketPath, _ := startTestServer(t, tc.httpResponse, tc.shouldError)
			c.collector.sysProbeClient = sysprobeclient.Get(socketPath)

			// Add ignored PIDs to the collector
			for _, pid := range tc.ignoredPids {
				c.collector.ignoredPids.Add(pid)
			}

			// Pre-populate store with existing processes
			for _, process := range tc.existingProcesses {
				c.mockStore.Set(process)
			}

			// Set mock clock to baseTime to control LastHeartbeat in tests
			c.mockClock.Set(baseTime)

			// Pre-populate pidHeartbeats cache if specified in test case
			if tc.pidHeartbeats != nil {
				c.collector.pidHeartbeats = tc.pidHeartbeats
			}

			// TODO: we should use Start() instead of these lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.containerProvider = c.mockContainerProvider
			c.collector.store = c.mockStore
			go c.collector.collectProcesses(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.collectServicesCached(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(makeProcessMap(tc.alivePids, tc.processCreateTimes), nil).Maybe()

			// Trigger process collection first to populate lastCollectedProcesses
			c.mockClock.Add(collectionInterval)

			// Wait for processes to be stored (confirms process collection completed)
			assertProcessesExist(t, c.mockStore, tc.alivePids)

			// Trigger service collection
			c.mockClock.Add(collectionInterval)

			assertStoredServices(t, c.mockStore, tc.expectStored)
			assertProcessWithoutServices(t, c.mockStore, tc.ignoredPids)

			// For HTTP error cases, verify processes exist but have no service data
			if tc.shouldError {
				assertProcessWithoutServices(t, c.mockStore, tc.alivePids)
			}

			// For processes that should exist but have no service data (e.g., too young)
			assertProcessWithoutServices(t, c.mockStore, tc.expectNoServiceDataPids)
		})
	}
}

func TestProcessDeathRemovesServiceData(t *testing.T) {
	const collectionInterval = 1 * time.Second

	c := setUpCollectorTest(t, nil, nil, nil)
	ctx := t.Context()

	// Set initial state: process entity in the store, SD was tracking a service,
	// the process collector reported no live processes.
	existingProcess := makeProcessEntityService(pidFreshService, "existing-service")
	c.mockStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceServiceDiscovery,
			Entity: existingProcess,
		},
	})
	c.collector.lastCollectedProcesses = make(map[int32]*procutil.Process)
	c.collector.pidHeartbeats[pidFreshService] = baseTime

	socketPath, _ := startTestServer(t, &model.ServicesEndpointResponse{}, false)
	c.collector.sysProbeClient = sysprobeclient.Get(socketPath)
	c.mockClock.Set(baseTime)

	c.collector.store = c.mockStore

	go c.collector.collectServicesCached(ctx, c.collector.clock.Ticker(collectionInterval))
	go c.collector.stream(ctx)

	c.mockClock.Add(collectionInterval)

	assertNoEntitiesForPids(t, c.mockStore, []int32{pidFreshService})
}
