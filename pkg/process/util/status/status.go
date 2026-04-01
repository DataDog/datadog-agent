// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package status

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	procstatus "github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// HostMeta holds the hostname reported by the process agent.
// We use a local struct here instead of hostMetadataUtils.Payload to avoid
// pulling in a CGo dependency (hostMetadataUtils → pkg/collector/python → rtloader).
// The template only consumes .core.metadata.meta.hostname, so this is sufficient.
type HostMeta struct {
	Hostname string `json:"hostname"`
}

// HostMetadata wraps HostMeta to match the JSON shape the template expects.
type HostMetadata struct {
	Meta *HostMeta `json:"meta"`
}

// CoreStatus holds core info about the process-agent
type CoreStatus struct {
	AgentVersion string       `json:"version"`
	GoVersion    string       `json:"go_version"`
	Arch         string       `json:"build_arch"`
	Config       ConfigStatus `json:"config"`
	Metadata     HostMetadata `json:"metadata"`
}

// ConfigStatus holds config settings from process-agent
type ConfigStatus struct {
	LogLevel string `json:"log_level"`
}

// InfoVersion holds information about process-agent version
type InfoVersion struct {
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
	GoVersion string
}

// MemInfo holds information about memory usage from process-agent
type MemInfo struct {
	Alloc uint64 `json:"alloc"`
}

type ExpvarsMap struct {
	Pid                             int                 `json:"pid"`
	Uptime                          int                 `json:"uptime"`
	UptimeNano                      float64             `json:"uptime_nano"`
	MemStats                        MemInfo             `json:"memstats"`
	Version                         InfoVersion         `json:"version"`
	DockerSocket                    string              `json:"docker_socket"`
	LastCollectTime                 string              `json:"last_collect_time"`
	ProcessCount                    int                 `json:"process_count"`
	ContainerCount                  int                 `json:"container_count"`
	ProcessQueueSize                int                 `json:"process_queue_size"`
	RTProcessQueueSize              int                 `json:"rtprocess_queue_size"`
	ConnectionsQueueSize            int                 `json:"connections_queue_size"`
	EventQueueSize                  int                 `json:"event_queue_size"`
	ProcessQueueBytes               int                 `json:"process_queue_bytes"`
	RTProcessQueueBytes             int                 `json:"rtprocess_queue_bytes"`
	ConnectionsQueueBytes           int                 `json:"connections_queue_bytes"`
	EventQueueBytes                 int                 `json:"event_queue_bytes"`
	ContainerID                     string              `json:"container_id"`
	ProxyURL                        string              `json:"proxy_url"`
	LogFile                         string              `json:"log_file"`
	EnabledChecks                   []string            `json:"enabled_checks"`
	Endpoints                       map[string][]string `json:"endpoints"`
	DropCheckPayloads               []string            `json:"drop_check_payloads"`
	SystemProbeProcessModuleEnabled bool                `json:"system_probe_process_module_enabled"`
	LanguageDetectionEnabled        bool                `json:"language_detection_enabled"`
	WlmExtractorCacheSize           int                 `json:"workloadmeta_extractor_cache_size"`
	WlmExtractorStaleDiffs          int                 `json:"workloadmeta_extractor_stale_diffs"`
	WlmExtractorDiffsDropped        int                 `json:"workloadmeta_extractor_diffs_dropped"`
	SubmissionErrorCount            int                 `json:"submission_error_count"`
}

// ProcessExpvars holds values fetched from the exp var server
type ProcessExpvars struct {
	ExpvarsMap ExpvarsMap `json:"process_agent"`
}

// Status holds runtime information from process-agent
type Status struct {
	Date    float64        `json:"date"`
	Core    CoreStatus     `json:"core"`    // Contains fields that are collected similarly to the core agent in pkg/status
	Expvars ProcessExpvars `json:"expvars"` // Contains the expvars retrieved from the process agent
}

// StatusOption is a function that acts on a Status object
type StatusOption func(s *Status)

// ConnectionError represents an error to connect to an HTTP server
type ConnectionError struct {
	error
}

// NewConnectionError returns a new ConnectionError
func NewConnectionError(err error) ConnectionError {
	return ConnectionError{err}
}

// OverrideTime overrides the Date from a Status object
func OverrideTime(t time.Time) StatusOption {
	return func(s *Status) {
		s.Date = float64(t.UnixNano())
	}
}

func getCoreStatus(coreConfig pkgconfigmodel.Reader, hostname string) (s CoreStatus) {
	return CoreStatus{
		AgentVersion: version.AgentVersion,
		GoVersion:    runtime.Version(),
		Arch:         runtime.GOARCH,
		Config: ConfigStatus{
			LogLevel: coreConfig.GetString("log_level"),
		},
		Metadata: HostMetadata{Meta: &HostMeta{Hostname: hostname}},
	}
}

func getExpvars(expVarURL string) (s ProcessExpvars, err error) {
	client := http.Client{}
	resp, err := client.Get(expVarURL)
	if err != nil {
		return s, ConnectionError{err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s, ConnectionError{err}
	}

	err = json.Unmarshal(body, &s)
	return
}

// GetInProcessStatus returns a Status object by reading process agent metrics
// directly from internal state, without going through expvar or HTTP.
// Config and hostname are read from the values stored by InitExpvars at startup,
// so the RAR gRPC adapter can call this as a pure bridge with no arguments.
func GetInProcessStatus() *Status {
	core := getCoreStatus(procstatus.GetConfig(), procstatus.GetHostname())
	m := procstatus.GetMetrics()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return &Status{
		Date: float64(time.Now().UnixNano()),
		Core: core,
		Expvars: ProcessExpvars{
			ExpvarsMap: ExpvarsMap{
				Pid:                             m.Pid,
				MemStats:                        MemInfo{Alloc: ms.Alloc},
				Uptime:                          m.Uptime,
				UptimeNano:                      float64(m.UptimeNano),
				DockerSocket:                    m.DockerSocket,
				LastCollectTime:                 m.LastCollectTime,
				ProcessCount:                    int(m.ProcessCount),
				ContainerCount:                  int(m.ContainerCount),
				ProcessQueueSize:                int(m.ProcessQueueSize),
				RTProcessQueueSize:              int(m.RTProcessQueueSize),
				ConnectionsQueueSize:            int(m.ConnectionsQueueSize),
				ProcessQueueBytes:               int(m.ProcessQueueBytes),
				RTProcessQueueBytes:             int(m.RTProcessQueueBytes),
				ConnectionsQueueBytes:           int(m.ConnectionsQueueBytes),
				ContainerID:                     m.ContainerID,
				EnabledChecks:                   m.EnabledChecks,
				Endpoints:                       m.Endpoints,
				DropCheckPayloads:               m.DropCheckPayloads,
				SystemProbeProcessModuleEnabled: m.SystemProbeProcessModuleEnabled,
				LanguageDetectionEnabled:        m.LanguageDetectionEnabled,
				WlmExtractorCacheSize:           int(m.WlmExtractorCacheSize),
				WlmExtractorStaleDiffs:          int(m.WlmExtractorStaleDiffs),
				WlmExtractorDiffsDropped:        int(m.WlmExtractorDiffsDropped),
				SubmissionErrorCount:            int(m.SubmissionErrorCount),
			},
		},
	}
}

// GetStatus returns a Status object with runtime information about process-agent
func GetStatus(coreConfig pkgconfigmodel.Reader, expVarURL string, hostname hostnameinterface.Component) (*Status, error) {
	hn, _ := hostname.Get(context.Background())
	coreStatus := getCoreStatus(coreConfig, hn)
	processExpVars, err := getExpvars(expVarURL)
	if err != nil {
		return nil, err
	}

	return &Status{
		Date:    float64(time.Now().UnixNano()),
		Core:    coreStatus,
		Expvars: processExpVars,
	}, nil
}
