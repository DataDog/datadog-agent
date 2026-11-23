// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// highSeverityLossRatio is the share of processed logs the main endpoint must lose
// before the issue reads as Broken. A rejected API key loses nearly all of them; a
// destination that briefly fell behind loses a sliver, which is worth a warning
// but is not a broken Agent.
const highSeverityLossRatio = 0.01

// reasonPhrases says what happened, as the clause after "because".
var reasonPhrases = map[logsmetrics.DropReason]string{
	logsmetrics.DropReasonAuth:       "the intake rejected the API key, or the Agent had none (HTTP 401/403)",
	logsmetrics.DropReasonTooLarge:   "payloads or single log lines were too large (HTTP 413)",
	logsmetrics.DropReasonMalformed:  "the intake rejected payloads as malformed, or the Agent could not encode them (HTTP 400)",
	logsmetrics.DropReasonBufferFull: "an endpoint fell behind and its buffer filled",
	logsmetrics.DropReasonSendFailed: "sends to a best-effort endpoint failed",
}

// LogsDroppedIssue is the template for "logs-dropped-before-delivery" issues.
type LogsDroppedIssue struct{}

// BuildIssue builds the proto Issue from the check's context.
func (LogsDroppedIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	f := parseFacts(ctx)
	ratio, ratioKnown := f.lossRatio()

	severity := healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	// HIGH lands in the Fleet UI's "Agent Health: Broken" bucket. Without a
	// denominator, assume the loss is significant rather than understate it.
	if f.MainLogs > 0 && (!ratioKnown || ratio >= highSeverityLossRatio) {
		severity = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	}

	startedAt, lastDropAt := formatTime(f.StartedAt), formatTime(f.LastDropAt)

	// Flat on purpose: Fleet renders scalars as rows and string lists as tags, and
	// drops lists of objects.
	extra, err := structpb.NewStruct(map[string]any{
		"impact":                   f.impact(),
		keyMainLogs:                f.MainLogs,
		keyMainBytes:               f.MainBytes,
		keyCopyLogs:                f.CopyLogs,
		keyCopyBytes:               f.CopyBytes,
		keyProcessed:               f.Processed,
		"main_endpoint_loss_ratio": ratio,
		keySourceCount:             f.SourceCount,
		keyTopSources:              stringsAsExtra(f.sourceLabels(true)),
		keyReasons:                 stringsAsExtra(reasonNames(f.Reasons)),
		"endpoints":                stringsAsExtra(f.allEndpoints()),
		keyStartedAt:               startedAt,
		keyLastDropAt:              lastDropAt,
	})
	if err != nil {
		return nil, fmt.Errorf("logsdropped: build issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName: IssueName,
		IssueType: IssueType,
		Title:     f.title(),
		// One block, no newlines or markdown: `agent diagnose` prints this verbatim
		// behind a fixed prefix (comp/core/diagnose/format/format.go).
		Description: f.description(ratio, ratioKnown),
		Category:    logsissue.Category,
		Location:    logsissue.Location,
		Severity:    severity,
		Source:      logsissue.Source,
		Extra:       extra,
		Tags:        append([]string{"logs", "data-loss", "dropped-logs"}, reasonNames(f.Reasons)...),
		Remediation: remediation(f),
	}, nil
}

// lossRatio is the share of processed logs the main endpoint lost. It is unknown
// when nothing was counted as Processed: every dropped log was processed first, so
// that only happens on a context without the key.
func (f facts) lossRatio() (float64, bool) {
	if f.Processed <= 0 {
		return 0, false
	}
	// The two windows line up only to the hour; never report more than all of it.
	return min(float64(f.MainLogs)/float64(f.Processed), 1), true
}

// headline is what the title counts: logs the main endpoint lost, or, when it lost
// none, the copies other endpoints lost.
func (f facts) headline() (logs, bytes int64, noun string) {
	if f.MainLogs > 0 {
		return f.MainLogs, f.MainBytes, plural(f.MainLogs, "log", "logs")
	}
	return f.CopyLogs, f.CopyBytes, plural(f.CopyLogs, "log copy", "log copies")
}

func (f facts) title() string {
	logs, bytes, noun := f.headline()
	return fmt.Sprintf("Dropped %s %s (%s) from %s",
		logsissue.HumanizeCount(logs), noun, logsissue.HumanizeBytes(bytes), f.scope())
}

// scope names the one source and service when they are the whole loss.
func (f facts) scope() string {
	if f.SourceCount == 1 && len(f.TopSources) == 1 {
		s := f.TopSources[0]
		return fmt.Sprintf("source %q (service %q)", s.Source, s.Service)
	}
	n := int64(f.SourceCount)
	return fmt.Sprintf("%d %s", n, logsissue.Pluralize(n, "source"))
}

func (f facts) description(ratio float64, ratioKnown bool) string {
	causes := make([]string, 0, len(f.Reasons))
	for _, r := range f.Reasons {
		causes = append(causes, reasonPhrases[r])
	}
	because := ""
	if len(causes) > 0 {
		because = " because " + strings.Join(causes, "; ")
	}
	subject := "Logs from " + f.scope()
	if !f.StartedAt.IsZero() {
		subject = "Since " + f.StartedAt.UTC().Format("2006-01-02 15:04 UTC") + ", logs from " + f.scope()
	}
	sentences := []string{fmt.Sprintf("%s were read by the Agent but never delivered%s.", subject, because)}
	if labels := f.sourceLabels(false); len(labels) > 1 {
		sentences = append(sentences, "Most affected: "+strings.Join(labels, ", ")+".")
	}
	switch {
	case f.MainLogs == 0:
		sentences = append(sentences, "The main endpoint still received these logs; only copies bound for other endpoints were lost.")
	case ratioKnown:
		sentences = append(sentences, fmt.Sprintf("The main endpoint lost %s of the %s logs the Agent processed since then.",
			formatRatio(ratio), logsissue.HumanizeCount(f.Processed)))
	}
	return strings.Join(sentences, " ")
}

// impact is the one line the View Fix modal shows above the steps.
func (f facts) impact() string {
	logs, _, noun := f.headline()
	names := make([]string, 0, 2)
	for _, s := range f.TopSources[:min(len(f.TopSources), 2)] {
		names = append(names, s.Source+"/"+s.Service)
	}
	from := strings.Join(names, " and ")
	if len(f.TopSources) > len(names) {
		from += " and others"
	}
	line := fmt.Sprintf("Lost %s %s", logsissue.HumanizeCount(logs), noun)
	if from != "" {
		line += " from " + from
	}
	if phrase, ok := reasonPhrases[f.TopReason]; ok {
		line += ": " + phrase
	}
	if f.MainLogs == 0 {
		return line + ". The main endpoint was not affected."
	}
	return line + "."
}

// sourceLabels renders "nginx/web: 12,000", with "logs" when long is set.
func (f facts) sourceLabels(long bool) []string {
	labels := make([]string, 0, len(f.TopSources))
	for _, s := range f.TopSources {
		label := fmt.Sprintf("%s/%s: %s", s.Source, s.Service, logsissue.HumanizeCount(s.Logs))
		if long {
			label += " " + plural(s.Logs, "log", "logs")
		}
		labels = append(labels, label)
	}
	return labels
}

func (f facts) allEndpoints() []string {
	seen := map[string]bool{}
	var hosts []string
	for _, r := range f.Reasons {
		for _, h := range f.Endpoints[r] {
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	return hosts
}

func reasonNames(reasons []logsmetrics.DropReason) []string {
	names := make([]string, 0, len(reasons))
	for _, r := range reasons {
		names = append(names, string(r))
	}
	return names
}

func stringsAsExtra(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// formatTime renders a timestamp for Extra, or Unknown when it is missing.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return logsissue.Unknown
	}
	return t.UTC().Format(time.RFC3339)
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// formatRatio renders a ratio as a percentage a reader can compare at a glance.
func formatRatio(r float64) string {
	pct := r * 100
	switch {
	case pct >= 10:
		return fmt.Sprintf("%.0f%%", pct)
	case pct >= 1:
		return fmt.Sprintf("%.1f%%", pct)
	case pct >= 0.01:
		return fmt.Sprintf("%.2f%%", pct)
	case pct > 0:
		return "less than 0.01%"
	}
	return "0%"
}
