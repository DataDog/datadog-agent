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
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
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
	infoEventQueueSize        atomic.Int64
	infoPodQueueSize          atomic.Int64
	infoProcessQueueBytes     atomic.Int64
	infoRTProcessQueueBytes   atomic.Int64
	infoConnectionsQueueBytes atomic.Int64
	infoEventQueueBytes       atomic.Int64
	infoPodQueueBytes         atomic.Int64
	infoEnabledChecks         []string
	infoDropCheckPayloads     []string

	// WorkloadMetaExtractor stats
	infoWlmExtractorCacheSize    atomic.Int64
	infoWlmExtractorStaleDiffs   atomic.Int64
	infoWlmExtractorDiffsDropped atomic.Int64
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
	EventQueueSize        int
	PodQueueSize          int
	ProcessQueueBytes     int64
	RtProcessQueueBytes   int64
	ConnectionsQueueBytes int64
	EventQueueBytes       int64
	PodQueueBytes         int64
}

//nolint:revive // TODO(PROC) Fix revive linter
func UpdateQueueStats(stats *QueueStats) {
	infoProcessQueueSize.Store(int64(stats.ProcessQueueSize))
	infoRTProcessQueueSize.Store(int64(stats.RtProcessQueueSize))
	infoConnectionsQueueSize.Store(int64(stats.ConnectionsQueueSize))
	infoEventQueueSize.Store(int64(stats.EventQueueSize))
	infoPodQueueSize.Store(int64(stats.PodQueueSize))
	infoProcessQueueBytes.Store(stats.ProcessQueueBytes)
	infoRTProcessQueueBytes.Store(stats.RtProcessQueueBytes)
	infoConnectionsQueueBytes.Store(stats.ConnectionsQueueBytes)
	infoEventQueueBytes.Store(stats.EventQueueBytes)
	infoPodQueueBytes.Store(stats.PodQueueBytes)
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

func publishEndpoints(eps []apicfg.Endpoint) func() interface{} {
	return func() interface{} {
		return getEndpointsInfo(eps)
	}
}

func getEndpointsInfo(eps []apicfg.Endpoint) interface{} {
	endpointsInfo := make(map[string][]string)

	// obfuscate the api keys
	for _, endpoint := range eps {
		apiKey := endpoint.APIKey
		if len(apiKey) > 5 {
			apiKey = apiKey[len(apiKey)-5:]
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

	return infoDropCheckPayloads
}

// InitExpvars initializes expvars
func InitExpvars(config ddconfig.Reader, hostname string, processModuleEnabled, languageDetectionEnabled bool, eps []apicfg.Endpoint) {
	infoOnce.Do(func() {
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
		processExpvars.Set("event_queue_size", publishInt(&infoEventQueueSize))
		processExpvars.Set("pod_queue_size", publishInt(&infoPodQueueSize))
		processExpvars.Set("process_queue_bytes", publishInt(&infoProcessQueueBytes))
		processExpvars.Set("rtprocess_queue_bytes", publishInt(&infoRTProcessQueueBytes))
		processExpvars.Set("connections_queue_bytes", publishInt(&infoConnectionsQueueBytes))
		processExpvars.Set("event_queue_bytes", publishInt(&infoEventQueueBytes))
		processExpvars.Set("pod_queue_bytes", publishInt(&infoPodQueueBytes))
		processExpvars.Set("container_id", expvar.Func(publishContainerID))
		processExpvars.Set("enabled_checks", expvar.Func(publishEnabledChecks))
		processExpvars.Set("endpoints", expvar.Func(publishEndpoints(eps)))
		processExpvars.Set("drop_check_payloads", expvar.Func(publishDropCheckPayloads))
		processExpvars.Set("system_probe_process_module_enabled", publishBool(processModuleEnabled))
		processExpvars.Set("language_detection_enabled", publishBool(languageDetectionEnabled))
		processExpvars.Set("workloadmeta_extractor_cache_size", publishInt(&infoWlmExtractorCacheSize))
		processExpvars.Set("workloadmeta_extractor_stale_diffs", publishInt(&infoWlmExtractorStaleDiffs))
		processExpvars.Set("workloadmeta_extractor_diffs_dropped", publishInt(&infoWlmExtractorDiffsDropped))
	})
}
