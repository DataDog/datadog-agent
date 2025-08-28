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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
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

// TestServiceStoreLifetimeProcessCollectionDisabled tests service discovery collection when process collection and language detection are disabled
func TestServiceStoreLifetimeProcessCollectionDisabled(t *testing.T) {
	const collectionInterval = 1 * time.Minute

	configOverrides := map[string]interface{}{
		"process_config.process_collection.enabled": false,
		"language_detection.enabled":                false,
		"process_config.process_collection.use_wlm": true,
	}

	sysConfigOverrides := map[string]interface{}{
		"discovery.enabled": true,
	}
	languagePython := &languagemodels.Language{
		Name: languagemodels.Python,
	}

	tests := []struct {
		name               string
		shouldError        bool
		httpResponse       *model.ServicesEndpointResponse
		ignoredPids        []int32
		processesToCollect map[int32]*procutil.Process
		existingProcesses  []*workloadmeta.Process
		expectStored       []*workloadmeta.Process
		pidHeartbeats      map[int32]time.Time
		expectNoEntities   []int32
	}{
		{
			name: "new service discovered",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidNewService, "new-service")},
			},
			expectStored: []*workloadmeta.Process{makeProcessEntityWithService(pidNewService, baseTime.Add(-2*time.Minute), languagePython, "new-service")},
		},
		{
			name: "http error handled",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			shouldError: true,
		},
		{
			name: "ignored pid skipped",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			ignoredPids: []int32{pidIgnoredService},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidIgnoredService, "ignored-service")},
			},
		},
		{
			name: "fresh vs stale services",
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntityWithService(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, "fresh-existing"),
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-existing"),
			},
			processesToCollect: map[int32]*procutil.Process{
				pidFreshService: makeProcess(pidFreshService, baseTime.Add(-5*time.Minute).UnixMilli(), nil),
				pidStaleService: makeProcess(pidStaleService, baseTime.Add(-20*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{
					makeModelService(pidStaleService, "stale-service"),
				},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityWithService(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, "fresh-existing"),
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-service"),
			},
			pidHeartbeats: map[int32]time.Time{
				pidFreshService: baseTime.Add(-5 * time.Minute),
				pidStaleService: baseTime.Add(-20 * time.Minute),
			},
		},
		{
			name: "young process ignored",
			processesToCollect: map[int32]*procutil.Process{
				// The service collector runs after advancing the mock clock by 60s.
				// To ensure the process is considered "young" (< 1 minute old) at that time,
				// set its start time to baseTime + 30s so that now - start = 30s when the tick fires.
				pidRecentService: makeProcess(pidRecentService, baseTime.Add(30*time.Second).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidRecentService, "recent-service")},
			},
			expectNoEntities: []int32{pidRecentService}, // Process should exist but have no service data
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := setUpCollectorTest(t, configOverrides, sysConfigOverrides, nil)
			defer c.cleanup()
			ctx := t.Context()

			socketPath, _ := startTestServer(t, tc.httpResponse, tc.shouldError)
			c.collector.sysProbeClient = sysprobeclient.Get(socketPath)

			for _, pid := range tc.ignoredPids {
				c.collector.ignoredPids.Add(pid)
			}

			for _, process := range tc.existingProcesses {
				// we use notify instead of set here because we want to control the source as it impacts how data is merged/stored in wlm
				c.mockStore.Notify([]workloadmeta.CollectorEvent{
					{
						Type:   workloadmeta.EventTypeSet,
						Source: workloadmeta.SourceServiceDiscovery,
						Entity: process,
					},
				})
			}

			c.mockClock.Set(baseTime)

			if tc.pidHeartbeats != nil {
				c.collector.pidHeartbeats = tc.pidHeartbeats
			}

			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

			// Mock processProbe.ProcessesByPID to be called directly by collectServicesDefault
			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Maybe()
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(map[int]string{}).AnyTimes()

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
	const collectionIntervalSeconds = 60
	const collectionInterval = time.Duration(collectionIntervalSeconds) * time.Second

	configOverrides := map[string]interface{}{
		"process_config.process_collection.enabled": true,
		"process_config.process_collection.use_wlm": true,
		"language_detection.enabled":                true,
		// setting process collection interval to the same as the service collection interval
		// because it makes the test simpler until the service collection interval is configurable
		"process_config.intervals.process": collectionIntervalSeconds,
	}

	sysConfigOverrides := map[string]interface{}{
		"discovery.enabled": true,
	}

	languagePython := &languagemodels.Language{
		Name: languagemodels.Python,
	}

	tests := []struct {
		name                string
		shouldError         bool
		httpResponse        *model.ServicesEndpointResponse
		ignoredPids         []int32
		existingProcessData []*workloadmeta.Process
		existingServiceData []*workloadmeta.Process
		expectStored        []*workloadmeta.Process
		pidHeartbeats       map[int32]time.Time
		processesToCollect  map[int32]*procutil.Process
	}{
		{
			name: "new service discovered and stored",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), languagePython),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidNewService, "new-service")},
			},
			expectStored: []*workloadmeta.Process{makeProcessEntityWithService(pidNewService, baseTime.Add(-2*time.Minute), languagePython, "new-service")},
		},
		{
			name: "http error handled gracefully",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), languagePython),
			},
			shouldError: true,
			// expectStored should have no service data should be stored when HTTP error occurs
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidNewService, baseTime.Add(-2*time.Minute), languagePython)},
		},
		{
			name: "ignored pid is skipped",
			processesToCollect: map[int32]*procutil.Process{
				pidIgnoredService: makeProcess(pidIgnoredService, baseTime.Add(-2*time.Minute).UnixMilli(), languagePython),
			},
			ignoredPids: []int32{pidIgnoredService},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidIgnoredService, "ignored-service")},
			},
			// Process should exist but have no service data
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidIgnoredService, baseTime.Add(-2*time.Minute), languagePython)},
		},
		{
			name: "fresh service not updated, stale service updated",
			existingProcessData: []*workloadmeta.Process{
				makeProcessEntity(pidFreshService, baseTime.Add(-5*time.Minute), languagePython),  // Recent
				makeProcessEntity(pidStaleService, baseTime.Add(-20*time.Minute), languagePython), // Stale (> 15min)
			},
			existingServiceData: []*workloadmeta.Process{
				makeProcessEntityService(pidFreshService, "fresh-existing"), // Recent
				makeProcessEntityService(pidStaleService, "stale-existing"), // Stale (> 15min)
			},
			processesToCollect: map[int32]*procutil.Process{
				pidFreshService: makeProcess(pidFreshService, baseTime.Add(-5*time.Minute).UnixMilli(), languagePython),
				pidStaleService: makeProcess(pidStaleService, baseTime.Add(-20*time.Minute).UnixMilli(), languagePython),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{
					makeModelService(pidStaleService, "stale-service"), // Only stale service should be requested
				},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityWithService(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, "fresh-existing"),
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-service"),
			},
			pidHeartbeats: map[int32]time.Time{
				pidFreshService: baseTime.Add(-5 * time.Minute),  // Fresh (5 minutes ago)
				pidStaleService: baseTime.Add(-20 * time.Minute), // Stale (20 minutes ago)
			},
		},
		{
			name: "young process ignored",
			processesToCollect: map[int32]*procutil.Process{
				// The test runs 2 collection intervals, so at the time of the second collection interval
				// 30 seconds ago = 1 minute and 30 seconds from now
				pidRecentService: makeProcess(pidRecentService, baseTime.Add(time.Minute+30*time.Second).UnixMilli(), languagePython),
			},
			httpResponse: &model.ServicesEndpointResponse{
				Services: []model.Service{makeModelService(pidRecentService, "recent-service")},
			},
			// Process should exist but have no service data
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidRecentService, baseTime.Add(time.Minute+30*time.Second), languagePython)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Collector setup
			c := setUpCollectorTest(t, configOverrides, sysConfigOverrides, nil)
			defer c.cleanup()
			ctx := t.Context()

			// Create test server & override collector client
			socketPath, _ := startTestServer(t, tc.httpResponse, tc.shouldError)
			c.collector.sysProbeClient = sysprobeclient.Get(socketPath)

			// Add ignored PIDs to the collector
			for _, pid := range tc.ignoredPids {
				c.collector.ignoredPids.Add(pid)
			}

			// Pre-populate store with existing processes
			for _, process := range tc.existingProcessData {
				// we use notify instead of set here because we want to control the source as it impacts how data is merged/stored in wlm
				c.mockStore.Notify([]workloadmeta.CollectorEvent{
					{
						Type:   workloadmeta.EventTypeSet,
						Source: workloadmeta.SourceProcessCollector,
						Entity: process,
					},
				})
			}
			for _, process := range tc.existingServiceData {
				// we use notify instead of set here because we want to control the source as it impacts how data is merged/stored in wlm
				c.mockStore.Notify([]workloadmeta.CollectorEvent{
					{
						Type:   workloadmeta.EventTypeSet,
						Source: workloadmeta.SourceServiceDiscovery,
						Entity: process,
					},
				})
			}

			// Set mock clock to baseTime to control LastHeartbeat in tests
			c.mockClock.Set(baseTime)

			// Pre-populate pidHeartbeats cache if specified in test case
			if tc.pidHeartbeats != nil {
				c.collector.pidHeartbeats = tc.pidHeartbeats
			}

			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Maybe()
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(map[int]string{}).AnyTimes()

			// Trigger process collection first to populate lastCollectedProcesses
			c.mockClock.Add(collectionInterval)

			// Wait for processes to be stored (confirms process collection completed)
			assertProcessData(t, c.mockStore, tc.expectStored)

			// Trigger service collection
			c.mockClock.Add(collectionInterval)

			// reconfirm data still exists
			assertProcessData(t, c.mockStore, tc.expectStored)

			// For HTTP error cases, verify processes exist but have no service data
			if tc.shouldError {
				var pids []int32
				for _, proc := range tc.expectStored {
					pids = append(pids, proc.Pid)
				}
				assertProcessWithoutServices(t, c.mockStore, pids)
			} else {
				assertStoredServices(t, c.mockStore, tc.expectStored)
			}
			assertProcessWithoutServices(t, c.mockStore, tc.ignoredPids)
		})
	}
}

func TestProcessDeathRemovesServiceData(t *testing.T) {
	const collectionIntervalSeconds = 60
	const collectionInterval = time.Duration(collectionIntervalSeconds) * time.Second

	configOverrides := map[string]interface{}{
		"process_config.process_collection.enabled": true,
		"process_config.process_collection.use_wlm": true,
		"language_detection.enabled":                true,
		// setting process collection interval to the same as the service collection interval
		// because it makes the test simpler until the service collection interval is configurable
		"process_config.intervals.process": collectionIntervalSeconds,
	}

	sysConfigOverrides := map[string]interface{}{
		"discovery.enabled": true,
	}

	c := setUpCollectorTest(t, configOverrides, sysConfigOverrides, nil)
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

	err := c.collector.Start(ctx, c.mockStore)
	assert.NoError(t, err)
	c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(nil, nil).Times(1)
	c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

	c.mockClock.Add(collectionInterval)

	assertNoEntitiesForPids(t, c.mockStore, []int32{pidFreshService})
}

func TestServiceLanguageToWLMLanguageMapping(t *testing.T) {
	for _, tc := range []struct {
		serviceLanguage string
		expected        *languagemodels.Language
	}{
		{string(language.Java), &languagemodels.Language{Name: languagemodels.Java}},
		{string(language.Node), &languagemodels.Language{Name: languagemodels.Node}},
		{string(language.Python), &languagemodels.Language{Name: languagemodels.Python}},
		{string(language.Ruby), &languagemodels.Language{Name: languagemodels.Ruby}},
		{string(language.DotNet), &languagemodels.Language{Name: languagemodels.Dotnet}},
		{string(language.Go), &languagemodels.Language{Name: languagemodels.Go}},
		{string(language.CPlusPlus), &languagemodels.Language{Name: languagemodels.CPP}},
		{string(language.Unknown), &languagemodels.Language{Name: languagemodels.Unknown}},
		{"RANDOM", &languagemodels.Language{Name: languagemodels.Unknown}},
	} {
		assert.Equal(t, tc.expected, convertServiceLanguageToWLMLanguage(tc.serviceLanguage))
	}
}

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
		TCPPorts:           []uint16{3000, 4000},
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
			TCPPorts:           []uint16{3000, 4000},
			APMInstrumentation: "manual",
			Type:               "database",
			LogFiles:           []string{"/var/log/" + name + ".log"},
		},
	}
}

func makeProcessEntity(pid int32, createTime time.Time, language *languagemodels.Language) *workloadmeta.Process {
	proc := makeProcess(pid, createTime.UnixMilli(), language)
	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(pid)),
		},
		CreationTime: time.UnixMilli(proc.Stats.CreateTime).UTC(),
		Pid:          proc.Pid,
		Ppid:         proc.Ppid,
		NsPid:        proc.NsPid,
		Name:         proc.Name,
		Cwd:          proc.Cwd,
		Exe:          proc.Exe,
		Comm:         proc.Comm,
		Cmdline:      proc.Cmdline,
		Language:     proc.Language,
		Uids:         proc.Uids,
		Gids:         proc.Gids,
	}
}

func makeProcessEntityWithService(pid int32, createTime time.Time, language *languagemodels.Language, name string) *workloadmeta.Process {
	process := makeProcessEntity(pid, createTime, language)
	process.Service = makeProcessEntityService(pid, name).Service
	return process
}

func makeProcess(pid int32, createTime int64, language *languagemodels.Language) *procutil.Process {
	return &procutil.Process{
		Pid:      pid,
		Ppid:     6,
		NsPid:    2,
		Name:     "some name",
		Cwd:      "some_directory/path",
		Exe:      "test",
		Comm:     "",
		Cmdline:  []string{"python3", "--version"},
		Language: language,
		Uids:     []int32{1, 2, 3, 4},
		Gids:     []int32{1, 2, 3, 4, 5},
		Stats: &procutil.Stats{
			CreateTime: createTime,
		},
	}
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
			if expectedProcess.Service == nil {
				assert.Nil(collectT, entity.Service)
			} else {
				require.NotNil(collectT, entity.Service)
				// Verify all service fields match expected values
				assert.Equal(collectT, expectedProcess.Service.GeneratedName, entity.Service.GeneratedName)
				assert.Equal(collectT, expectedProcess.Service.GeneratedNameSource, entity.Service.GeneratedNameSource)
				assert.Equal(collectT, expectedProcess.Service.AdditionalGeneratedNames, entity.Service.AdditionalGeneratedNames)
				assert.Equal(collectT, expectedProcess.Service.TracerMetadata, entity.Service.TracerMetadata)
				assert.Equal(collectT, expectedProcess.Service.DDService, entity.Service.DDService)
				assert.Equal(collectT, expectedProcess.Service.TCPPorts, entity.Service.TCPPorts)
				assert.Equal(collectT, expectedProcess.Service.UDPPorts, entity.Service.UDPPorts)
				assert.Equal(collectT, expectedProcess.Service.APMInstrumentation, entity.Service.APMInstrumentation)
				assert.Equal(collectT, expectedProcess.Service.Type, entity.Service.Type)
				assert.Equal(collectT, expectedProcess.Service.LogFiles, entity.Service.LogFiles)
			}
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

func assertProcessData(t *testing.T, store workloadmetamock.Mock, expectedProcesses []*workloadmeta.Process) {
	if len(expectedProcesses) == 0 {
		procs := store.ListProcesses()
		assert.Len(t, procs, 0)
		return
	}

	// Verify that processes exist (regardless of service data)
	assert.EventuallyWithT(t, func(collectT *assert.CollectT) {
		for _, expectedProcess := range expectedProcesses {
			entity, err := store.GetProcess(expectedProcess.Pid)
			assert.NoError(collectT, err, "PID %d should exist in store", expectedProcess.Pid)
			require.NotNil(collectT, entity, "PID %d should exist in store", expectedProcess.Pid)
			assert.Equal(collectT, expectedProcess.Pid, entity.Pid)
			assert.Equal(collectT, expectedProcess.NsPid, entity.NsPid)
			assert.Equal(collectT, expectedProcess.Ppid, entity.Ppid)
			assert.Equal(collectT, expectedProcess.Name, entity.Name)
			assert.Equal(collectT, expectedProcess.Cwd, entity.Cwd)
			assert.Equal(collectT, expectedProcess.Exe, entity.Exe)
			assert.Equal(collectT, expectedProcess.Comm, entity.Comm)
			assert.Equal(collectT, expectedProcess.Cmdline, entity.Cmdline)
			assert.Equal(collectT, expectedProcess.Uids, entity.Uids)
			assert.Equal(collectT, expectedProcess.Gids, entity.Gids)
			assert.Equal(collectT, expectedProcess.ContainerID, entity.ContainerID)
			assert.Equal(collectT, expectedProcess.CreationTime, entity.CreationTime)
			assert.Equal(collectT, expectedProcess.Language, entity.Language)
			assert.Equal(collectT, expectedProcess.Owner, entity.Owner)
		}
	}, 1*time.Second, 100*time.Millisecond)
}
