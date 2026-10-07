// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status provides log agent status information
package status

import (
	"expvar"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/atomic"

	logsMetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/profilerec"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

// Builder is used to build the status.
type Builder struct {
	isRunning       *atomic.Uint32
	endpoints       *config.Endpoints
	sources         *sourcesPkg.LogSources
	tailers         *tailers.TailerTracker
	warnings        *config.Messages
	errors          *config.Messages
	logsExpVars     *expvar.Map
	pipelineMonitor logsMetrics.PipelineMonitor
	config          model.Reader
	loss            profilerec.LossWindow
}

// NewBuilder returns a new builder. pipelineMonitor and cfg may be nil (e.g. in
// tests), in which case the backpressure and performance-profile sections are empty.
func NewBuilder(isRunning *atomic.Uint32, endpoints *config.Endpoints, sources *sourcesPkg.LogSources, tracker *tailers.TailerTracker, warnings *config.Messages, errors *config.Messages, logExpVars *expvar.Map, pipelineMonitor logsMetrics.PipelineMonitor, cfg model.Reader) *Builder {
	return &Builder{
		isRunning:       isRunning,
		endpoints:       endpoints,
		sources:         sources,
		tailers:         tracker,
		warnings:        warnings,
		errors:          errors,
		logsExpVars:     logExpVars,
		pipelineMonitor: pipelineMonitor,
		config:          cfg,
	}
}

// getPerformanceProfile returns the active profile with each setting's effective
// value and source, or nil when no profile is active or no config is available.
func (b *Builder) getPerformanceProfile() *PerformanceProfile {
	if b.config == nil {
		return nil
	}
	name, version, settings, ok := pkgconfigsetup.ResolvedLogsPerformanceProfile(b.config)
	if !ok {
		return nil
	}
	pp := &PerformanceProfile{Name: name, Version: version}
	for _, s := range settings {
		pp.Settings = append(pp.Settings, PerformanceProfileSetting{
			Key:    s.Key,
			Value:  fmt.Sprintf("%v", b.config.Get(s.Key)),
			Source: string(b.config.GetSource(s.Key)),
		})
	}
	return pp
}

// BuildStatus returns the status of the logs-agent.
func (b *Builder) BuildStatus(verbose bool) Status {
	tailers := []Tailer{}
	if verbose {
		tailers = b.getTailers()
	}
	var snaps []logsMetrics.ComponentSnapshot
	if b.pipelineMonitor != nil {
		snaps = b.pipelineMonitor.Snapshots()
	}
	utils := b.getComponentUtilization(snaps)
	bp := b.getBackpressureStatus(snaps)
	profile := b.getPerformanceProfile()
	activeProfile := ""
	if profile != nil {
		activeProfile = profile.Name
	}
	counters := profilerec.ReadCounters(b.logsExpVars)
	droppedRecently, missedRecently, delivering := b.loss.Observe(counters, time.Now())
	return Status{
		IsRunning:             b.getIsRunning(),
		Endpoints:             b.getEndpoints(),
		Integrations:          b.getIntegrations(),
		Tailers:               tailers,
		StatusMetrics:         b.getMetricsStatus(),
		ProcessFileStats:      b.getProcessFileStats(),
		Warnings:              b.getWarnings(),
		Errors:                b.getErrors(),
		UseHTTP:               b.getUseHTTP(),
		ComponentUtilization:  utils,
		Backpressure:          bp,
		PerformanceProfile:    profile,
		ProfileRecommendation: b.getProfileRecommendation(utils, activeProfile, counters.SenderLatencyMs, droppedRecently, missedRecently, delivering),
		BackpressureTable:     b.formatBackpressureSection(utils, bp),
	}
}

// getComponentUtilization returns per-component snapshots sorted in pipeline order.
func (b *Builder) getComponentUtilization(snaps []logsMetrics.ComponentSnapshot) []ComponentUtilization {
	if snaps == nil {
		return nil
	}
	result := make([]ComponentUtilization, 0, len(snaps))
	for _, s := range snaps {
		// A capacity-only aggregation point ("sender", between the strategy and the workers)
		// has no utilization monitor, so it carries no signal for this table.
		if !s.Measured {
			continue
		}
		lastSat := ""
		if s.Windows.HasLastSaturated {
			lastSat = s.Windows.LastSaturatedAt.Local().Format("15:04:05")
		}
		result = append(result, ComponentUtilization{
			Name:                s.Name,
			Instance:            s.Instance,
			AvgRatio:            s.AvgRatio,
			RawRatio:            s.RawRatio,
			AvgItems:            s.AvgItems,
			RawItems:            s.RawItems,
			AvgBytes:            s.AvgBytes,
			RawBytes:            s.RawBytes,
			Avg5m:               s.Windows.Avg5m,
			Max5m:               s.Windows.Max5m,
			Avg30m:              s.Windows.Avg30m,
			Max30m:              s.Windows.Max30m,
			Max2h:               s.Windows.Max2h,
			Max5h:               s.Windows.Max5h,
			Max10h:              s.Windows.Max10h,
			Saturated1mSeconds:  int64(s.Windows.Saturated1m.Seconds()),
			Saturated30mSeconds: int64(s.Windows.Saturated30m.Seconds()),
			LastSaturatedAt:     lastSat,
			HasLastSaturated:    s.Windows.HasLastSaturated,
			CurrentlySaturated:  s.Windows.CurrentlySaturated,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		ri, rj := profilerec.ComponentRank(result[i].Name), profilerec.ComponentRank(result[j].Name)
		if ri != rj {
			return ri < rj
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Instance < result[j].Instance
	})
	return result
}

// getBackpressureStatus returns SATURATED (saturated in last 1m), WARNING (last 30m only), or HEALTHY.
func (b *Builder) getBackpressureStatus(snaps []logsMetrics.ComponentSnapshot) BackpressureStatus {
	// Unlike loss attribution, this ranks non-blocking destinations too: they drop payloads when saturated.
	state, bottleneck := logsMetrics.SelectBottleneck(logsMetrics.BackpressureComponents(snaps))
	if bottleneck == nil {
		return BackpressureStatus{State: state}
	}

	dur30m := fmtDuration(time.Duration(bottleneck.Saturated30mSeconds) * time.Second)
	// SATURATED means at or above threshold right now; it clears within seconds of recovery.
	if state == logsMetrics.BackpressureSaturated {
		return BackpressureStatus{
			State:     state,
			Reason:    fmt.Sprintf("%s pipeline %s is currently saturated (saturated for %s in the last 30m)", bottleneck.Component, bottleneck.Instance, dur30m),
			Component: bottleneck.Component,
		}
	}
	// WARNING: saturation occurred in the last 1m or 30m but nothing is currently at threshold.
	return BackpressureStatus{
		State:     state,
		Reason:    fmt.Sprintf("%s pipeline %s is not currently saturated but was saturated for %s in the last 30m", bottleneck.Component, bottleneck.Instance, dur30m),
		Component: bottleneck.Component,
	}
}

// getProfileRecommendation delegates to profilerec so agent status and Agent Health agree.
func (b *Builder) getProfileRecommendation(utils []ComponentUtilization, activeProfile string, latencyMs int64, droppedRecently, missedRecently, delivering bool) *ProfileRecommendation {
	stages := make([]profilerec.Stage, 0, len(utils))
	for _, u := range utils {
		stages = append(stages, profilerec.Stage{
			Name:                u.Name,
			CurrentlySaturated:  u.CurrentlySaturated,
			Saturated1mSeconds:  u.Saturated1mSeconds,
			Saturated30mSeconds: u.Saturated30mSeconds,
		})
	}
	rec := profilerec.Recommend(stages, activeProfile, profilerec.Signals{
		DroppedRecently: droppedRecently,
		MissedRecently:  missedRecently,
		Delivering:      delivering,
		SenderLatencyMs: latencyMs,
	})
	if rec == nil {
		return nil
	}
	return &ProfileRecommendation{Profile: rec.Profile, Reason: rec.Reason, ReasonCode: rec.ReasonCode, Bottleneck: rec.Bottleneck}
}

// formatBackpressureSection renders the backpressure section as preformatted text (omitted from JSON).
func (b *Builder) formatBackpressureSection(utils []ComponentUtilization, bp BackpressureStatus) string {
	if len(utils) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("  Logs Agent Backpressure\n")
	sb.WriteString("  =======================\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Overall state: %s\n", bp.State))
	if bp.Reason != "" {
		sb.WriteString(fmt.Sprintf("  Reason: %s\n", bp.Reason))
	}
	sb.WriteString("\n")

	// Size columns to the widest name/instance so the table stays aligned.
	nameW := len("Component")
	instW := len("Instance")
	for _, u := range utils {
		if len(u.Name) > nameW {
			nameW = len(u.Name)
		}
		if len(u.Instance) > instW {
			instW = len(u.Instance)
		}
	}
	rowFmt := fmt.Sprintf("  %%-%ds %%-%ds %%-9s %%-13s %%-14s %%-9s %%-9s %%-10s %%-16s %%s\n", nameW, instW)

	sb.WriteString(fmt.Sprintf(rowFmt,
		"Component", "Instance", "Current", "5m avg/max", "30m avg/max",
		"2h max", "5h max", "10h max", "30m saturated", "Last saturated"))

	for _, u := range utils {
		lastSat := u.LastSaturatedAt
		if !u.HasLastSaturated {
			lastSat = "-"
		}
		sb.WriteString(fmt.Sprintf(rowFmt,
			u.Name,
			u.Instance,
			bpPct(u.AvgRatio),
			bpPctRange(u.Avg5m, u.Max5m),
			bpPctRange(u.Avg30m, u.Max30m),
			bpPct(u.Max2h),
			bpPct(u.Max5h),
			bpPct(u.Max10h),
			fmtDuration(time.Duration(u.Saturated30mSeconds)*time.Second),
			lastSat,
		))
	}
	return sb.String()
}

func bpPct(v float64) string {
	return fmt.Sprintf("%d%%", int(math.Round(v*100)))
}

func bpPctRange(avg, max float64) string {
	return fmt.Sprintf("%d/%d%%", int(math.Round(avg*100)), int(math.Round(max*100)))
}

func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// getIsRunning returns true if the agent is running,
// this needs to be thread safe as it can be accessed
// from different commands (start, stop, status).
func (b *Builder) getIsRunning() bool {
	return b.isRunning.Load() == StatusRunning
}

func (b *Builder) getUseHTTP() bool {
	return b.endpoints.UseHTTP
}

func (b *Builder) getEndpoints() []string {
	return b.endpoints.GetStatus()
}

// getWarnings returns all the warning messages that
// have been accumulated during the life cycle of the logs-agent.
func (b *Builder) getWarnings() []string {
	return b.warnings.GetMessages()
}

// getErrors returns all the errors messages which are responsible
// for shutting down the logs-agent
func (b *Builder) getErrors() []string {
	return b.errors.GetMessages()
}

// getIntegrations returns all the information about the logs integrations.
func (b *Builder) getIntegrations() []Integration {
	var integrations []Integration
	for name, logSources := range b.groupSourcesByName() {
		var sources []Source
		for _, source := range logSources {
			sources = append(sources, Source{
				Type:          source.Config.Type,
				Configuration: b.configToDictionary(source),
				Status:        b.toString(source.Status()),
				Inputs:        source.GetInputs(),
				Messages:      source.Messages.GetMessages(),
				Info:          source.GetInfoStatus(),
			})
		}
		integrations = append(integrations, Integration{
			Name:    name,
			Sources: sources,
		})
	}
	return integrations
}

// getTailers returns all the information about the logs integrations.
func (b *Builder) getTailers() []Tailer {
	tailers := b.tailers.All()
	tailerStatus := make([]Tailer, 0, len(tailers))
	for _, tailer := range tailers {

		info := tailer.GetInfo().Rendered()

		tailerStatus = append(tailerStatus, Tailer{
			ID:   tailer.GetID(),
			Type: tailer.GetType(),
			Info: info,
		})
	}
	return tailerStatus
}

// groupSourcesByName groups all logs sources by name so that they get properly displayed
// on the agent status.
func (b *Builder) groupSourcesByName() map[string][]*sourcesPkg.LogSource {
	sources := make(map[string][]*sourcesPkg.LogSource)
	for _, source := range b.sources.GetSources() {
		if source.IsHiddenFromStatus() {
			continue
		}
		if _, exists := sources[source.Name]; !exists {
			sources[source.Name] = []*sourcesPkg.LogSource{}
		}
		sources[source.Name] = append(sources[source.Name], source)
	}
	return sources
}

// toString returns a representation of a status.
func (b *Builder) toString(status *status.LogStatus) string {
	var value string
	if status.IsPending() {
		value = "Pending"
	} else if status.IsSuccess() {
		value = "OK"
	} else if status.IsError() {
		value = status.GetError()
	}
	return value
}

// configToDictionary returns a representation of the source's configuration.
func (b *Builder) configToDictionary(source *sourcesPkg.LogSource) map[string]interface{} {
	c := source.Config
	dictionary := make(map[string]interface{})
	dictionary["Service"] = c.Service
	dictionary["Source"] = c.Source
	switch c.Type {
	case config.TCPType:
		dictionary["Port"] = c.Port
		if c.TLS != nil {
			dictionary["TLS"] = "true"
		}
		if c.Format != "" {
			dictionary["Format"] = c.Format
		}
		if len(c.AllowedIPs) > 0 {
			dictionary["AllowedIPs"] = strings.Join(c.AllowedIPs, ", ")
		}
		if len(c.DeniedIPs) > 0 {
			dictionary["DeniedIPs"] = strings.Join(c.DeniedIPs, ", ")
		}
	case config.UDPType:
		dictionary["Port"] = c.Port
		if c.Format != "" {
			dictionary["Format"] = c.Format
		}
		if len(c.AllowedIPs) > 0 {
			dictionary["AllowedIPs"] = strings.Join(c.AllowedIPs, ", ")
		}
		if len(c.DeniedIPs) > 0 {
			dictionary["DeniedIPs"] = strings.Join(c.DeniedIPs, ", ")
		}
	case config.FileType:
		dictionary["Path"] = c.Path
		dictionary["TailingMode"] = source.GetTailingMode()
		dictionary["Identifier"] = c.Identifier
		if c.Format != "" {
			dictionary["Format"] = c.Format
		}
	case config.DockerType:
		dictionary["Image"] = c.Image
		dictionary["Label"] = c.Label
		dictionary["Name"] = c.Name
	case config.JournaldType:
		dictionary["IncludeSystemUnits"] = strings.Join(c.IncludeSystemUnits, ", ")
		dictionary["ExcludeSystemUnits"] = strings.Join(c.ExcludeSystemUnits, ", ")
		dictionary["IncludeUserUnits"] = strings.Join(c.IncludeUserUnits, ", ")
		dictionary["ExcludeUserUnits"] = strings.Join(c.ExcludeUserUnits, ", ")
		dictionary["IncludeMatches"] = strings.Join(c.IncludeMatches, ", ")
		dictionary["ExcludeMatches"] = strings.Join(c.ExcludeMatches, ", ")
	case config.WindowsEventType:
		dictionary["ChannelPath"] = c.ChannelPath
		dictionary["Query"] = c.Query
	}
	for k, v := range dictionary {
		if v == "" {
			delete(dictionary, k)
		}
	}
	return dictionary
}

// getMetricsStatus exposes some aggregated metrics of the log agent on the agent status
func (b *Builder) getMetricsStatus() map[string]string {
	var metrics = make(map[string]string)
	counters := profilerec.ReadCounters(b.logsExpVars)
	metrics["LogsProcessed"] = strconv.FormatInt(b.logsExpVars.Get("LogsProcessed").(*expvar.Int).Value(), 10)
	metrics["LogsSent"] = strconv.FormatInt(b.logsExpVars.Get("LogsSent").(*expvar.Int).Value(), 10)
	metrics["LogsDropped"] = strconv.FormatInt(counters.Dropped, 10)
	metrics["BytesMissed"] = strconv.FormatInt(counters.Missed, 10)
	metrics["BytesSent"] = strconv.FormatInt(b.logsExpVars.Get("BytesSent").(*expvar.Int).Value(), 10)
	metrics["RetryCount"] = strconv.FormatInt(b.logsExpVars.Get("RetryCount").(*expvar.Int).Value(), 10)
	metrics["RetryTimeSpent"] = time.Duration(b.logsExpVars.Get("RetryTimeSpent").(*expvar.Int).Value()).String()
	metrics["EncodedBytesSent"] = strconv.FormatInt(b.logsExpVars.Get("EncodedBytesSent").(*expvar.Int).Value(), 10)
	metrics["LogsTruncated"] = strconv.FormatInt(b.logsExpVars.Get("LogsTruncated").(*expvar.Int).Value(), 10)
	metrics["SenderLatency"] = time.Duration(counters.SenderLatencyMs * int64(time.Millisecond)).String()
	return metrics
}

func (b *Builder) getProcessFileStats() map[string]uint64 {
	stats := make(map[string]uint64)
	fs, err := procfilestats.GetProcessFileStats()
	if err != nil {
		return stats
	}

	stats["CoreAgentProcessOpenFiles"] = fs.AgentOpenFiles
	stats["OSFileLimit"] = fs.OsFileLimit
	return stats
}
