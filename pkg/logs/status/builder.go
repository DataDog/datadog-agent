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
	"sync"
	"time"

	"go.uber.org/atomic"

	logsMetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	loss            lossWindow
}

// lossRecencyWindow is how long after the last observed loss the pipeline is
// still treated as "actively losing logs" for recommendation purposes.
const lossRecencyWindow = 5 * time.Minute

// lossWindow turns the monotonic logs-dropped / bytes-missed / processed / sent
// counters into recent-activity signals. Each observe records when a counter
// last increased, so a stale historical loss ages out instead of keeping a
// recommendation pinned on, and delivery is judged on the latest interval rather
// than lifetime totals.
type lossWindow struct {
	mu                      sync.Mutex
	seeded                  bool
	lastDropped, lastMissed int64
	lastProcessed, lastSent int64
	droppedAt, missedAt     time.Time
}

// observe records the current counter totals at time now and reports whether
// each kind of loss occurred within lossRecencyWindow, plus whether the intake
// is currently delivering. The first call only seeds the baseline (no history to
// compare against yet), reporting no loss and assuming delivery.
//
// delivering is false only when logs advanced through processing in the latest
// interval but none were sent — a currently rejecting or unreachable intake.
// Lifetime totals are deliberately not used: an outage after a period of
// successful delivery leaves LogsSent > 0 forever, which would hide the outage.
func (w *lossWindow) observe(dropped, missed, processed, sent int64, now time.Time) (droppedRecently, missedRecently, delivering bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.seeded {
		w.seeded = true
		w.lastDropped, w.lastMissed = dropped, missed
		w.lastProcessed, w.lastSent = processed, sent
		return false, false, true
	}
	if dropped > w.lastDropped {
		w.droppedAt = now
	}
	if missed > w.lastMissed {
		w.missedAt = now
	}
	delivering = !(processed > w.lastProcessed && sent == w.lastSent)
	w.lastDropped, w.lastMissed = dropped, missed
	w.lastProcessed, w.lastSent = processed, sent
	droppedRecently = !w.droppedAt.IsZero() && now.Sub(w.droppedAt) <= lossRecencyWindow
	missedRecently = !w.missedAt.IsZero() && now.Sub(w.missedAt) <= lossRecencyWindow
	return droppedRecently, missedRecently, delivering
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
	utils := b.getComponentUtilization()
	bp := b.getBackpressureStatus(utils)
	profile := b.getPerformanceProfile()
	activeProfile := ""
	if profile != nil {
		activeProfile = profile.Name
	}
	droppedRecently, missedRecently, delivering := b.loss.observe(b.logsDropped(), b.bytesMissed(), b.logsProcessed(), b.logsSent(), time.Now())
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
		ProfileRecommendation: b.getProfileRecommendation(utils, activeProfile, b.senderLatencyMs(), droppedRecently, missedRecently, delivering),
		BackpressureTable:     b.formatBackpressureSection(utils, bp),
	}
}

// componentSortOrder defines the canonical display order for pipeline components.
var componentSortOrder = map[string]int{
	"processor": 0,
	"strategy":  1,
	"worker":    2,
}

func componentRank(name string) int {
	if r, ok := componentSortOrder[name]; ok {
		return r
	}
	return 10 // destination_* and anything else comes last
}

// getComponentUtilization returns per-component snapshots sorted in pipeline order.
func (b *Builder) getComponentUtilization() []ComponentUtilization {
	if b.pipelineMonitor == nil {
		return nil
	}
	snaps := b.pipelineMonitor.Snapshots()
	result := make([]ComponentUtilization, 0, len(snaps))
	for _, s := range snaps {
		// "sender" is a capacity-only aggregation point (items/bytes between the strategy and the
		// workers) with no utilization monitor, so its ratio/saturation is always 0. It carries no
		// signal for the backpressure table, so omit it.
		if s.Name == logsMetrics.SenderTlmName {
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

// getBackpressureStatus returns SATURATED (saturated in last 1m), WARNING (last 30m only), or HEALTHY.
func (b *Builder) getBackpressureStatus(utils []ComponentUtilization) BackpressureStatus {
	// SATURATED signal: among currently-saturated components, surface the one with the highest EWMA.
	var hasCurrSat bool
	var maxCurrRatio float64
	var currSatName, currSatInst string
	var currSat30m int64

	// WARNING signal: the component with the most recent 1m/30m saturation.
	var maxSat1m, maxSat30m int64
	var sat30mForMaxSat1m int64
	var satName1m, satInst1m string
	var satName30m, satInst30m string

	for _, u := range utils {
		if u.CurrentlySaturated && u.AvgRatio > maxCurrRatio {
			hasCurrSat = true
			maxCurrRatio = u.AvgRatio
			currSatName = u.Name
			currSatInst = u.Instance
			currSat30m = u.Saturated30mSeconds
		}
		if u.Saturated1mSeconds > maxSat1m {
			maxSat1m = u.Saturated1mSeconds
			sat30mForMaxSat1m = u.Saturated30mSeconds
			satName1m = u.Name
			satInst1m = u.Instance
		}
		if u.Saturated30mSeconds > maxSat30m {
			maxSat30m = u.Saturated30mSeconds
			satName30m = u.Name
			satInst30m = u.Instance
		}
	}

	// SATURATED: a component is at or above threshold right now. Clears within seconds of recovery.
	if hasCurrSat {
		dur30m := time.Duration(currSat30m) * time.Second
		return BackpressureStatus{
			State:     "SATURATED",
			Reason:    fmt.Sprintf("%s pipeline %s is currently saturated (saturated for %s in the last 30m)", currSatName, currSatInst, fmtDuration(dur30m)),
			Component: currSatName,
		}
	}
	// WARNING: saturation occurred in the last 1m or 30m but no component is currently at threshold.
	if maxSat1m > 0 {
		dur30m := time.Duration(sat30mForMaxSat1m) * time.Second
		return BackpressureStatus{
			State:     "WARNING",
			Reason:    fmt.Sprintf("%s pipeline %s is not currently saturated but was saturated for %s in the last 30m", satName1m, satInst1m, fmtDuration(dur30m)),
			Component: satName1m,
		}
	}
	if maxSat30m > 0 {
		dur30m := time.Duration(maxSat30m) * time.Second
		return BackpressureStatus{
			State:     "WARNING",
			Reason:    fmt.Sprintf("%s pipeline %s is not currently saturated but was saturated for %s in the last 30m", satName30m, satInst30m, fmtDuration(dur30m)),
			Component: satName30m,
		}
	}
	return BackpressureStatus{State: "HEALTHY"}
}

// Profiles recommended for specific bottleneck classes. These names must exist
// in the pkg/config/setup catalog (guarded by a unit test).
const (
	profileHighThroughput  = "high-throughput"
	profileHighConcurrency = "high-concurrency"
)

// senderLatencyHighThresholdMs is the intake round-trip latency (ms) above which
// a send bottleneck is treated as latency-bound. Heuristic; tunable.
const senderLatencyHighThresholdMs = 250

// senderLatencyMs returns the most recent HTTP sender latency to the intake in
// milliseconds, or 0 when unavailable.
func (b *Builder) senderLatencyMs() int64 {
	if b.logsExpVars == nil {
		return 0
	}
	if v, ok := b.logsExpVars.Get("SenderLatency").(*expvar.Int); ok && v != nil {
		return v.Value()
	}
	return 0
}

// bytesMissed returns total bytes lost before consumption, e.g. a file rotating
// before the tailer drained it (read-side loss), or 0.
func (b *Builder) bytesMissed() int64 {
	if b.logsExpVars == nil {
		return 0
	}
	if v, ok := b.logsExpVars.Get("BytesMissed").(*expvar.Int); ok && v != nil {
		return v.Value()
	}
	return 0
}

// logsDropped returns total logs dropped across all destinations (send-side loss), or 0.
func (b *Builder) logsDropped() int64 {
	if b.logsExpVars == nil {
		return 0
	}
	m, ok := b.logsExpVars.Get("DestinationLogsDropped").(*expvar.Map)
	if !ok || m == nil {
		return 0
	}
	var total int64
	m.Do(func(kv expvar.KeyValue) {
		if v, ok := kv.Value.(*expvar.Int); ok && v != nil {
			total += v.Value()
		}
	})
	return total
}

// logsProcessed returns the total number of logs that have entered the pipeline, or 0.
func (b *Builder) logsProcessed() int64 {
	if b.logsExpVars == nil {
		return 0
	}
	if v, ok := b.logsExpVars.Get("LogsProcessed").(*expvar.Int); ok && v != nil {
		return v.Value()
	}
	return 0
}

// logsSent returns the total number of logs successfully sent to the intake, or 0.
func (b *Builder) logsSent() int64 {
	if b.logsExpVars == nil {
		return 0
	}
	if v, ok := b.logsExpVars.Get("LogsSent").(*expvar.Int); ok && v != nil {
		return v.Value()
	}
	return 0
}

// isSendStage reports whether the component is part of the network send/transport
// stage (the worker pool, the sender aggregation point, or a destination).
func isSendStage(component string) bool {
	return component == "worker" || component == logsMetrics.SenderTlmName || strings.HasPrefix(component, "destination_")
}

// bottleneckComponent returns the most-downstream currently-saturated stage,
// falling back to recent (1m/30m) saturation, or "" if none.
func (b *Builder) bottleneckComponent(utils []ComponentUtilization) string {
	if c := mostDownstreamSaturated(utils, func(u ComponentUtilization) bool { return u.CurrentlySaturated }); c != "" {
		return c
	}
	return mostDownstreamSaturated(utils, func(u ComponentUtilization) bool {
		return u.Saturated1mSeconds > 0 || u.Saturated30mSeconds > 0
	})
}

// mostDownstreamSaturated returns the deepest component for which sat() is true,
// or "". Backpressure propagates upstream, so the deepest saturated stage is the
// true bottleneck; the rest are propagation victims.
func mostDownstreamSaturated(utils []ComponentUtilization, sat func(ComponentUtilization) bool) string {
	best := ""
	bestRank := -1
	for _, u := range utils {
		if !sat(u) {
			continue
		}
		if r := componentRank(u.Name); r > bestRank {
			bestRank = r
			best = u.Name
		}
	}
	return best
}

// recommendProfileForBottleneck maps the bottleneck component to a profile and a
// one-line diagnosis: CPU-bound upstream stages want more pipelines; a send-stage
// bottleneck wants more send concurrency.
func recommendProfileForBottleneck(component string, latencyMs int64) (profile string, reason string) {
	switch {
	case component == "processor":
		return profileHighThroughput, "The logs pipeline is bottlenecked at the processor stage, which is CPU-bound."
	case component == "strategy":
		return profileHighThroughput, "The logs pipeline is bottlenecked at the compression and batching stage, which is CPU-bound."
	case component == "worker" || component == logsMetrics.SenderTlmName || strings.HasPrefix(component, "destination_"):
		if latencyMs >= senderLatencyHighThresholdMs {
			return profileHighConcurrency, fmt.Sprintf("The logs pipeline is bottlenecked at the network send stage, with high intake latency (%dms).", latencyMs)
		}
		return profileHighConcurrency, "The logs pipeline is bottlenecked at the network send stage."
	default:
		return profileHighThroughput, "The logs pipeline is saturated."
	}
}

// getProfileRecommendation suggests a profile only when logs were recently lost
// (droppedRecently/missedRecently); saturation merely localizes the bottleneck.
// Returns nil when nothing is being lost, when no profile would help, or when the
// active profile already covers the suggestion.
func (b *Builder) getProfileRecommendation(utils []ComponentUtilization, activeProfile string, latencyMs int64, droppedRecently, missedRecently, delivering bool) *ProfileRecommendation {
	if !droppedRecently && !missedRecently {
		return nil
	}

	bottleneck := b.bottleneckComponent(utils)

	// A profile is warranted when recent loss localizes to a stage one can fix:
	// send-side drops at a saturated, still-delivering send stage; or read-side
	// backpressure at any saturated stage (but not an idle reader or unreachable
	// intake, which no profile fixes). Evaluated independently so a recent drop
	// that maps to nothing does not mask a fixable read-side bottleneck.
	sendStageLoss := droppedRecently && isSendStage(bottleneck) && delivering
	backpressureLoss := missedRecently && bottleneck != "" && !(isSendStage(bottleneck) && !delivering)
	if !sendStageLoss && !backpressureLoss {
		return nil
	}

	recommended, reason := recommendProfileForBottleneck(bottleneck, latencyMs)
	// Skip if nothing to suggest, already active, or the active profile is a
	// superset of it (a lower-tier "switch" would only lower settings).
	//
	// The coverage check compares catalog profiles by name, not effective config
	// values, and that is sufficient: a profile sets its knobs to the covering
	// values unless the user explicitly overrode one, and an explicit override
	// always wins over a profile (see applyLogsPerformanceProfile). So the only
	// way effective coverage breaks is an explicit override — in which case
	// switching profiles would not change that knob anyway, so the recommendation
	// would be futile. We therefore never hide an actionable switch here.
	if recommended == "" || recommended == activeProfile ||
		pkgconfigsetup.LogsPerformanceProfileCovers(activeProfile, recommended) {
		return nil
	}
	return &ProfileRecommendation{Profile: recommended, Reason: "Logs are being lost. " + reason}
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
	metrics["LogsDropped"] = strconv.FormatInt(b.logsDropped(), 10)
	metrics["BytesMissed"] = strconv.FormatInt(b.bytesMissed(), 10)
	metrics["BytesSent"] = strconv.FormatInt(b.logsExpVars.Get("BytesSent").(*expvar.Int).Value(), 10)
	metrics["RetryCount"] = strconv.FormatInt(b.logsExpVars.Get("RetryCount").(*expvar.Int).Value(), 10)
	metrics["RetryTimeSpent"] = time.Duration(b.logsExpVars.Get("RetryTimeSpent").(*expvar.Int).Value()).String()
	metrics["EncodedBytesSent"] = strconv.FormatInt(b.logsExpVars.Get("EncodedBytesSent").(*expvar.Int).Value(), 10)
	metrics["LogsTruncated"] = strconv.FormatInt(b.logsExpVars.Get("LogsTruncated").(*expvar.Int).Value(), 10)
	metrics["SenderLatency"] = time.Duration(b.senderLatencyMs() * int64(time.Millisecond)).String()
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
