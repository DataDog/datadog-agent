// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsprofile

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/profilerec"
)

const (
	// Encoded recommendation; Context is map[string]string.
	contextKeyRecommendation = "recommendation"

	issueSource   = "logs-profile-recommendation"
	issueCategory = "logs_pipeline"

	recommendationKind          = "logs_performance_profile"
	recommendationSchemaVersion = 1
	defaultVerifyWindowSeconds  = 30 * 60
)

type kind struct {
	name     string
	typ      string
	idPrefix string
	severity healthplatform.IssueSeverity
	title    string
}

var (
	recommended = kind{
		name:     IssueName,
		typ:      IssueType,
		idPrefix: IssueID,
		severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		title:    "Recommended: apply the %s logs performance profile",
	}
	suggested = kind{
		name:     SuggestedIssueName,
		typ:      SuggestedIssueType,
		idPrefix: SuggestedIssueID,
		severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW,
		title:    "Suggested: apply the %s logs performance profile",
	}
)

var profileBenefits = map[string]string{
	profilerec.ProfileHighConcurrency: "keeps more payloads in flight to the intake at once, so slow round trips hold up less of the pipeline",
	profilerec.ProfileHighThroughput:  "runs one pipeline per CPU core, up to 16, so more logs are processed in parallel",
}

var profileTradeoffs = map[string]string{
	profilerec.ProfileHighConcurrency: "More payloads in flight at once; higher memory use under load.",
	profilerec.ProfileHighThroughput:  "One pipeline per CPU core, up to 16; more CPU and memory under load.",
}

// recommendation is the check-time state as it crosses Context. Its JSON tags are the wire
// contract with the check.
type recommendation struct {
	Profile             string        `json:"profile"`
	ProfileVersion      int           `json:"profile_version"`
	ProfileDescription  string        `json:"profile_description,omitempty"`
	ProfileKeySource    string        `json:"profile_key_source,omitempty"`
	ReasonCode          string        `json:"reason_code"`
	Reason              string        `json:"reason"`
	Bottleneck          string        `json:"bottleneck,omitempty"`
	BackpressureState   string        `json:"backpressure_state"`
	SenderLatencyMs     int64         `json:"sender_latency_ms"`
	DroppedRecently     bool          `json:"logs_dropped_recent"`
	MissedRecently      bool          `json:"bytes_missed_recent"`
	Saturated30mSeconds int64         `json:"saturated_30m_s"`
	ActiveProfile       string        `json:"active_profile,omitempty"`
	VerifyWindowSeconds int64         `json:"verify_window_s,omitempty"`
	Current             []settingWire `json:"current,omitempty"`
	Changes             []changeWire  `json:"changes,omitempty"`
	Blocked             []blockedWire `json:"blocked,omitempty"`
}

type settingWire struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

type changeWire struct {
	Key  string `json:"key"`
	From any    `json:"from"`
	To   any    `json:"to"`
}

type blockedWire struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}

// RecommendedIssue is the template for "logs-performance-profile-recommended" issues.
type RecommendedIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (RecommendedIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	return buildIssue(recommended, ctx)
}

// SuggestedIssue is the template for "logs-performance-profile-suggested" issues.
type SuggestedIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (SuggestedIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	return buildIssue(suggested, ctx)
}

func buildIssue(k kind, ctx map[string]string) (*healthplatform.Issue, error) {
	var w recommendation
	// Malformed input leaves w empty, so the issue ships without Extra.recommendation.
	_ = json.Unmarshal([]byte(ctx[contextKeyRecommendation]), &w)

	extraFields := map[string]any{}
	if w.Profile != "" {
		extraFields["recommendation"] = recommendationExtra(w)
	}
	extra, err := structpb.NewStruct(extraFields)
	if err != nil {
		return nil, fmt.Errorf("logsprofile: build issue extra: %w", err)
	}

	name := w.Profile
	if name == "" {
		name = "recommended"
	}
	return &healthplatform.Issue{
		IssueName:   k.name,
		IssueType:   k.typ,
		Title:       fmt.Sprintf(k.title, name),
		Description: describe(w, name),
		Category:    issueCategory,
		Location:    "logs-agent",
		Severity:    k.severity,
		Source:      issueSource,
		Extra:       extra,
		Tags:        []string{"logs", "performance-profile", w.Profile},
		Remediation: &healthplatform.Remediation{
			Summary: fmt.Sprintf("Apply the %s logs performance profile; the Agent restarts to pick it up.", name),
			Steps:   remediationSteps(w),
		},
	}, nil
}

// describe stays one plain-text block: `agent diagnose` prints it verbatim.
func describe(w recommendation, name string) string {
	var sentences []string
	switch {
	case w.Bottleneck == "":
		sentences = append(sentences, "The logs pipeline is saturated.")
	case w.Saturated30mSeconds > 0:
		sentences = append(sentences, fmt.Sprintf("The %s of the logs pipeline was saturated for %s of the last 30 minutes.",
			strings.ToLower(stageLabel(w.Bottleneck)), fmtSeconds(w.Saturated30mSeconds)))
	default:
		sentences = append(sentences, fmt.Sprintf("The %s of the logs pipeline is saturated.", strings.ToLower(stageLabel(w.Bottleneck))))
	}
	if w.SenderLatencyMs > 0 && profilerec.IsSendStage(w.Bottleneck) {
		sentences = append(sentences, fmt.Sprintf("Intake latency is %d ms.", w.SenderLatencyMs))
	}
	if benefit, ok := profileBenefits[w.Profile]; ok {
		sentences = append(sentences, fmt.Sprintf("The %s profile %s.", name, benefit))
	} else {
		sentences = append(sentences, fmt.Sprintf("Applying the %s logs performance profile should relieve it.", name))
	}
	return strings.Join(sentences, " ")
}

func remediationSteps(w recommendation) []*healthplatform.RemediationStep {
	profile, version := w.Profile, strconv.Itoa(w.ProfileVersion)
	if profile == "" {
		profile, version = "<profile>", "<version>"
	}
	texts := []string{
		"Run `sudo datadog-agent status` and check the Logs Agent Backpressure section to confirm which stage is saturated.",
	}
	if w.ProfileKeySource == string(pkgconfigmodel.SourceEnvVar) {
		texts = append(texts, fmt.Sprintf("`logs_config.profile` is set by an environment variable on this host. Set `DD_LOGS_CONFIG_PROFILE=%s` and `DD_LOGS_CONFIG_PROFILE_VERSION=%s` instead, then restart the Agent.",
			profile, version))
	} else {
		texts = append(texts, fmt.Sprintf("Deploy the profile from Fleet Automation, or set `logs_config.profile: %s` and `logs_config.profile_version: %s` in datadog.yaml and restart the Agent. Deploying from Fleet restarts the Agent for you.",
			profile, version))
	}
	if w.ProfileDescription != "" {
		texts = append(texts, "What the profile changes: "+w.ProfileDescription+" Settings set explicitly on this host keep their values.")
	}
	texts = append(texts, "Re-run `sudo datadog-agent status` after the restart to confirm the profile is active. This issue resolves once the logs pipeline stays healthy.")

	steps := make([]*healthplatform.RemediationStep, 0, len(texts))
	for i, text := range texts {
		steps = append(steps, &healthplatform.RemediationStep{Order: int32(i + 1), Text: text})
	}
	return steps
}

func recommendationExtra(w recommendation) map[string]any {
	settings := make([]any, 0, len(w.Current))
	for _, s := range w.Current {
		settings = append(settings, map[string]any{"key": s.Key, "value": scalar(s.Value), "source": s.Source})
	}
	rec := map[string]any{
		"kind":               recommendationKind,
		"profile":            w.Profile,
		"profile_version":    w.ProfileVersion,
		"evidence":           evidence(w),
		"tradeoffs":          tradeoffs(w),
		"schema_version":     recommendationSchemaVersion,
		"reason_code":        w.ReasonCode,
		"profile_key_source": w.ProfileKeySource,
		"current":            map[string]any{"profile": w.ActiveProfile, "settings": settings},
		"evidence_detail": map[string]any{
			"bottleneck":          w.Bottleneck,
			"backpressure_state":  w.BackpressureState,
			"sender_latency_ms":   w.SenderLatencyMs,
			"logs_dropped_recent": w.DroppedRecently,
			"bytes_missed_recent": w.MissedRecently,
			"saturated_30m_s":     w.Saturated30mSeconds,
		},
		"verify_window_s": cmp.Or(w.VerifyWindowSeconds, defaultVerifyWindowSeconds),
	}
	if len(w.Changes) > 0 {
		changes := make([]any, 0, len(w.Changes))
		for _, c := range w.Changes {
			changes = append(changes, map[string]any{"key": c.Key, "from": scalar(c.From), "to": scalar(c.To)})
		}
		rec["changes"] = changes
	}
	if len(w.Blocked) > 0 {
		blocked := make([]any, 0, len(w.Blocked))
		for _, b := range w.Blocked {
			blocked = append(blocked, map[string]any{"key": b.Key, "source": b.Source})
		}
		rec["blocked_keys"] = blocked
	}
	return rec
}

func evidence(w recommendation) []any {
	var out []any
	if w.Bottleneck != "" {
		if w.Saturated30mSeconds > 0 {
			out = append(out, fmt.Sprintf("%s saturated for %s of the last 30 minutes", stageLabel(w.Bottleneck), fmtSeconds(w.Saturated30mSeconds)))
		} else {
			out = append(out, stageLabel(w.Bottleneck)+" saturated right now")
		}
	}
	if w.SenderLatencyMs > 0 && profilerec.IsSendStage(w.Bottleneck) {
		out = append(out, fmt.Sprintf("Intake latency: %d ms", w.SenderLatencyMs))
	}
	if w.BackpressureState != "" {
		out = append(out, "Pipeline backpressure: "+strings.ToLower(w.BackpressureState))
	}
	if out == nil {
		out = []any{}
	}
	return out
}

func tradeoffs(w recommendation) []any {
	var out []any
	if t, ok := profileTradeoffs[w.Profile]; ok {
		out = append(out, t)
	}
	if len(w.Blocked) > 0 {
		keys := make([]string, 0, len(w.Blocked))
		for _, b := range w.Blocked {
			keys = append(keys, b.Key)
		}
		out = append(out, blockedTradeoff(keys))
	}
	if out == nil {
		out = []any{}
	}
	return out
}

func blockedTradeoff(keys []string) string {
	for i, k := range keys {
		keys[i] = "`" + k + "`"
	}
	if len(keys) == 1 {
		return keys[0] + " is set explicitly on this host and keeps its current value."
	}
	list := strings.Join(keys[:len(keys)-1], ", ") + " and " + keys[len(keys)-1]
	return list + " are set explicitly on this host and keep their current values."
}

func stageLabel(name string) string {
	switch {
	case profilerec.IsSendStage(name):
		return "Network send stage"
	case name == "processor":
		return "Processor stage"
	case name == "strategy":
		return "Compression and batching stage"
	}
	return "Pipeline stage"
}

func fmtSeconds(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm", (seconds+30)/60)
}

// scalar reduces a config value to a type structpb and JSON both carry.
func scalar(v any) any {
	if v == nil {
		return ""
	}
	if d, ok := v.(time.Duration); ok {
		return d.String()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		if f := rv.Float(); !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f
		}
	}
	return fmt.Sprint(v)
}
