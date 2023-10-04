// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file provides methods to set various expvar values, which are then queried by the `status` command.

package status

import (
	"bufio"
	"expvar"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
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

func InitExpvars(config ddconfig.ConfigReader, telemetry telemetry.Component, hostname string, processModuleEnabled, languageDetectionEnabled bool, eps []apicfg.Endpoint) {
	infoOnce.Do(func() {
		expvar.NewString("host").Set(hostname)
		expvar.NewInt("pid").Set(int64(os.Getpid()))
		expvar.Publish("uptime", expvar.Func(publishUptime))
		expvar.Publish("uptime_nano", expvar.Func(publishUptimeNano))
		expvar.Publish("version", expvar.Func(publishVersion))
		expvar.Publish("docker_socket", expvar.Func(publishDockerSocket))
		expvar.Publish("last_collect_time", expvar.Func(publishLastCollectTime))
		expvar.Publish("process_count", publishInt(&infoProcCount))
		expvar.Publish("container_count", publishInt(&infoContainerCount))
		expvar.Publish("process_queue_size", publishInt(&infoProcessQueueSize))
		expvar.Publish("rtprocess_queue_size", publishInt(&infoRTProcessQueueSize))
		expvar.Publish("connections_queue_size", publishInt(&infoConnectionsQueueSize))
		expvar.Publish("event_queue_size", publishInt(&infoEventQueueSize))
		expvar.Publish("pod_queue_size", publishInt(&infoPodQueueSize))
		expvar.Publish("process_queue_bytes", publishInt(&infoProcessQueueBytes))
		expvar.Publish("rtprocess_queue_bytes", publishInt(&infoRTProcessQueueBytes))
		expvar.Publish("connections_queue_bytes", publishInt(&infoConnectionsQueueBytes))
		expvar.Publish("event_queue_bytes", publishInt(&infoEventQueueBytes))
		expvar.Publish("pod_queue_bytes", publishInt(&infoPodQueueBytes))
		expvar.Publish("container_id", expvar.Func(publishContainerID))
		expvar.Publish("enabled_checks", expvar.Func(publishEnabledChecks))
		expvar.Publish("endpoints", expvar.Func(publishEndpoints(eps)))
		expvar.Publish("drop_check_payloads", expvar.Func(publishDropCheckPayloads))
		expvar.Publish("system_probe_process_module_enabled", publishBool(processModuleEnabled))
		expvar.Publish("language_detection_enabled", publishBool(languageDetectionEnabled))
		expvar.Publish("workloadmeta_extractor_cache_size", publishInt(&infoWlmExtractorCacheSize))
		expvar.Publish("workloadmeta_extractor_stale_diffs", publishInt(&infoWlmExtractorStaleDiffs))
		expvar.Publish("workloadmeta_extractor_diffs_dropped", publishInt(&infoWlmExtractorDiffsDropped))
	})

	// Run a profile & telemetry server.
	if config.GetBool("telemetry.enabled") {
		http.Handle("/telemetry", telemetry.Handler())
	}
}
