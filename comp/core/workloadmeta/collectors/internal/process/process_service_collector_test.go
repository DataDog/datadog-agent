// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package process

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
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
	pidInjectedOnly   = 111 // Process with injection but no service data
	pidGPUOnly        = 222 // Process using GPU but not a service
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

	newPids, heartbeatPids, pidsToService := c.collector.filterPidsToRequest(alivePids, procs)
	pids := append(newPids, heartbeatPids...)

	// Verify categorization
	require.Len(t, newPids, 1, "Should have 1 new PID")
	require.Contains(t, newPids, int32(pidNewService))

	require.Len(t, heartbeatPids, 1, "Should have 1 heartbeat PID")
	require.Contains(t, heartbeatPids, int32(pidStaleService))

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

	sysConfigOverrides := map[string]interface{}{
		"discovery.enabled": true,
	}
	languagePython := &languagemodels.Language{
		Name: languagemodels.Python,
	}

	tests := []struct {
		name                     string
		shouldError              bool
		httpResponse             *model.ServicesResponse
		ignoredPids              []int32
		processesToCollect       map[int32]*procutil.Process
		containerMapping         map[int]string
		existingProcesses        []*workloadmeta.Process
		expectStored             []*workloadmeta.Process
		pidHeartbeats            map[int32]time.Time
		expectNoEntities         []int32
		knownInjectionStatusPids []int32 // PIDs whose injection status was already reported in a previous cycle
	}{
		{
			name: "new service discovered",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{makeModelService(pidNewService, "new-service")},
				InjectedPIDs: []int{pidNewService},
			},
			expectStored: []*workloadmeta.Process{makeProcessEntityWithService(pidNewService, baseTime.Add(-2*time.Minute), languagePython, "new-service", workloadmeta.InjectionInjected, "")},
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
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidIgnoredService, "ignored-service")},
			},
		},
		{
			name: "fresh vs stale services",
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntityWithService(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, "fresh-existing", workloadmeta.InjectionInjected, ""), // Previously injected
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-existing", workloadmeta.InjectionNotInjected, ""),
			},
			processesToCollect: map[int32]*procutil.Process{
				pidFreshService: makeProcess(pidFreshService, baseTime.Add(-5*time.Minute).UnixMilli(), nil),
				pidStaleService: makeProcess(pidStaleService, baseTime.Add(-20*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{
					makeModelService(pidStaleService, "stale-existing"),
				},
				// Note: No InjectedPIDs here - simulates that injection status is not re-detected on heartbeats
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityWithService(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, "fresh-existing", workloadmeta.InjectionInjected, ""), // Should preserve injection status
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-existing", workloadmeta.InjectionNotInjected, ""),
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
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidRecentService, "recent-service")},
			},
			expectNoEntities: []int32{pidRecentService}, // Process should exist but have no service data
		},
		{
			name: "injected process",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{},    // No services detected
				InjectedPIDs: []int{pidNewService}, // But process is injected
			},
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidNewService, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, "")}, // Process with injection status but no service
		},
		{
			name: "not_injected_no_service",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{}, // No service detected
				InjectedPIDs: []int{},           // Not injected
			},
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidNewService, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionNotInjected, "")},
		},
		{
			name: "preserve injection state",
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, ""), // Already reported in previous cycle
			},
			knownInjectionStatusPids: []int32{pidInjectedOnly}, // We already reported this PID's injection status
			processesToCollect: map[int32]*procutil.Process{
				pidInjectedOnly: makeProcess(pidInjectedOnly, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{},      // Still no service
				InjectedPIDs: []int{pidInjectedOnly}, // Same injection state as before
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, ""), // Injection state preserved, no duplicate entity
			},
		},
		{
			name: "injected_death_cleanup",
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, ""), // Pre-existing injected-only process
			},
			processesToCollect: map[int32]*procutil.Process{
				// Process is no longer alive
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{},
				InjectedPIDs: []int{},
			},
			expectStored: []*workloadmeta.Process{},
		},
		{
			name: "service with container",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			containerMapping: map[int]string{
				int(pidNewService): "container_abc123",
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidNewService, "new-service")},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityWithService(pidNewService, baseTime.Add(-2*time.Minute), languagePython, "new-service", workloadmeta.InjectionNotInjected, "container_abc123"),
			},
		},
		{
			name: "containerized services",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService:   makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
				pidStaleService: makeProcess(pidStaleService, baseTime.Add(-20*time.Minute).UnixMilli(), nil),
			},
			containerMapping: map[int]string{
				int(pidNewService): "container_1",
				// pidStaleService has no container
			},
			pidHeartbeats: map[int32]time.Time{
				pidStaleService: baseTime.Add(-20 * time.Minute),
			},
			existingProcesses: []*workloadmeta.Process{
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-existing", workloadmeta.InjectionNotInjected, ""),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{
					makeModelService(pidNewService, "new-service"),
					makeModelService(pidStaleService, "stale-existing"),
				},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityWithService(pidNewService, baseTime.Add(-2*time.Minute), languagePython, "new-service", workloadmeta.InjectionNotInjected, "container_1"),
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-existing", workloadmeta.InjectionNotInjected, ""),
			},
		},
		{
			name: "injected with container",
			processesToCollect: map[int32]*procutil.Process{
				pidInjectedOnly: makeProcess(pidInjectedOnly, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			containerMapping: map[int]string{
				int(pidInjectedOnly): "container_injected",
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{},
				InjectedPIDs: []int{pidInjectedOnly},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, "container_injected"),
			},
		},
		{
			name: "gpu only process",
			processesToCollect: map[int32]*procutil.Process{
				pidGPUOnly: makeProcess(pidGPUOnly, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{},
				GPUPIDs:  []int{pidGPUOnly},
			},
			expectStored: func() []*workloadmeta.Process {
				e := makeProcessEntity(pidGPUOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionNotInjected, "")
				e.HasNvidiaGPU = true
				return []*workloadmeta.Process{e}
			}(),
		},
		{
			name: "service with gpu",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{func() model.Service {
					s := makeModelService(pidNewService, "gpu-service")
					s.HasNvidiaGPU = true
					return s
				}()},
			},
			expectStored: func() []*workloadmeta.Process {
				e := makeProcessEntity(pidNewService, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionNotInjected, "")
				e.Service = makeProcessEntityService(pidNewService, "gpu-service", workloadmeta.InjectionNotInjected).Service
				e.HasNvidiaGPU = true
				return []*workloadmeta.Process{e}
			}(),
		},
		{
			name: "gpu status preserved across heartbeat",
			existingProcesses: func() []*workloadmeta.Process {
				e := makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), nil, "stale-gpu-service", workloadmeta.InjectionNotInjected, "")
				e.HasNvidiaGPU = true
				return []*workloadmeta.Process{e}
			}(),
			processesToCollect: map[int32]*procutil.Process{
				pidStaleService: makeProcess(pidStaleService, baseTime.Add(-20*time.Minute).UnixMilli(), nil),
			},
			pidHeartbeats: map[int32]time.Time{
				pidStaleService: baseTime.Add(-20 * time.Minute),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidStaleService, "stale-gpu-service")},
			},
			expectStored: func() []*workloadmeta.Process {
				e := makeProcessEntity(pidStaleService, baseTime.Add(-20*time.Minute), nil, workloadmeta.InjectionNotInjected, "")
				e.Service = makeProcessEntityService(pidStaleService, "stale-gpu-service", workloadmeta.InjectionNotInjected).Service
				e.HasNvidiaGPU = true
				return []*workloadmeta.Process{e}
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetWithoutSource("process_config.process_collection.enabled", false)
			cfg.SetWithoutSource("language_detection.enabled", false)

			c := setUpCollectorTest(t, cfg, sysConfigOverrides, nil)
			defer c.cleanup()
			ctx := t.Context()

			socketPath, _ := startTestServer(t, tc.httpResponse, tc.shouldError)
			c.collector.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))

			for _, pid := range tc.ignoredPids {
				c.collector.ignoredPids.Add(pid)
			}

			c.collector.lastCollectedProcesses = make(map[int32]*procutil.Process)

			for _, process := range tc.existingProcesses {
				// we use notify instead of set here because we want to control the source as it impacts how data is merged/stored in wlm
				c.mockStore.Notify([]workloadmeta.CollectorEvent{
					{
						Type:   workloadmeta.EventTypeSet,
						Source: workloadmeta.SourceServiceDiscovery,
						Entity: process,
					},
				})

				c.collector.lastCollectedProcesses[process.Pid] = &procutil.Process{
					Pid:     process.Pid,
					Cmdline: []string{"python3", "--version"},
					Stats:   &procutil.Stats{CreateTime: process.CreationTime.UnixMilli()},
				}
			}

			c.mockClock.Set(baseTime)

			if tc.pidHeartbeats != nil {
				c.collector.pidHeartbeats = tc.pidHeartbeats
			}

			for _, pid := range tc.knownInjectionStatusPids {
				c.collector.knownInjectionStatusPids.Add(pid)
			}

			// Mock processProbe.ProcessesByPID to be called directly by collectServicesDefault
			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Maybe()
			containerMapping := tc.containerMapping
			if containerMapping == nil {
				containerMapping = map[int]string{}
			}
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(containerMapping).AnyTimes()

			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

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

	sysConfigOverrides := map[string]interface{}{
		"discovery.enabled": true,
	}

	languagePython := &languagemodels.Language{
		Name: languagemodels.Python,
	}

	tests := []struct {
		name                     string
		shouldError              bool
		httpResponse             *model.ServicesResponse
		ignoredPids              []int32
		existingProcessData      []*workloadmeta.Process
		existingServiceData      []*workloadmeta.Process
		expectStored             []*workloadmeta.Process
		pidHeartbeats            map[int32]time.Time
		processesToCollect       map[int32]*procutil.Process
		knownInjectionStatusPids []int32 // PIDs whose injection status was already reported in a previous cycle
	}{
		{
			name: "new service discovered and stored",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), languagePython),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidNewService, "new-service")},
			},
			expectStored: []*workloadmeta.Process{makeProcessEntityWithService(pidNewService, baseTime.Add(-2*time.Minute), languagePython, "new-service", workloadmeta.InjectionNotInjected, "")},
		},
		{
			name: "http error handled gracefully",
			processesToCollect: map[int32]*procutil.Process{
				pidNewService: makeProcess(pidNewService, baseTime.Add(-2*time.Minute).UnixMilli(), languagePython),
			},
			shouldError: true,
			// expectStored should have no service data should be stored when HTTP error occurs
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidNewService, baseTime.Add(-2*time.Minute), languagePython, workloadmeta.InjectionUnknown, "")},
		},
		{
			name: "ignored pid is skipped",
			processesToCollect: map[int32]*procutil.Process{
				pidIgnoredService: makeProcess(pidIgnoredService, baseTime.Add(-2*time.Minute).UnixMilli(), languagePython),
			},
			ignoredPids: []int32{pidIgnoredService},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidIgnoredService, "ignored-service")},
			},
			// Process should exist but have no service data
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidIgnoredService, baseTime.Add(-2*time.Minute), languagePython, workloadmeta.InjectionUnknown, "")},
		},
		{
			name: "fresh service not updated, stale service updated",
			existingProcessData: []*workloadmeta.Process{
				makeProcessEntity(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, workloadmeta.InjectionNotInjected, ""),  // Recent
				makeProcessEntity(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, workloadmeta.InjectionNotInjected, ""), // Stale (> 15min)
			},
			existingServiceData: []*workloadmeta.Process{
				makeProcessEntityService(pidFreshService, "fresh-existing", workloadmeta.InjectionNotInjected), // Recent
				makeProcessEntityService(pidStaleService, "stale-existing", workloadmeta.InjectionNotInjected), // Stale (> 15min)
			},
			processesToCollect: map[int32]*procutil.Process{
				pidFreshService: makeProcess(pidFreshService, baseTime.Add(-5*time.Minute).UnixMilli(), languagePython),
				pidStaleService: makeProcess(pidStaleService, baseTime.Add(-20*time.Minute).UnixMilli(), languagePython),
			},
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{
					makeModelService(pidStaleService, "stale-existing"), // Only stale service should be requested
				},
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntityWithService(pidFreshService, baseTime.Add(-5*time.Minute), languagePython, "fresh-existing", workloadmeta.InjectionNotInjected, ""),
				makeProcessEntityWithService(pidStaleService, baseTime.Add(-20*time.Minute), languagePython, "stale-existing", workloadmeta.InjectionNotInjected, ""),
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
			httpResponse: &model.ServicesResponse{
				Services: []model.Service{makeModelService(pidRecentService, "recent-service")},
			},
			// Process should exist but have no service data
			expectStored: []*workloadmeta.Process{makeProcessEntity(pidRecentService, baseTime.Add(time.Minute+30*time.Second), languagePython, workloadmeta.InjectionUnknown, "")},
		},
		{
			name: "preserve injection state",
			existingServiceData: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, ""), // Already reported in previous cycle
			},
			knownInjectionStatusPids: []int32{pidInjectedOnly}, // We already reported this PID's injection status
			processesToCollect: map[int32]*procutil.Process{
				pidInjectedOnly: makeProcess(pidInjectedOnly, baseTime.Add(-2*time.Minute).UnixMilli(), nil),
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{},      // Still no service
				InjectedPIDs: []int{pidInjectedOnly}, // Same injection state as before
			},
			expectStored: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, ""), // Injection state preserved, no duplicate entity
			},
		},
		{
			name: "injected_death_cleanup",
			existingServiceData: []*workloadmeta.Process{
				makeProcessEntity(pidInjectedOnly, baseTime.Add(-2*time.Minute), nil, workloadmeta.InjectionInjected, ""), // Pre-existing injected-only process
			},
			processesToCollect: map[int32]*procutil.Process{
				// Process is NOT in processesToCollect = it's dead/no longer alive
			},
			httpResponse: &model.ServicesResponse{
				Services:     []model.Service{}, // No services
				InjectedPIDs: []int{},           // No longer injected (process is dead)
			},
			expectStored: []*workloadmeta.Process{
				// Should be empty - the injected-only process should be deleted
			},
			// Note: injected-only processes are NOT in pidHeartbeats (no service data)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetWithoutSource("process_config.process_collection.enabled", true)
			cfg.SetWithoutSource("language_detection.enabled", true)
			// setting process collection interval to the same as the service collection interval
			// because it makes the test simpler until the service collection interval is configurable
			cfg.SetWithoutSource("process_config.intervals.process", collectionIntervalSeconds)

			// Collector setup
			c := setUpCollectorTest(t, cfg, sysConfigOverrides, nil)
			defer c.cleanup()
			ctx := t.Context()

			// Create test server & override collector client
			socketPath, _ := startTestServer(t, tc.httpResponse, tc.shouldError)
			c.collector.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))

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
			c.collector.lastCollectedProcesses = make(map[int32]*procutil.Process)

			for _, process := range tc.existingServiceData {
				// we use notify instead of set here because we want to control the source as it impacts how data is merged/stored in wlm
				c.mockStore.Notify([]workloadmeta.CollectorEvent{
					{
						Type:   workloadmeta.EventTypeSet,
						Source: workloadmeta.SourceServiceDiscovery,
						Entity: process,
					},
				})

				c.collector.lastCollectedProcesses[process.Pid] = &procutil.Process{
					Pid:     process.Pid,
					Cmdline: []string{"python3", "--version"},
					Stats:   &procutil.Stats{CreateTime: process.CreationTime.UnixMilli()},
				}

				// If this is a process whose injection status we've reported (but has no service), add to tracking
				if process.Service == nil {
					c.collector.knownInjectionStatusPids.Add(process.Pid)
				}
			}

			// Set mock clock to baseTime to control LastHeartbeat in tests
			c.mockClock.Set(baseTime)

			// Pre-populate pidHeartbeats cache if specified in test case
			if tc.pidHeartbeats != nil {
				c.collector.pidHeartbeats = tc.pidHeartbeats
			}

			for _, pid := range tc.knownInjectionStatusPids {
				c.collector.knownInjectionStatusPids.Add(pid)
			}

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Maybe()
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(map[int]string{}).AnyTimes()

			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

			// Trigger service collection (service collection waits for first tick)
			c.mockClock.Add(collectionInterval)

			// Wait for processes and service data to be stored
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

	sysConfigOverrides := map[string]interface{}{
		"discovery.enabled": true,
	}

	cfg := config.NewMock(t)
	cfg.SetWithoutSource("process_config.process_collection.enabled", true)
	cfg.SetWithoutSource("language_detection.enabled", true)
	// setting process collection interval to the same as the service collection interval
	// because it makes the test simpler until the service collection interval is configurable
	cfg.SetWithoutSource("process_config.intervals.process", collectionIntervalSeconds)

	c := setUpCollectorTest(t, cfg, sysConfigOverrides, nil)
	ctx := t.Context()

	// Set initial state: process entity in the store, SD was tracking a service,
	// the process collector reported no live processes.
	existingProcess := makeProcessEntityService(pidFreshService, "existing-service", workloadmeta.InjectionNotInjected)
	c.mockStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceServiceDiscovery,
			Entity: existingProcess,
		},
	})
	c.collector.lastCollectedProcesses = make(map[int32]*procutil.Process)
	c.collector.pidHeartbeats[pidFreshService] = baseTime

	socketPath, _ := startTestServer(t, &model.ServicesResponse{}, false)
	c.collector.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))
	c.mockClock.Set(baseTime)

	c.collector.store = c.mockStore

	c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(nil, nil).Times(2)
	c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(2)

	err := c.collector.Start(ctx, c.mockStore)
	assert.NoError(t, err)

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
func startTestServer(t *testing.T, response *model.ServicesResponse, shouldError bool) (string, *httptest.Server) {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle CheckClient's startup check
		if r.URL.Path == "/debug/stats" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}

		if r.URL.Path != "/discovery/services" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if shouldError {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		// Parse request to identify heartbeat PIDs
		var params core.Params
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &params)
		}

		// For heartbeat PIDs, return only dynamic fields
		modifiedResponse := *response
		for i := range modifiedResponse.Services {
			for _, hbPid := range params.HeartbeatPids {
				if modifiedResponse.Services[i].PID == int(hbPid) {
					// Keep only dynamic fields for heartbeat
					modifiedResponse.Services[i] = model.Service{
						PID:      modifiedResponse.Services[i].PID,
						TCPPorts: modifiedResponse.Services[i].TCPPorts,
						UDPPorts: modifiedResponse.Services[i].UDPPorts,
						LogFiles: modifiedResponse.Services[i].LogFiles,
					}
					break
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		responseBytes, _ := json.Marshal(modifiedResponse)
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
		TCPPorts:           []uint16{3000, 4000},
		APMInstrumentation: true,
		Language:           "python",
		Type:               "database",
		LogFiles:           []string{"/var/log/" + name + ".log"},
		UST: model.UST{
			Service: "dd-model-" + name,
			Env:     "production",
			Version: "1.2.3",
		},
	}
}

func makeProcessEntityService(pid int32, name string, injectionState workloadmeta.InjectionState) *workloadmeta.Process {
	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(pid)),
		},
		Pid:            pid,
		InjectionState: injectionState,
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
			TCPPorts:           []uint16{3000, 4000},
			APMInstrumentation: true,
			Type:               "database",
			LogFiles:           []string{"/var/log/" + name + ".log"},
			UST: workloadmeta.UST{
				Service: "dd-model-" + name,
				Env:     "production",
				Version: "1.2.3",
			},
		},
	}
}

func makeProcessEntity(pid int32, createTime time.Time, language *languagemodels.Language, injectionState workloadmeta.InjectionState, containerID string) *workloadmeta.Process {
	proc := makeProcess(pid, createTime.UnixMilli(), language)

	var owner *workloadmeta.EntityID
	if containerID != "" {
		owner = &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		}
	}

	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(pid)),
		},
		CreationTime:   time.UnixMilli(proc.Stats.CreateTime).UTC(),
		Pid:            proc.Pid,
		Ppid:           proc.Ppid,
		NsPid:          proc.NsPid,
		Name:           proc.Name,
		Cwd:            proc.Cwd,
		Exe:            proc.Exe,
		Comm:           proc.Comm,
		Cmdline:        proc.Cmdline,
		Language:       proc.Language,
		Uids:           proc.Uids,
		Gids:           proc.Gids,
		InjectionState: injectionState,
		ContainerID:    containerID,
		Owner:          owner,
	}
}

func makeProcessEntityWithService(pid int32, createTime time.Time, language *languagemodels.Language, name string, injectionState workloadmeta.InjectionState, containerID string) *workloadmeta.Process {
	process := makeProcessEntity(pid, createTime, language, injectionState, containerID)
	process.Service = makeProcessEntityService(pid, name, injectionState).Service
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
			require.NoError(collectT, err)
			require.NotNil(collectT, entity)
			assert.Equal(collectT, expectedProcess.HasNvidiaGPU, entity.HasNvidiaGPU)
			if expectedProcess.Service == nil {
				assert.Nil(collectT, entity.Service)
			} else {
				require.NotNil(collectT, entity.Service)
				// Verify all service fields match expected values
				assert.Equal(collectT, expectedProcess.Service.GeneratedName, entity.Service.GeneratedName)
				assert.Equal(collectT, expectedProcess.Service.GeneratedNameSource, entity.Service.GeneratedNameSource)
				assert.Equal(collectT, expectedProcess.Service.AdditionalGeneratedNames, entity.Service.AdditionalGeneratedNames)
				assert.Equal(collectT, expectedProcess.Service.TracerMetadata, entity.Service.TracerMetadata)
				assert.Equal(collectT, expectedProcess.Service.TCPPorts, entity.Service.TCPPorts)
				assert.Equal(collectT, expectedProcess.Service.UDPPorts, entity.Service.UDPPorts)
				assert.Equal(collectT, expectedProcess.Service.APMInstrumentation, entity.Service.APMInstrumentation)
				assert.Equal(collectT, expectedProcess.Service.Type, entity.Service.Type)
				assert.Equal(collectT, expectedProcess.Service.LogFiles, entity.Service.LogFiles)
				assert.Equal(collectT, expectedProcess.Service.UST, entity.Service.UST)
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
			assert.Equal(collectT, expectedProcess.InjectionState, entity.InjectionState)
			assert.Equal(collectT, expectedProcess.HasNvidiaGPU, entity.HasNvidiaGPU)
		}
	}, 1*time.Second, 100*time.Millisecond)
}

func TestConvertModelServiceToService_Normalization(t *testing.T) {
	tests := []struct {
		name                    string
		inputService            *model.Service
		expectedGeneratedName   string
		expectedAdditionalNames []string
	}{
		{
			name: "normalize service name",
			inputService: &model.Service{
				GeneratedName:            "My@service_12🤪",
				GeneratedNameSource:      "env",
				AdditionalGeneratedNames: []string{"@foo", "def", "ABC", "service.name"},
				Language:                 "java",
			},
			expectedGeneratedName:   "my_service_12",
			expectedAdditionalNames: []string{"_foo", "abc", "def", "service.name"},
		},
		{
			name: "fallback service name",
			inputService: &model.Service{
				GeneratedName:            "",
				GeneratedNameSource:      "env",
				AdditionalGeneratedNames: []string{},
				Language:                 "jvm",
			},
			expectedGeneratedName:   "unnamed-jvm-service",
			expectedAdditionalNames: []string{},
		},
		{
			name: "fallback service name with unknown language",
			inputService: &model.Service{
				GeneratedName:            "",
				GeneratedNameSource:      "env",
				AdditionalGeneratedNames: []string{},
				Language:                 string(language.Unknown),
			},
			expectedGeneratedName:   "unnamed-service",
			expectedAdditionalNames: []string{},
		},
		{
			name: "filter empty additional names",
			inputService: &model.Service{
				GeneratedName:            "service",
				GeneratedNameSource:      "env",
				AdditionalGeneratedNames: []string{"", "  ", "valid"},
				Language:                 "node",
			},
			expectedGeneratedName:   "service",
			expectedAdditionalNames: []string{"valid"},
		},
		{
			name: "empty additional names list",
			inputService: &model.Service{
				GeneratedName:            "service",
				GeneratedNameSource:      "env",
				AdditionalGeneratedNames: []string{},
				Language:                 "ruby",
			},
			expectedGeneratedName:   "service",
			expectedAdditionalNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertModelServiceToService(tt.inputService)
			assert.Equal(t, tt.expectedGeneratedName, result.GeneratedName)
			assert.Equal(t, tt.expectedAdditionalNames, result.AdditionalGeneratedNames)
		})
	}
}

func TestTracerAlreadyCollectsLogs(t *testing.T) {
	tests := []struct {
		name             string
		inputService     *model.Service
		expectedLogFiles []string
	}{
		{
			name: "logs not collected by tracer passes log files through",
			inputService: &model.Service{
				GeneratedName: "my-service",
				Language:      "python",
				LogFiles:      []string{"/var/log/app.log", "/tmp/debug.log"},
				TracerMetadata: []tracermetadata.TracerMetadata{
					{TracerLanguage: "python", LogsCollected: false},
				},
			},
			expectedLogFiles: []string{"/var/log/app.log", "/tmp/debug.log"},
		},
		{
			name: "logs collected by tracer filters out log files",
			inputService: &model.Service{
				GeneratedName: "my-service",
				Language:      "python",
				LogFiles:      []string{"/var/log/app.log", "/tmp/debug.log"},
				TracerMetadata: []tracermetadata.TracerMetadata{
					{TracerLanguage: "python", LogsCollected: true},
				},
			},
			expectedLogFiles: nil,
		},
		{
			name: "no tracer metadata passes log files through",
			inputService: &model.Service{
				GeneratedName: "my-service",
				Language:      "python",
				LogFiles:      []string{"/var/log/app.log"},
			},
			expectedLogFiles: []string{"/var/log/app.log"},
		},
		{
			name: "logs collected by one of multiple tracers filters out log files",
			inputService: &model.Service{
				GeneratedName: "my-service",
				Language:      "python",
				LogFiles:      []string{"/var/log/app.log"},
				TracerMetadata: []tracermetadata.TracerMetadata{
					{TracerLanguage: "python", LogsCollected: false},
					{TracerLanguage: "java", LogsCollected: true},
				},
			},
			expectedLogFiles: nil,
		},
		{
			name: "no log files with tracer collecting logs",
			inputService: &model.Service{
				GeneratedName: "my-service",
				Language:      "python",
				TracerMetadata: []tracermetadata.TracerMetadata{
					{TracerLanguage: "python", LogsCollected: true},
				},
			},
			expectedLogFiles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertModelServiceToService(tt.inputService)
			assert.Equal(t, tt.expectedLogFiles, result.LogFiles)
		})
	}
}
