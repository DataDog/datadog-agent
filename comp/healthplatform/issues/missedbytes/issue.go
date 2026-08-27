// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/dustin/go-humanize"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	contextKeyBytes          = "bytes_missed_24h"
	contextKeyRotations      = "rotation_count_24h"
	contextKeySourceCount    = "source_count"
	contextKeySourcesOmitted = "sources_omitted"
	contextKeyLastLossAt     = "last_loss_at"

	// contextKeySources carries the breakdown as a JSON array of sourceLoss;
	// IssueReport.Context is map[string]string, so a list travels encoded.
	contextKeySources = "sources"

	// issueSource is the reporting component: the logs agent, not the
	// scheduler's checkSource label.
	issueSource = "logs"

	// issueCategory describes the failing subsystem. The loss happens in the log
	// pipeline; configuration is only where the fix lands.
	issueCategory = "logs_pipeline"

	// maxNameLen bounds a single source or service name. Both are free-form —
	// they come from user YAML and pod annotations — and each one ships twice
	// per payload, once rendered into Description and once structured in
	// Extra. Nothing else in comp/healthplatform bounds them.
	maxNameLen = 64

	// nameEllipsis marks a truncated name. Deliberately ASCII: Description is
	// printed verbatim by `agent diagnose`, including on Windows consoles that
	// may not be running a UTF-8 code page.
	nameEllipsis = "..."

	unknownValue = "unknown"
)

// sourceLoss is one (source, service) tuple's contribution to the host total.
// Its JSON tags are the wire contract between the check and this template.
type sourceLoss struct {
	Source    string `json:"source"`
	Service   string `json:"service"`
	Bytes     int64  `json:"bytes"`
	Rotations int64  `json:"rotations"`
}

// MissedBytesIssue is the template for "log-data-lost-after-rotation" issues.
type MissedBytesIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (MissedBytesIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	// Read, not summed from the breakdown, which is truncated to the largest
	// maxBreakdownSources tuples.
	bytesLost, _ := strconv.ParseInt(ctx[contextKeyBytes], 10, 64)
	rotations, _ := strconv.ParseInt(ctx[contextKeyRotations], 10, 64)
	sourceCount, _ := strconv.ParseInt(ctx[contextKeySourceCount], 10, 64)
	omitted, _ := strconv.ParseInt(ctx[contextKeySourcesOmitted], 10, 64)

	lastLossAt := ctx[contextKeyLastLossAt]
	if lastLossAt == "" {
		lastLossAt = unknownValue
	}

	sources := decodeSources(ctx[contextKeySources])

	// A named source is more actionable than a count of one, and needs no
	// breakdown repeating it.
	named := sourceCount == 1 && len(sources) == 1

	// scope is the Title's "from ..." clause; subject opens the Description.
	// file distinguishes the one known file from an unidentified one.
	scope := fmt.Sprintf("%d %s", sourceCount, pluralize(sourceCount, "source"))
	subject := "Logs from " + scope
	file := "a file"
	breakdown := describeSources(sources, omitted)
	if named {
		scope = "source " + sources[0].Source
		subject = fmt.Sprintf("Logs from source %q (service %q)", sources[0].Source, sources[0].Service)
		file = "the file"
		breakdown = ""
	}

	// Each aggregate lands in exactly one place: the byte total in the Title,
	// the rotation count in the Description, the per-source split in the
	// breakdown. Repeating all three in both fields is what made the old
	// wording read as a wall of text.
	sentences := []string{
		fmt.Sprintf("%s never reached Datadog: %d log %s closed %s before the Agent finished reading it.",
			subject, rotations, pluralize(rotations, "rotation"), file),
	}
	if breakdown != "" {
		sentences = append(sentences, breakdown)
	}
	sentences = append(sentences, "Last loss "+describeLastLoss(lastLossAt)+".")

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyBytes:          bytesLost,
		contextKeyRotations:      rotations,
		contextKeySourceCount:    sourceCount,
		contextKeySourcesOmitted: omitted,
		contextKeyLastLossAt:     lastLossAt,
		contextKeySources:        sourcesAsExtra(sources),
	})
	if err != nil {
		return nil, fmt.Errorf("missedbytes: build issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName: IssueName,
		IssueType: IssueType,
		Title: fmt.Sprintf("Lost %s of logs from %s in the last 24 hours",
			humanizeBytes(bytesLost), scope),
		// Single block of sentences, no newlines and no markdown: this string
		// is rendered verbatim by `agent diagnose` behind a fixed "  Diagnosis: "
		// prefix (comp/core/diagnose/format/format.go), so a line break would
		// break that indentation and a bullet would print as a literal "*".
		// It repeats the breakdown that Extra already carries structurally
		// because Description is the only field confirmed to render to a
		// customer.
		Description: strings.Join(sentences, " "),
		Category:    issueCategory,
		Location:    "logs-agent",
		// HIGH puts this in the Fleet UI's "Agent Health: Broken" bucket.
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:   issueSource,
		Extra:    extra,
		Tags:     []string{"logs", "file-tailing", "rotation", "data-loss"},
		Remediation: &healthplatform.Remediation{
			// Summary only: remediation content is authored backend-side.
			Summary: "Adjust your agent configuration",
		},
	}, nil
}

// decodeSources parses the breakdown, tolerating a malformed or absent value:
// totals stay usable, so a decode failure only degrades the wording.
func decodeSources(encoded string) []sourceLoss {
	if encoded == "" {
		return nil
	}
	var sources []sourceLoss
	if err := json.Unmarshal([]byte(encoded), &sources); err != nil {
		return nil
	}
	// Bound the free-form names here, the one point every consumer reads
	// through: the Title's named case, the Description breakdown, and Extra
	// all take their strings from this slice.
	for i := range sources {
		sources[i].Source = truncateName(sources[i].Source)
		sources[i].Service = truncateName(sources[i].Service)
	}
	return sources
}

// truncateName bounds one free-form name, cutting on a rune boundary so a
// multi-byte name is never split into invalid UTF-8.
func truncateName(name string) string {
	if utf8.RuneCountInString(name) <= maxNameLen {
		return name
	}
	return string([]rune(name)[:maxNameLen-utf8.RuneCountInString(nameEllipsis)]) + nameEllipsis
}

// describeSources renders the breakdown as a sentence, including a count of the
// tuples the cap left out. Per-source rotation counts are deliberately left to
// Extra: the list is ranked by bytes, and repeating a second number for every
// entry is what made the ten-source case unreadable.
func describeSources(sources []sourceLoss, omitted int64) string {
	if len(sources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sources)+1)
	for _, s := range sources {
		parts = append(parts, fmt.Sprintf("%s/%s %s", s.Source, s.Service, humanizeBytes(s.Bytes)))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, pluralize(omitted, "source")))
	}
	return "Most affected: " + strings.Join(parts, ", ") + "."
}

// describeLastLoss renders recency as a tail phrase. A relative time reads
// better than an RFC3339 stamp dropped mid-sentence, but a value that does not
// parse is still surfaced verbatim rather than discarded.
func describeLastLoss(raw string) string {
	if raw == "" || raw == unknownValue {
		return "at an " + unknownValue + " time"
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return humanize.Time(t)
	}
	return "at " + raw
}

// sourcesAsExtra converts the breakdown into the shape structpb accepts, so it
// reaches the backend as a list of objects rather than a JSON string.
func sourcesAsExtra(sources []sourceLoss) []any {
	values := make([]any, 0, len(sources))
	for _, s := range sources {
		values = append(values, map[string]any{
			"source":    s.Source,
			"service":   s.Service,
			"bytes":     s.Bytes,
			"rotations": s.Rotations,
		})
	}
	return values
}

// humanizeBytes renders a byte count for a customer-facing string, tolerating
// the negative value a malformed context could parse to.
func humanizeBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.Bytes(uint64(n))
}

func pluralize(n int64, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
