// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

const (
	issueName = IssueName
	issueType = IssueType

	contextKeyLogs  = "logs_dropped_24h"
	contextKeyBytes = "bytes_dropped_24h"

	// Distinct source names, not rows: this is the count that reaches the prose.
	contextKeySourceCount = "source_count"

	contextKeyRowsOmitted   = "rows_omitted"
	contextKeyCausesOmitted = "causes_omitted"

	// "true" when the main endpoint lost logs too.
	contextKeyPrimaryAffected = "primary_affected"

	// Logs the main endpoint lost (including those dropped before sending), and the
	// logs the Agent processed over about the same window. Their ratio decides
	// severity.
	contextKeyPrimaryLogs    = "main_endpoint_logs_dropped_24h"
	contextKeyPrimaryBytes   = "main_endpoint_bytes_dropped_24h"
	contextKeyPrimarySources = "main_endpoint_source_count"
	contextKeyProcessed      = "logs_processed_24h"

	// Every reason seen, comma-separated: the causes list is capped, remediation is not.
	contextKeyReasons = "reasons"

	// Reported in Extra only: computed, not part of the context contract.
	extraKeyLossRatio = "main_endpoint_loss_ratio"

	contextKeyLastDropAt = "last_drop_at"

	// Context is map[string]string, so both lists travel as encoded JSON.
	contextKeyCauses = "causes"
	contextKeyDrops  = "drops"

	// A hostname is at most 253 characters.
	maxHostLen = 253

	// highSeverityLossRatio is the share of processed logs the main endpoint must
	// lose before the issue reads as Broken. A rejected API key loses nearly all of
	// them; a destination that briefly fell behind loses a sliver, which is worth a
	// warning but is not a broken Agent.
	highSeverityLossRatio = 0.01
)

// cause is one (reason, destination) group's share of the host total. Its JSON tags
// are the wire contract with the check.
type cause struct {
	Reason      string `json:"reason"`
	Destination string `json:"destination"`
	Kind        string `json:"kind"`
	Logs        int64  `json:"logs"`
	Bytes       int64  `json:"bytes"`
}

// sourceDrop is one (source, service, reason, destination) row of the breakdown.
type sourceDrop struct {
	Source      string `json:"source"`
	Service     string `json:"service"`
	Reason      string `json:"reason"`
	Destination string `json:"destination"`
	Kind        string `json:"kind"`
	Logs        int64  `json:"logs"`
	Bytes       int64  `json:"bytes"`
}

// LogsDroppedIssue is the template for "logs-dropped-before-delivery" issues.
type LogsDroppedIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (LogsDroppedIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	// Read, not summed: the lists hold only the largest entries.
	logsDropped, _ := strconv.ParseInt(ctx[contextKeyLogs], 10, 64)
	bytesDropped, _ := strconv.ParseInt(ctx[contextKeyBytes], 10, 64)
	sourceCount, _ := strconv.ParseInt(ctx[contextKeySourceCount], 10, 64)
	rowsOmitted, _ := strconv.ParseInt(ctx[contextKeyRowsOmitted], 10, 64)
	causesOmitted, _ := strconv.ParseInt(ctx[contextKeyCausesOmitted], 10, 64)
	// Unparseable reads as affected: overstating severity beats hiding loss.
	primaryAffected := ctx[contextKeyPrimaryAffected] != "false"
	primaryLogs, _ := strconv.ParseInt(ctx[contextKeyPrimaryLogs], 10, 64)
	primaryBytes, _ := strconv.ParseInt(ctx[contextKeyPrimaryBytes], 10, 64)
	primarySources, _ := strconv.ParseInt(ctx[contextKeyPrimarySources], 10, 64)
	processed, _ := strconv.ParseInt(ctx[contextKeyProcessed], 10, 64)
	lossRatio, ratioKnown := mainEndpointLossRatio(primaryLogs, processed)

	lastDropAt := ctx[contextKeyLastDropAt]
	if lastDropAt == "" {
		lastDropAt = logsissue.Unknown
	}

	causes := decodeCauses(ctx[contextKeyCauses])
	drops := decodeDrops(ctx[contextKeyDrops])

	// The title counts logs, not copies. When the main endpoint lost logs, that is
	// the loss the customer's org sees; otherwise every lost row is a copy bound for
	// an additional endpoint, which the title says.
	headlineLogs, headlineBytes, headlineSources, unit := primaryLogs, primaryBytes, primarySources, "log"
	if !primaryAffected {
		headlineLogs, headlineBytes, headlineSources, unit = logsDropped, bytesDropped, sourceCount, "log copy"
	}
	if _, ok := ctx[contextKeyPrimaryLogs]; !ok && primaryAffected {
		// A context from before the main-endpoint totals existed.
		headlineLogs, headlineBytes, headlineSources = logsDropped, bytesDropped, sourceCount
	}

	// Nameable only when one source and one service are the entire loss.
	named := sourceCount == 1 && len(drops) > 0 && rowsOmitted == 0 && singleOrigin(drops)

	scope := fmt.Sprintf("%d %s", headlineSources, logsissue.Pluralize(headlineSources, "source"))
	subject := "Logs from " + scope
	if named {
		scope = fmt.Sprintf("source %q (service %q)", drops[0].Source, drops[0].Service)
		subject = fmt.Sprintf("Logs from source %q (service %q)", drops[0].Source, drops[0].Service)
	}

	sentences := []string{subject + " were read by the Agent but never delivered" + describeCauses(causes, causesOmitted) + "."}
	if !named {
		if breakdown := describeDrops(drops, rowsOmitted); breakdown != "" {
			sentences = append(sentences, breakdown)
		}
	}
	switch {
	case !primaryAffected:
		sentences = append(sentences, "The main endpoint still received these logs; only the copies listed were lost.")
	case ratioKnown:
		sentences = append(sentences, fmt.Sprintf("The main endpoint lost %s of the %s logs the Agent processed in this period.",
			formatRatio(lossRatio), logsissue.HumanizeCount(processed)))
	}

	severity := healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	// HIGH lands in the Fleet UI's "Agent Health: Broken" bucket. Without a
	// denominator, assume the loss is significant rather than understate it.
	if primaryAffected && (!ratioKnown || lossRatio >= highSeverityLossRatio) {
		severity = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	}

	reasons := reasonsPresent(ctx[contextKeyReasons], causes)

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyLogs:            logsDropped,
		contextKeyBytes:           bytesDropped,
		contextKeySourceCount:     sourceCount,
		contextKeyRowsOmitted:     rowsOmitted,
		contextKeyCausesOmitted:   causesOmitted,
		contextKeyPrimaryAffected: primaryAffected,
		contextKeyPrimaryLogs:     primaryLogs,
		contextKeyPrimaryBytes:    primaryBytes,
		contextKeyPrimarySources:  primarySources,
		contextKeyProcessed:       processed,
		contextKeyReasons:         reasonsAsExtra(reasons),
		extraKeyLossRatio:         lossRatio,
		contextKeyLastDropAt:      lastDropAt,
		contextKeyCauses:          causesAsExtra(causes),
		contextKeyDrops:           dropsAsExtra(drops),
	})
	if err != nil {
		return nil, fmt.Errorf("logsdropped: build issue extra: %w", err)
	}

	tags := []string{"logs", "data-loss", "dropped-logs"}
	for _, r := range reasons {
		tags = append(tags, string(r))
	}

	return &healthplatform.Issue{
		IssueName: issueName,
		IssueType: issueType,
		Title: fmt.Sprintf("Dropped %s %s (%s) from %s in the last 24 hours",
			logsissue.HumanizeCount(headlineLogs), pluralizeUnit(headlineLogs, unit),
			logsissue.HumanizeBytes(headlineBytes), scope),
		// One block, no newlines or markdown: `agent diagnose` prints this verbatim
		// behind a fixed prefix (comp/core/diagnose/format/format.go).
		Description: strings.Join(sentences, " "),
		Category:    logsissue.Category,
		Location:    logsissue.Location,
		Severity:    severity,
		Source:      logsissue.Source,
		Extra:       extra,
		Tags:        tags,
		Remediation: &healthplatform.Remediation{
			// Plain text: Summary is not rendered as markdown.
			Summary: "Fix what makes the intake or a destination reject logs, so the Agent stops dropping logs it has already read.",
			Steps:   remediationSteps(reasons),
		},
	}, nil
}

func pluralizeUnit(n int64, unit string) string {
	if n == 1 {
		return unit
	}
	if unit == "log copy" {
		return "log copies"
	}
	return unit + "s"
}

func reasonsAsExtra(reasons []logsmetrics.DropReason) []any {
	values := make([]any, 0, len(reasons))
	for _, r := range reasons {
		values = append(values, string(r))
	}
	return values
}

// mainEndpointLossRatio is the share of processed logs the main endpoint lost. It
// is unknown when nothing was counted as processed, which happens only when the
// context lacks the key: every dropped log was processed first.
func mainEndpointLossRatio(lost, processed int64) (float64, bool) {
	if processed <= 0 {
		return 0, false
	}
	// The two windows line up only to the hour; never report more than all of it.
	return min(float64(lost)/float64(processed), 1), true
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

// reasonOrder is the order remediation steps appear in: likeliest and cheapest fix
// first. Every DropReason has exactly one entry.
var reasonOrder = []logsmetrics.DropReason{
	logsmetrics.DropReasonAuthRejected,
	logsmetrics.DropReasonCredentialUnavailable,
	logsmetrics.DropReasonPayloadTooLarge,
	logsmetrics.DropReasonMessageTooLarge,
	logsmetrics.DropReasonBufferFull,
	logsmetrics.DropReasonSendFailed,
	logsmetrics.DropReasonBadRequest,
	logsmetrics.DropReasonEncodeFailed,
}

// reasonSteps holds the step each reason adds. Both reasons that only support can
// diagnose share the flare step, which remediationSteps adds once.
var reasonSteps = map[logsmetrics.DropReason]string{
	logsmetrics.DropReasonAuthRejected:          "The intake rejected the API key (HTTP 401 or 403). Check that `api_key`, or the `api_key` of the affected entry under `logs_config.additional_endpoints`, is a valid key for the Datadog site that endpoint sends to. If the key comes from a secrets backend, run `sudo datadog-agent secret` to check that it resolves. Then restart the Agent.",
	logsmetrics.DropReasonCredentialUnavailable: "The Agent had no API key to send with. Check that `api_key` is set, and that the secrets backend (`sudo datadog-agent secret`) or delegated authentication can provide one. Then restart the Agent.",
	logsmetrics.DropReasonPayloadTooLarge:       "The intake rejected payloads as too large (HTTP 413). If `logs_config.batch_max_content_size` or `logs_config.batch_max_size` was raised, set it back to its default (5000000 bytes and 1000 logs).",
	logsmetrics.DropReasonMessageTooLarge:       "Some log lines were larger than a whole batch. Keep `logs_config.max_message_size_bytes` (default 900000) below `logs_config.batch_max_content_size` (default 5000000) so long lines are truncated instead of dropped.",
	logsmetrics.DropReasonBufferFull:            "An endpoint fell behind until its buffer filled. Check connectivity from this host to that endpoint (proxy, DNS, firewall) and the Agent log for its send errors. If it is an additional endpoint, the main endpoint was not affected by this.",
	logsmetrics.DropReasonSendFailed:            "Sends failed with an error the Agent does not retry. Best-effort endpoints (`is_reliable: false`) never retry, so any network error loses that copy: check connectivity to the endpoint, or set `is_reliable: true` on it if every log must arrive there. For the main endpoint, check the Agent log for the failed request.",
	logsmetrics.DropReasonBadRequest:            flareStep,
	logsmetrics.DropReasonEncodeFailed:          flareStep,
}

const flareStep = "The intake rejected payloads as malformed (HTTP 400), or the Agent could not encode them. Run `sudo datadog-agent flare` and send it to Datadog support."

// remediationSteps orders the diagnostic steps first, then one step per reason
// present, then the check. Order stays contiguous and 1-indexed.
func remediationSteps(reasons []logsmetrics.DropReason) []*healthplatform.RemediationStep {
	texts := []string{
		"Run `sudo datadog-agent status` and check the Logs Agent section for the endpoints in use, their API key status, and any errors.",
		"Search the Agent log for rejected or failed sends to see each status code and endpoint, for example `grep -iE \"failed to post http payload|could not send payload|dropped message|dropping payload\" /var/log/datadog/agent.log` (on Windows, `C:\\ProgramData\\Datadog\\logs\\agent.log`). An endpoint that fell behind logs nothing when its buffer fills.",
	}
	seen := make(map[string]bool, len(reasons))
	for _, r := range reasons {
		text, ok := reasonSteps[r]
		if !ok || seen[text] {
			continue
		}
		seen[text] = true
		texts = append(texts, text)
	}
	texts = append(texts, "Run `sudo datadog-agent status` again and confirm the Logs Agent section shows no new errors. This issue clears 24 hours after the last dropped log.")

	steps := make([]*healthplatform.RemediationStep, 0, len(texts))
	for i, text := range texts {
		steps = append(steps, &healthplatform.RemediationStep{Order: int32(i + 1), Text: text})
	}
	return steps
}

// reasonsPresent returns the known reasons in the encoded list, in reasonOrder.
// Contexts without the list fall back to the ranked causes.
func reasonsPresent(encoded string, causes []cause) []logsmetrics.DropReason {
	present := make(map[logsmetrics.DropReason]bool, len(reasonOrder))
	if encoded != "" {
		for _, r := range strings.Split(encoded, ",") {
			present[logsmetrics.DropReason(r)] = true
		}
	} else {
		for _, c := range causes {
			present[logsmetrics.DropReason(c.Reason)] = true
		}
	}
	var reasons []logsmetrics.DropReason
	for _, r := range reasonOrder {
		if present[r] {
			reasons = append(reasons, r)
		}
	}
	return reasons
}

var reasonPhrases = map[logsmetrics.DropReason]string{
	logsmetrics.DropReasonAuthRejected:          "the intake rejected the API key (HTTP 401/403)",
	logsmetrics.DropReasonCredentialUnavailable: "no API key was available",
	logsmetrics.DropReasonPayloadTooLarge:       "the intake rejected payloads as too large (HTTP 413)",
	logsmetrics.DropReasonBadRequest:            "the intake rejected payloads as malformed (HTTP 400)",
	logsmetrics.DropReasonMessageTooLarge:       "log lines were larger than a whole batch",
	logsmetrics.DropReasonEncodeFailed:          "batches failed to encode",
	logsmetrics.DropReasonBufferFull:            "the endpoint fell behind and its buffer was full",
	logsmetrics.DropReasonSendFailed:            "a best-effort send failed",
}

var kindPhrases = map[logsmetrics.DropTargetKind]string{
	logsmetrics.DropTargetPrimary:    "main endpoint",
	logsmetrics.DropTargetAdditional: "additional endpoint",
	logsmetrics.DropTargetBestEffort: "best-effort endpoint",
	logsmetrics.DropTargetMRF:        "failover endpoint",
}

// describeCauses renders ": 10,000 because <reason> at the main endpoint <host>;
// ..." for the ranked causes.
func describeCauses(causes []cause, omitted int64) string {
	if len(causes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(causes)+1)
	for _, c := range causes {
		phrase, ok := reasonPhrases[logsmetrics.DropReason(c.Reason)]
		if !ok {
			phrase = "of an unrecognized error"
		}
		parts = append(parts, fmt.Sprintf("%s because %s %s",
			logsissue.HumanizeCount(c.Logs), phrase, describeTarget(c)))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, logsissue.Pluralize(omitted, "cause")))
	}
	return ": " + strings.Join(parts, "; ")
}

func describeTarget(c cause) string {
	kind := logsmetrics.DropTargetKind(c.Kind)
	if kind == logsmetrics.DropTargetAll {
		return "before sending"
	}
	phrase, ok := kindPhrases[kind]
	if !ok {
		phrase = "endpoint"
	}
	return "at the " + phrase + " " + c.Destination
}

// describeDrops renders the breakdown ranked by logs dropped, plus a count of what
// the cap left out. Reasons and destinations are left to Extra.
func describeDrops(drops []sourceDrop, omitted int64) string {
	if len(drops) == 0 {
		return ""
	}
	parts := make([]string, 0, len(drops)+1)
	for _, d := range drops {
		parts = append(parts, fmt.Sprintf("%s/%s %s", d.Source, d.Service, logsissue.HumanizeCount(d.Logs)))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, logsissue.Pluralize(omitted, "row")))
	}
	return "Most affected: " + strings.Join(parts, ", ") + "."
}

func singleOrigin(drops []sourceDrop) bool {
	for _, d := range drops[1:] {
		if d.Source != drops[0].Source || d.Service != drops[0].Service {
			return false
		}
	}
	return true
}

// decodeCauses tolerates a malformed value: totals stay usable, so a failure only
// degrades the wording. Every consumer reads values through here, so it sanitizes.
func decodeCauses(encoded string) []cause {
	if encoded == "" {
		return nil
	}
	var causes []cause
	if err := json.Unmarshal([]byte(encoded), &causes); err != nil {
		return nil
	}
	for i := range causes {
		causes[i].Reason = logsissue.SanitizeName(causes[i].Reason)
		causes[i].Kind = logsissue.SanitizeName(causes[i].Kind)
		causes[i].Destination = logsissue.SanitizeTo(causes[i].Destination, maxHostLen)
	}
	return causes
}

func decodeDrops(encoded string) []sourceDrop {
	if encoded == "" {
		return nil
	}
	var drops []sourceDrop
	if err := json.Unmarshal([]byte(encoded), &drops); err != nil {
		return nil
	}
	for i := range drops {
		drops[i].Source = logsissue.SanitizeName(drops[i].Source)
		drops[i].Service = logsissue.SanitizeName(drops[i].Service)
		drops[i].Reason = logsissue.SanitizeName(drops[i].Reason)
		drops[i].Kind = logsissue.SanitizeName(drops[i].Kind)
		drops[i].Destination = logsissue.SanitizeTo(drops[i].Destination, maxHostLen)
	}
	return drops
}

// causesAsExtra and dropsAsExtra reshape the lists so the backend receives objects,
// not strings.
func causesAsExtra(causes []cause) []any {
	values := make([]any, 0, len(causes))
	for _, c := range causes {
		values = append(values, map[string]any{
			"reason":      c.Reason,
			"destination": c.Destination,
			"kind":        c.Kind,
			"logs":        c.Logs,
			"bytes":       c.Bytes,
		})
	}
	return values
}

func dropsAsExtra(drops []sourceDrop) []any {
	values := make([]any, 0, len(drops))
	for _, d := range drops {
		values = append(values, map[string]any{
			"source":      d.Source,
			"service":     d.Service,
			"reason":      d.Reason,
			"destination": d.Destination,
			"kind":        d.Kind,
			"logs":        d.Logs,
			"bytes":       d.Bytes,
		})
	}
	return values
}
