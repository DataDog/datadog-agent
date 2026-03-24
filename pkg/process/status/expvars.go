// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file provides methods to set various expvar values, which are then queried by the `status` command.

//nolint:revive // TODO(PROC) Fix revive linter
package status

import (
	"bufio"
	"expvar"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	infoMutex                 sync.RWMutex
	infoOnce                  sync.Once
	infoStart                 = time.Now()
	infoDockerSocket          string
	infoLastCollectTime       string
	infoProcCount             atomic.Int64
	infoContainerCount        atomic.Int64
	infoProcessQueueSize      atomic.Int64
	infoRTProcessQueueSize    atomic.Int64
	infoConnectionsQueueSize  atomic.Int64
	infoProcessQueueBytes     atomic.Int64
	infoRTProcessQueueBytes   atomic.Int64
	infoConnectionsQueueBytes atomic.Int64
	infoSubmissionErrorCount  atomic.Int64
	infoEnabledChecks         []string
	infoDropCheckPayloads     []string

	// WorkloadMetaExtractor stats
	infoWlmExtractorCacheSize    atomic.Int64
	infoWlmExtractorStaleDiffs   atomic.Int64
	infoWlmExtractorDiffsDropped atomic.Int64

	// Stored at InitExpvars time for direct access without going through expvar.
	infoProcessModuleEnabled     bool
	infoLanguageDetectionEnabled bool
	infoConfig                   config.Component
)

func publishUptime() interface{} {
	return int(time.Since(infoStart) / time.Second)
}

func publishUptimeNano() interface{} {
	return infoStart.UnixNano()
}

func publishVersion() interface{} {
	agentVersion, _ := version.Agent()
	return agentVersion
}

func publishDockerSocket() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoDockerSocket
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateDockerSocket(path string) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoDockerSocket = path
}

func publishLastCollectTime() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoLastCollectTime
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateLastCollectTime(t time.Time) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoLastCollectTime = t.Format("2006-01-02 15:04:05")
}

func AddSubmissionErrorCount(errors int64) {
	infoSubmissionErrorCount.Add(errors)
}

func publishInt(i *atomic.Int64) expvar.Func {
	return func() any {
		return i.Load()
	}
}

func publishBool(v bool) expvar.Func {
	return func() any {
		return v
	}
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateProcContainerCount(msgs []model.MessageBody) {
	var procCount, containerCount int
	for _, m := range msgs {
		switch msg := m.(type) {
		case *model.CollectorContainer:
			containerCount += len(msg.Containers)
		case *model.CollectorProc:
			procCount += len(msg.Processes)
			containerCount += len(msg.Containers)
		}
	}

	infoProcCount.Store(int64(procCount))
	infoContainerCount.Store(int64(containerCount))
}

//nolint:revive // TODO(PROC) Fix revive linter
type QueueStats struct {
	ProcessQueueSize      int
	RtProcessQueueSize    int
	ConnectionsQueueSize  int
	ProcessQueueBytes     int64
	RtProcessQueueBytes   int64
	ConnectionsQueueBytes int64
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateQueueStats(stats *QueueStats) {
	infoProcessQueueSize.Store(int64(stats.ProcessQueueSize))
	infoRTProcessQueueSize.Store(int64(stats.RtProcessQueueSize))
	infoConnectionsQueueSize.Store(int64(stats.ConnectionsQueueSize))
	infoProcessQueueBytes.Store(stats.ProcessQueueBytes)
	infoRTProcessQueueBytes.Store(stats.RtProcessQueueBytes)
	infoConnectionsQueueBytes.Store(stats.ConnectionsQueueBytes)
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateEnabledChecks(enabledChecks []string) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoEnabledChecks = enabledChecks
}

// WlmExtractorStats are stats from the WorkloadMetaExtractor
type WlmExtractorStats struct {
	CacheSize    int
	StaleDiffs   int64
	DiffsDropped int64
}

// UpdateWlmExtractorStats updates the expvar stats for the WorkloadMetaExtractor
func UpdateWlmExtractorStats(stats WlmExtractorStats) {
	infoWlmExtractorCacheSize.Store(int64(stats.CacheSize))
	infoWlmExtractorStaleDiffs.Store(stats.StaleDiffs)
	infoWlmExtractorDiffsDropped.Store(stats.DiffsDropped)
}

func publishEnabledChecks() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoEnabledChecks
}

func publishContainerID() interface{} {
	cgroupFile := "/proc/self/cgroup"
	if !filesystem.FileExists(cgroupFile) {
		return nil
	}
	f, err := os.Open(cgroupFile)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	// the content of the file should have "docker/" on each line, the last
	// bit of each line after the "/" should be the container id, e.g.
	//
	// 	11:name=systemd:/docker/49de419da182a44f29659b9761a963543cdbf1dee8b51313b9104edec4461c58
	//
	// we could just extract that and treat it as current container id
	containerID := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "docker/") {
			slices := strings.Split(line, "/")
			// it's not totally safe to assume the format, but it's the only thing we can do for now
			if len(slices) == 3 {
				containerID = slices[len(slices)-1]
				break
			}
		}
	}
	return containerID
}

func publishEndpoints(config config.Component) func() interface{} {
	return func() interface{} {
		return getEndpointsInfo(config)
	}
}

func getEndpointsInfo(config config.Component) interface{} {
	endpointsInfo := make(map[string][]string)

	eps, _ := endpoint.GetAPIEndpoints(config)

	// obfuscate the api keys
	for _, endpoint := range eps {
		apiKey := endpoint.APIKey
		if len(apiKey) > 4 {
			apiKey = apiKey[len(apiKey)-4:]
		}

		endpointsInfo[endpoint.Endpoint.String()] = append(endpointsInfo[endpoint.Endpoint.String()], apiKey)
	}
	return endpointsInfo
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateDropCheckPayloads(drops []string) {
	infoMutex.RLock()
	defer infoMutex.RUnlock()

	infoDropCheckPayloads = make([]string, len(drops))
	copy(infoDropCheckPayloads, drops)
}

func publishDropCheckPayloads() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()

	return slices.Clone(infoDropCheckPayloads)
}

// InitExpvars initializes expvars
func InitExpvars(cfg config.Component, hostname string, processModuleEnabled, languageDetectionEnabled bool, eps []apicfg.Endpoint) {
	// Store for direct access via GetMetrics(), independent of expvar initialization.
	infoProcessModuleEnabled = processModuleEnabled
	infoLanguageDetectionEnabled = languageDetectionEnabled
	infoConfig = cfg
	infoOnce.Do(func() {
		config := cfg
		processExpvars := expvar.NewMap("process_agent")
		hostString := expvar.NewString("host")
		hostString.Set(hostname)
		processExpvars.Set("host", hostString)
		pid := expvar.NewInt("pid")
		pid.Set(int64(os.Getpid()))
		processExpvars.Set("pid", pid)
		processExpvars.Set("uptime", expvar.Func(publishUptime))
		processExpvars.Set("uptime_nano", expvar.Func(publishUptimeNano))
		processExpvars.Set("version", expvar.Func(publishVersion))
		processExpvars.Set("docker_socket", expvar.Func(publishDockerSocket))
		processExpvars.Set("last_collect_time", expvar.Func(publishLastCollectTime))
		processExpvars.Set("process_count", publishInt(&infoProcCount))
		processExpvars.Set("container_count", publishInt(&infoContainerCount))
		processExpvars.Set("process_queue_size", publishInt(&infoProcessQueueSize))
		processExpvars.Set("rtprocess_queue_size", publishInt(&infoRTProcessQueueSize))
		processExpvars.Set("connections_queue_size", publishInt(&infoConnectionsQueueSize))
		processExpvars.Set("process_queue_bytes", publishInt(&infoProcessQueueBytes))
		processExpvars.Set("rtprocess_queue_bytes", publishInt(&infoRTProcessQueueBytes))
		processExpvars.Set("connections_queue_bytes", publishInt(&infoConnectionsQueueBytes))
		processExpvars.Set("container_id", expvar.Func(publishContainerID))
		processExpvars.Set("enabled_checks", expvar.Func(publishEnabledChecks))
		processExpvars.Set("endpoints", expvar.Func(publishEndpoints(config)))
		processExpvars.Set("drop_check_payloads", expvar.Func(publishDropCheckPayloads))
		processExpvars.Set("system_probe_process_module_enabled", publishBool(processModuleEnabled))
		processExpvars.Set("language_detection_enabled", publishBool(languageDetectionEnabled))
		processExpvars.Set("workloadmeta_extractor_cache_size", publishInt(&infoWlmExtractorCacheSize))
		processExpvars.Set("workloadmeta_extractor_stale_diffs", publishInt(&infoWlmExtractorStaleDiffs))
		processExpvars.Set("workloadmeta_extractor_diffs_dropped", publishInt(&infoWlmExtractorDiffsDropped))
		processExpvars.Set("submission_error_count", publishInt(&infoSubmissionErrorCount))
	})
}

// Metrics holds process agent runtime metrics read directly from internal state.
// Used by GetMetrics() for the RAR gRPC path, bypassing expvar.
type Metrics struct {
	Pid                             int
	Uptime                          int
	UptimeNano                      int64
	DockerSocket                    string
	LastCollectTime                 string
	ProcessCount                    int64
	ContainerCount                  int64
	ProcessQueueSize                int64
	RTProcessQueueSize              int64
	ConnectionsQueueSize            int64
	ProcessQueueBytes               int64
	RTProcessQueueBytes             int64
	ConnectionsQueueBytes           int64
	ContainerID                     string
	EnabledChecks                   []string
	Endpoints                       map[string][]string
	DropCheckPayloads               []string
	SystemProbeProcessModuleEnabled bool
	LanguageDetectionEnabled        bool
	WlmExtractorCacheSize           int64
	WlmExtractorStaleDiffs          int64
	WlmExtractorDiffsDropped        int64
	SubmissionErrorCount            int64
}

// GetMetrics reads process agent runtime metrics directly from internal state,
// bypassing expvar. This is the data source for the RAR gRPC status path.
func GetMetrics() Metrics {
	infoMutex.RLock()
	dockerSocket := infoDockerSocket
	lastCollectTime := infoLastCollectTime
	enabledChecks := slices.Clone(infoEnabledChecks)
	dropCheckPayloads := slices.Clone(infoDropCheckPayloads)
	infoMutex.RUnlock()

	containerID := ""
	if v := publishContainerID(); v != nil {
		if s, ok := v.(string); ok {
			containerID = s
		}
	}

	var endpoints map[string][]string
	if infoConfig != nil {
		endpoints = getEndpointsInfo(infoConfig).(map[string][]string)
	}

	return Metrics{
		Pid:                             os.Getpid(),
		Uptime:                          int(time.Since(infoStart) / time.Second),
		UptimeNano:                      infoStart.UnixNano(),
		DockerSocket:                    dockerSocket,
		LastCollectTime:                 lastCollectTime,
		ProcessCount:                    infoProcCount.Load(),
		ContainerCount:                  infoContainerCount.Load(),
		ProcessQueueSize:                infoProcessQueueSize.Load(),
		RTProcessQueueSize:              infoRTProcessQueueSize.Load(),
		ConnectionsQueueSize:            infoConnectionsQueueSize.Load(),
		ProcessQueueBytes:               infoProcessQueueBytes.Load(),
		RTProcessQueueBytes:             infoRTProcessQueueBytes.Load(),
		ConnectionsQueueBytes:           infoConnectionsQueueBytes.Load(),
		ContainerID:                     containerID,
		EnabledChecks:                   enabledChecks,
		Endpoints:                       endpoints,
		DropCheckPayloads:               dropCheckPayloads,
		SystemProbeProcessModuleEnabled: infoProcessModuleEnabled,
		LanguageDetectionEnabled:        infoLanguageDetectionEnabled,
		WlmExtractorCacheSize:           infoWlmExtractorCacheSize.Load(),
		WlmExtractorStaleDiffs:          infoWlmExtractorStaleDiffs.Load(),
		WlmExtractorDiffsDropped:        infoWlmExtractorDiffsDropped.Load(),
		SubmissionErrorCount:            infoSubmissionErrorCount.Load(),
	}
}
