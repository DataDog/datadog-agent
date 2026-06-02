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
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

// Builder is used to build the status.
type Builder struct {
	isRunning   *atomic.Uint32
	endpoints   *config.Endpoints
	sources     *sourcesPkg.LogSources
	tailers     *tailers.TailerTracker
	warnings    *config.Messages
	errors      *config.Messages
	logsExpVars *expvar.Map
}

// NewBuilder returns a new builder.
func NewBuilder(isRunning *atomic.Uint32, endpoints *config.Endpoints, sources *sourcesPkg.LogSources, tracker *tailers.TailerTracker, warnings *config.Messages, errors *config.Messages, logExpVars *expvar.Map) *Builder {
	return &Builder{
		isRunning:   isRunning,
		endpoints:   endpoints,
		sources:     sources,
		tailers:     tracker,
		warnings:    warnings,
		errors:      errors,
		logsExpVars: logExpVars,
	}
}

// BuildStatus returns the status of the logs-agent.
func (b *Builder) BuildStatus(verbose bool) Status {
	tailers := []Tailer{}
	if verbose {
		tailers = b.getTailers()
	}
	utils := b.getComponentUtilization()
	bp := b.getBackpressureStatus(utils)
	return Status{
		IsRunning:            b.getIsRunning(),
		Endpoints:            b.getEndpoints(),
		Integrations:         b.getIntegrations(),
		Tailers:              tailers,
		StatusMetrics:        b.getMetricsStatus(),
		ProcessFileStats:     b.getProcessFileStats(),
		Warnings:             b.getWarnings(),
		Errors:               b.getErrors(),
		UseHTTP:              b.getUseHTTP(),
		ComponentUtilization: utils,
		Backpressure:         bp,
		BackpressureTable:    b.formatBackpressureSection(utils, bp),
	}
}

// componentSortOrder defines the canonical display order for pipeline components.
var componentSortOrder = map[string]int{
	"processor": 0,
	"strategy":  1,
	"sender":    2,
	"worker":    3,
}

func componentRank(name string) int {
	if r, ok := componentSortOrder[name]; ok {
		return r
	}
	return 10 // destination_* and anything else comes last
}

// getComponentUtilization returns the latest per-component utilization snapshots,
// sorted in pipeline order (processor → strategy → sender → worker → destination_*).
func (b *Builder) getComponentUtilization() []ComponentUtilization {
	snaps := logsMetrics.GlobalComponentSnapshots()
	result := make([]ComponentUtilization, 0, len(snaps))
	for _, s := range snaps {
		lastSat := ""
		if s.Windows.HasLastSaturated {
			lastSat = s.Windows.LastSaturatedAt.Local().Format("15:04:05")
		}
		result = append(result, ComponentUtilization{
			Name:                s.Name,
			Instance:            s.Instance,
			AvgRatio:            s.AvgRatio,
			RawRatio:            s.RawRatio,
			ShortAvgRatio:       s.ShortAvgRatio,
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
			Saturated30mSeconds: int64(s.Windows.Saturated30m.Seconds()),
			LastSaturatedAt:     lastSat,
			HasLastSaturated:    s.Windows.HasLastSaturated,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		ri, rj := componentRank(result[i].Name), componentRank(result[j].Name)
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

// getBackpressureStatus computes the overall pipeline backpressure state.
func (b *Builder) getBackpressureStatus(utils []ComponentUtilization) BackpressureStatus {
	var maxSat int64
	var satName, satInst string
	for _, u := range utils {
		if u.Saturated30mSeconds > maxSat {
			maxSat = u.Saturated30mSeconds
			satName = u.Name
			satInst = u.Instance
		}
	}
	if maxSat > 0 {
		dur := time.Duration(maxSat) * time.Second
		return BackpressureStatus{
			State:  "SATURATED",
			Reason: fmt.Sprintf("%s pipeline %s was saturated for %s in the last 30m", satName, satInst, fmtDuration(dur)),
		}
	}
	return BackpressureStatus{State: "HEALTHY"}
}

// formatBackpressureSection renders the full backpressure section as a pre-formatted string
// for direct emission in text status output. Omitted from JSON (BackpressureTable has json:"-").
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
	// Header
	sb.WriteString(fmt.Sprintf("  %-12s %-10s %-9s %-13s %-14s %-9s %-9s %-10s %-16s %s\n",
		"Component", "Instance", "Current", "5m avg/max", "30m avg/max",
		"2h max", "5h max", "10h max", "30m saturated", "Last saturated"))
	// Rows
	for _, u := range utils {
		lastSat := u.LastSaturatedAt
		if !u.HasLastSaturated {
			lastSat = "-"
		}
		sb.WriteString(fmt.Sprintf("  %-12s %-10s %-9s %-13s %-14s %-9s %-9s %-10s %-16s %s\n",
			u.Name,
			u.Instance,
			bpPct(u.RawRatio),
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
				Configuration: b.toDictionary(source.Config),
				Status:        b.toString(source.Status),
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

// toDictionary returns a representation of the configuration.
func (b *Builder) toDictionary(c *config.LogsConfig) map[string]interface{} {
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
		dictionary["TailingMode"] = c.TailingMode
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
	metrics["LogsProcessed"] = strconv.FormatInt(b.logsExpVars.Get("LogsProcessed").(*expvar.Int).Value(), 10)
	metrics["LogsSent"] = strconv.FormatInt(b.logsExpVars.Get("LogsSent").(*expvar.Int).Value(), 10)
	metrics["BytesSent"] = strconv.FormatInt(b.logsExpVars.Get("BytesSent").(*expvar.Int).Value(), 10)
	metrics["RetryCount"] = strconv.FormatInt(b.logsExpVars.Get("RetryCount").(*expvar.Int).Value(), 10)
	metrics["RetryTimeSpent"] = time.Duration(b.logsExpVars.Get("RetryTimeSpent").(*expvar.Int).Value()).String()
	metrics["EncodedBytesSent"] = strconv.FormatInt(b.logsExpVars.Get("EncodedBytesSent").(*expvar.Int).Value(), 10)
	metrics["LogsTruncated"] = strconv.FormatInt(b.logsExpVars.Get("LogsTruncated").(*expvar.Int).Value(), 10)
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
