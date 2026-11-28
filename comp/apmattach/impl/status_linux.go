// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"embed"
	"io"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

const statusKey = "apmAttach"

type processStatus struct {
	PID           int    `json:"pid"`
	StartTime     uint64 `json:"start_time"`
	Exe           string `json:"exe"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	Detail        string `json:"detail"`
	Rule          string `json:"rule"`
	Layer         string `json:"layer"`
	Service       string `json:"service"`
	TracerVersion string `json:"tracer_version"`
	DurationMs    int64  `json:"duration_ms"`
	Failures      int    `json:"failures"`
	LastAttempt   string `json:"last_attempt"`
	NextRetry     string `json:"next_retry,omitempty"`
}

type attachStatus struct {
	Enabled           bool            `json:"enabled"`
	InjectorPath      string          `json:"injector_path"`
	InjectorAvailable bool            `json:"injector_available"`
	RunningAsRoot     bool            `json:"running_as_root"`
	ScanInterval      string          `json:"scan_interval"`
	LastScan          string          `json:"last_scan"`
	LastPolicyChange  string          `json:"last_policy_change"`
	LastError         string          `json:"last_error"`
	Processes         []processStatus `json:"processes"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (a *attacher) statusSnapshot() attachStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := attachStatus{
		Enabled:           a.enabled,
		InjectorPath:      a.injectorPath,
		InjectorAvailable: a.injectorAvailable,
		RunningAsRoot:     a.euid == 0,
		ScanInterval:      a.scanInterval.String(),
		LastScan:          formatTime(a.lastScan),
		LastPolicyChange:  formatTime(a.lastPolicyChange),
		LastError:         a.lastError,
		Processes:         make([]processStatus, 0, len(a.states)),
	}
	for _, st := range a.states {
		p := processStatus{
			PID:           st.key.pid,
			StartTime:     st.key.startTime,
			Exe:           st.exe,
			Decision:      st.result.Decision,
			Reason:        st.result.Reason,
			Detail:        st.result.Detail,
			Rule:          st.result.Rule,
			Layer:         st.result.Layer,
			Service:       st.result.Service,
			TracerVersion: st.result.TracerVersion,
			DurationMs:    st.result.DurationMs,
			Failures:      st.consecutiveFailures,
			LastAttempt:   formatTime(st.lastAttempt),
		}
		if st.result.Decision == decisionFailed && st.consecutiveFailures < maxAttempts {
			p.NextRetry = formatTime(st.nextRetry)
		}
		s.Processes = append(s.Processes, p)
	}
	sort.Slice(s.Processes, func(i, j int) bool { return s.Processes[i].PID < s.Processes[j].PID })
	return s
}

// statusProvider renders the "APM Attach" section of the Agent status.
type statusProvider struct {
	a *attacher
}

func (p statusProvider) Name() string { return "APM Attach" }

func (p statusProvider) Section() string { return "APM Attach" }

func (p statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats[statusKey] = p.a.statusSnapshot()
	return nil
}

func (p statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "apmattach.tmpl", buffer, p.data())
}

func (p statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "apmattachHTML.tmpl", buffer, p.data())
}

func (p statusProvider) data() map[string]interface{} {
	return map[string]interface{}{statusKey: p.a.statusSnapshot()}
}
