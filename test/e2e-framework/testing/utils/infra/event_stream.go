// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// Symbols used to render engine events that are not resource steps.
const (
	eventSymbolWarning = "⚠"
	eventSymbolInfo    = "ℹ"
	eventSymbolSummary = "📊"
	eventSymbolCancel  = "🛑"
)

// eventStreamBufferSize is the buffer size of engine event channels. The Pulumi
// SDK forwards events as they occur; a generous buffer keeps the engine from
// blocking on a slow logger.
const eventStreamBufferSize = 200

// resourceOpVerbs maps a Pulumi resource operation to its in-progress verb
// ("creating") and completed verb ("created").
var resourceOpVerbs = map[apitype.OpType][2]string{
	apitype.OpCreate:               {"creating", "created"},
	apitype.OpUpdate:               {"updating", "updated"},
	apitype.OpDelete:               {"deleting", "deleted"},
	apitype.OpReplace:              {"replacing", "replaced"},
	apitype.OpCreateReplacement:    {"creating replacement", "created replacement"},
	apitype.OpDeleteReplaced:       {"deleting replaced", "deleted replaced"},
	apitype.OpRead:                 {"reading", "read"},
	apitype.OpReadReplacement:      {"reading replacement", "read"},
	apitype.OpRefresh:              {"refreshing", "refreshed"},
	apitype.OpReadDiscard:          {"discarding", "discarded"},
	apitype.OpDiscardReplaced:      {"discarding", "discarded"},
	apitype.OpImport:               {"importing", "imported"},
	apitype.OpImportReplacement:    {"importing replacement", "imported"},
	apitype.OpRemovePendingReplace: {"removing", "removed"},
	apitype.OpSame:                 {"matching", "unchanged"},
}

// summaryOpOrder fixes the display order of the resource change counts of a
// summary event, so the rendered output is deterministic.
var summaryOpOrder = []apitype.OpType{
	apitype.OpSame,
	apitype.OpCreate,
	apitype.OpCreateReplacement,
	apitype.OpUpdate,
	apitype.OpReplace,
	apitype.OpDelete,
	apitype.OpDeleteReplaced,
	apitype.OpRead,
	apitype.OpRefresh,
	apitype.OpImport,
}

// summaryVerbs maps a resource operation to the verb used for its count in a
// summary event (e.g. "3 created").
var summaryVerbs = map[apitype.OpType]string{
	apitype.OpSame:              "unchanged",
	apitype.OpCreate:            "created",
	apitype.OpCreateReplacement: "created",
	apitype.OpUpdate:            "updated",
	apitype.OpReplace:           "replaced",
	apitype.OpDelete:            "deleted",
	apitype.OpDeleteReplaced:    "deleted",
	apitype.OpRead:              "read",
	apitype.OpRefresh:           "refreshed",
	apitype.OpImport:            "imported",
}

// startEventStreamLogger creates a buffered channel for Pulumi engine events
// and starts a goroutine that formats each event received on the channel to
// logger. The goroutine returns once the channel is closed, which the Pulumi
// SDK does when the stack operation completes (successfully or not). A fresh
// channel must be used for every stack operation: the SDK closes the channel
// after each operation, and sending to a closed channel would panic.
func startEventStreamLogger(logger io.Writer) chan<- events.EngineEvent {
	ch := make(chan events.EngineEvent, eventStreamBufferSize)
	go func() {
		for event := range ch {
			if line, show := formatEngineEvent(event); show {
				// The destination is terminal-like output; write errors are not
				// fatal for the deployment and are intentionally ignored.
				_, _ = fmt.Fprintln(logger, line)
			}
		}
	}()
	return ch
}

// formatEngineEvent renders a Pulumi engine event as a clean, formatted line.
// It returns false when the event should be suppressed.
func formatEngineEvent(event events.EngineEvent) (string, bool) {
	// Stream errors are reported by the SDK on the event itself when it fails
	// to decode an engine event.
	if event.Error != nil {
		return fmt.Sprintf("%s event stream error: %v", eventSymbolWarning, event.Error), true
	}

	switch {
	case event.ResourcePreEvent != nil:
		return formatResourceStepEvent(event.ResourcePreEvent.Metadata, false, false)
	case event.ResOutputsEvent != nil:
		return formatResourceStepEvent(event.ResOutputsEvent.Metadata, true, false)
	case event.ResOpFailedEvent != nil:
		return formatResourceStepEvent(event.ResOpFailedEvent.Metadata, true, true)
	case event.DiagnosticEvent != nil:
		return formatDiagnosticEvent(*event.DiagnosticEvent)
	case event.SummaryEvent != nil:
		return formatSummaryEvent(*event.SummaryEvent)
	case event.CancelEvent != nil:
		return fmt.Sprintf("%s stack operation canceled", eventSymbolCancel), true
	case event.ProgressEvent != nil:
		// Plugin download/install progress is noise.
		return "", false
	default:
		// Stdout, prelude, policy and debugging events are engine internals.
		return "", false
	}
}

// formatResourceStepEvent renders a resource step event (before, completed or
// failed) as "<symbol> <type> <name> <verb>".
func formatResourceStepEvent(meta apitype.StepEventMetadata, done, failed bool) (string, bool) {
	// Provider reads and other engine internals are noise.
	if strings.HasPrefix(meta.Type, "pulumi:providers:") {
		return "", false
	}
	name := stepResourceName(meta)
	if name == "" {
		return "", false
	}

	if failed {
		return fmt.Sprintf("  %s %s %s %s failed", progressSymbolFailed, meta.Type, name, meta.Op), true
	}

	verbs, ok := resourceOpVerbs[meta.Op]
	if !ok {
		verbs = [2]string{string(meta.Op), string(meta.Op)}
	}
	verb := verbs[0] + "..."
	symbol := progressSymbolInProgress
	if done {
		verb = verbs[1]
		symbol = progressSymbolDone
	}
	return fmt.Sprintf("  %s %s %s %s", symbol, meta.Type, name, verb), true
}

// stepResourceName returns the resource name part of the step's URN (the last
// "::"-delimited segment), e.g. "myeks" for
// "urn:pulumi:stack::project::aws:eks:Cluster::myeks".
func stepResourceName(meta apitype.StepEventMetadata) string {
	if meta.URN == "" {
		return ""
	}
	return resource.URN(meta.URN).Name()
}

// formatDiagnosticEvent renders a provider/engine diagnostic, such as errors
// and warnings from a cloud resource provider.
func formatDiagnosticEvent(d apitype.DiagnosticEvent) (string, bool) {
	message := strings.TrimSpace(d.Message)
	if message == "" {
		return "", false
	}

	switch {
	case d.Severity == "error":
		return fmt.Sprintf("%s error: %s", progressSymbolFailed, message), true
	case d.Severity == "warning":
		return fmt.Sprintf("%s warning: %s", eventSymbolWarning, message), true
	case strings.HasPrefix(d.Severity, "info"):
		// General info diagnostics are engine noise; only resource-specific
		// info diagnostics (attached to a resource URN) are worth showing.
		if d.URN == "" {
			return "", false
		}
		return fmt.Sprintf("%s %s", eventSymbolInfo, message), true
	default:
		return "", false
	}
}

// formatSummaryEvent renders the final summary of a stack operation, e.g.
// "📊 18 created, 1 updated (2m11s)".
func formatSummaryEvent(s apitype.SummaryEvent) (string, bool) {
	var parts []string
	for _, op := range summaryOpOrder {
		if n := s.ResourceChanges[op]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, summaryVerbs[op]))
		}
	}

	// Include any operation not covered above, in a stable order.
	var extra []string
	for op, n := range s.ResourceChanges {
		if _, covered := summaryVerbs[op]; !covered && n > 0 {
			extra = append(extra, string(op))
		}
	}
	sort.Strings(extra)
	for _, op := range extra {
		parts = append(parts, fmt.Sprintf("%d %s", s.ResourceChanges[apitype.OpType(op)], op))
	}

	if len(parts) == 0 {
		parts = []string{"no changes"}
	}
	summary := fmt.Sprintf("%s %s (%s)", eventSymbolSummary, strings.Join(parts, ", "), formatDurationSeconds(s.DurationSeconds))
	if s.Result != "" && s.Result != apitype.OperationResultSucceeded {
		summary += fmt.Sprintf(" — %s", s.Result)
	}
	return summary, true
}

// formatDurationSeconds renders a duration in seconds as "45s", "2m11s", etc.
func formatDurationSeconds(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
}
