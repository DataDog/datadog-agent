// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"expvar"
	"strings"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

// Transport is the transport used by logs-agent, i.e TCP or HTTP
type Transport string

const (
	// TransportHTTP indicates logs-agent is using HTTP transport
	TransportHTTP Transport = "HTTP"
	// TransportTCP indicates logs-agent is using TCP transport
	TransportTCP Transport = "TCP"

	// StatusNotStarted means that the logs agent is not started
	StatusNotStarted = 0
	// StatusRunning means that the logs agent is running and fully operational
	StatusRunning = 1
)

var (
	// globalsLock prevents the builder, warnings and errors variables
	// (not objects behind them, they have their own locks) from data
	// races between reads in Add* and Get, and writes in Init and
	// Clear.
	globalsLock sync.RWMutex

	builder  *Builder
	warnings *config.Messages
	errors   *config.Messages

	// currentTransport is the current transport used by logs-agent, i.e TCP or HTTP
	currentTransport Transport
)

// Source provides some information about a logs source.
type Source struct {
	Type          string                 `json:"type"`
	Configuration map[string]interface{} `json:"configuration"`
	Status        string                 `json:"status"`
	Inputs        []string               `json:"inputs"`
	Messages      []string               `json:"messages"`
	Info          map[string][]string    `json:"info"`
}

// Tailer provides status information about a log tailer
type Tailer struct {
	ID   string              `json:"id"`
	Type string              `json:"type"`
	Info map[string][]string `json:"info"`
}

// Integration provides some information about a logs integration.
type Integration struct {
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
}

// ComponentUtilization holds the utilization and capacity snapshot for a single
// pipeline component instance.
type ComponentUtilization struct {
	Name     string `json:"name"`
	Instance string `json:"instance"`
	// Current 1-second window values
	AvgRatio      float64 `json:"avg_ratio"`       // N=30 EWMA
	RawRatio      float64 `json:"raw_ratio"`       // instantaneous 1-second sample
	ShortAvgRatio float64 `json:"short_avg_ratio"` // N=15 EWMA (~15-second window)
	AvgItems      float64 `json:"avg_items"`
	RawItems      int64   `json:"raw_items"`
	AvgBytes      float64 `json:"avg_bytes"`
	RawBytes      int64   `json:"raw_bytes"`
	// Rolling window statistics over the utilization ratio
	Avg5m  float64 `json:"avg_5m"`
	Max5m  float64 `json:"max_5m"`
	Avg30m float64 `json:"avg_30m"`
	Max30m float64 `json:"max_30m"`
	Max2h  float64 `json:"max_2h"`
	Max5h  float64 `json:"max_5h"`
	Max10h float64 `json:"max_10h"`
	// Saturation stats (threshold = 90% raw ratio)
	Saturated1mSeconds  int64  `json:"saturated_1m_s"`       // seconds spent saturated in last 1m
	Saturated30mSeconds int64  `json:"saturated_30m_s"`      // seconds spent saturated in last 30m
	LastSaturatedAt     string `json:"last_saturated_at"`    // "15:04:05" or ""
	HasLastSaturated    bool   `json:"has_last_saturated"`
}

// BackpressureStatus summarises the overall backpressure state of the logs pipeline.
type BackpressureStatus struct {
	// State is one of:
	//   "HEALTHY"   — no saturation in the last 30m
	//   "WARNING"   — no current saturation but at least one component was saturated in the last 30m
	//   "SATURATED" — at least one component was saturated within the last 1m
	State  string `json:"state"`
	Reason string `json:"reason"` // human-readable explanation; empty when HEALTHY
}

// Status provides some information about logs-agent.
type Status struct {
	IsRunning            bool                   `json:"is_running"`
	Endpoints            []string               `json:"endpoints"`
	StatusMetrics        map[string]string      `json:"metrics"`
	ProcessFileStats     map[string]uint64      `json:"process_file_stats"`
	Integrations         []Integration          `json:"integrations"`
	Tailers              []Tailer               `json:"tailers"`
	Errors               []string               `json:"errors"`
	Warnings             []string               `json:"warnings"`
	UseHTTP              bool                   `json:"use_http"`
	ComponentUtilization []ComponentUtilization `json:"component_utilization"`
	Backpressure         BackpressureStatus     `json:"backpressure"`
	// BackpressureTable is a pre-formatted human-readable backpressure table for
	// text output. It is omitted from JSON; use ComponentUtilization for structured access.
	BackpressureTable string `json:"-"`
}

// SetCurrentTransport sets the current transport used by the log agent.
func SetCurrentTransport(t Transport) {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	currentTransport = t
}

// GetCurrentTransport returns the current transport used by the log agent.
func GetCurrentTransport() Transport {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	return currentTransport
}

// Init instantiates the builder that builds the status on the fly.
func Init(isRunning *atomic.Uint32, endpoints *config.Endpoints, sources *sources.LogSources, tracker *tailers.TailerTracker, logExpVars *expvar.Map) {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	warnings = config.NewMessages()
	errors = config.NewMessages()
	builder = NewBuilder(isRunning, endpoints, sources, tracker, warnings, errors, logExpVars)
}

// Clear clears the status which means it needs to be initialized again to be used.
func Clear() {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	builder = nil
	warnings = nil
	errors = nil
	metrics.ClearComponentSnapshots()
}

// Get returns the status of the logs-agent computed on the fly.
func Get(verbose bool) Status {
	globalsLock.RLock()
	defer globalsLock.RUnlock()

	if builder == nil {
		return Status{
			IsRunning: false,
		}
	}
	return builder.BuildStatus(verbose)
}

// AddGlobalWarning keeps track of a warning message to display on the status.
func AddGlobalWarning(key string, warning string) {
	globalsLock.RLock()
	defer globalsLock.RUnlock()

	if warnings != nil {
		warnings.AddMessage(key, warning)
	}
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func RemoveGlobalWarning(key string) {
	globalsLock.RLock()
	defer globalsLock.RUnlock()

	if warnings != nil {
		warnings.RemoveMessage(key)
	}
}

// AddGlobalError an error message for the status display (errors will stop the agent)
func AddGlobalError(key string, errorMessage string) {
	globalsLock.RLock()
	defer globalsLock.RUnlock()

	if errors != nil {
		errors.AddMessage(key, errorMessage)
	}
}

func init() {
	metrics.LogsExpvars.Set("Errors", expvar.Func(func() interface{} {
		return strings.Join(Get(false).Errors, ", ")
	}))
	metrics.LogsExpvars.Set("Warnings", expvar.Func(func() interface{} {
		return strings.Join(Get(false).Warnings, ", ")
	}))
	metrics.LogsExpvars.Set("IsRunning", expvar.Func(func() interface{} {
		return Get(false).IsRunning
	}))
}
