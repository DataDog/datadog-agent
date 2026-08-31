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
	"unicode"
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

	// Context is map[string]string, so the breakdown travels as encoded sourceLoss.
	contextKeySources = "sources"

	// The reporting component, not the scheduler's checkSource label.
	issueSource = "logs"

	// The failing subsystem, not where the fix lands.
	issueCategory = "logs_pipeline"

	// Bounds one free-form name, which ships in both Description and Extra.
	maxNameLen = 64

	// ASCII on purpose: `agent diagnose` prints Description on non-UTF-8 consoles.
	nameEllipsis = "..."

	unknownValue = "unknown"
)

// sourceLoss is one tuple's share of the host total. Its JSON tags are the wire
// contract with the check.
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
	// Read, not summed: the breakdown holds only the largest maxBreakdownSources.
	bytesLost, _ := strconv.ParseInt(ctx[contextKeyBytes], 10, 64)
	rotations, _ := strconv.ParseInt(ctx[contextKeyRotations], 10, 64)
	sourceCount, _ := strconv.ParseInt(ctx[contextKeySourceCount], 10, 64)
	omitted, _ := strconv.ParseInt(ctx[contextKeySourcesOmitted], 10, 64)

	lastLossAt := ctx[contextKeyLastLossAt]
	if lastLossAt == "" {
		lastLossAt = unknownValue
	}

	sources := decodeSources(ctx[contextKeySources])

	named := sourceCount == 1 && len(sources) == 1

	// scope is the Title's "from ..." clause; subject opens the Description.
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

	// Each aggregate lands in one place: bytes in the Title, rotations here.
	sentences := []string{
		fmt.Sprintf("%s never reached Datadog: %d log %s closed %s before the Agent finished reading it.",
			subject, rotations, pluralize(rotations, "rotation"), file),
	}
	if breakdown != "" {
		sentences = append(sentences, breakdown)
	}

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
		// One block, no newlines or markdown: `agent diagnose` prints this verbatim
		// behind a fixed prefix (comp/core/diagnose/format/format.go).
		Description: strings.Join(sentences, " "),
		Category:    issueCategory,
		Location:    "logs-agent",
		// HIGH lands in the Fleet UI's "Agent Health: Broken" bucket.
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:   issueSource,
		Extra:    extra,
		Tags:     []string{"logs", "file-tailing", "rotation", "data-loss"},
		Remediation: &healthplatform.Remediation{
			// Generic for now; detailed remediation is authored backend-side.
			Summary: "Run `agent status` and review the Logs Agent Backpressure section",
		},
	}, nil
}

// decodeSources tolerates a malformed value: totals stay usable, so a failure only
// degrades the wording. Every consumer reads names through here, so it sanitizes.
func decodeSources(encoded string) []sourceLoss {
	if encoded == "" {
		return nil
	}
	var sources []sourceLoss
	if err := json.Unmarshal([]byte(encoded), &sources); err != nil {
		return nil
	}
	for i := range sources {
		sources[i].Source = sanitizeName(sources[i].Source)
		sources[i].Service = sanitizeName(sources[i].Service)
	}
	return sources
}

// sanitizeName bounds a name and drops control characters: names come from user
// YAML and reach Title and Description unescaped. Cuts on a rune boundary.
func sanitizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	switch {
	case name == "":
		return unknownValue
	case utf8.RuneCountInString(name) <= maxNameLen:
		return name
	}
	return string([]rune(name)[:maxNameLen-utf8.RuneCountInString(nameEllipsis)]) + nameEllipsis
}

// describeSources renders the breakdown ranked by bytes, plus a count of what the
// cap left out. Rotation counts are left to Extra.
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

// sourcesAsExtra reshapes the breakdown so the backend receives objects, not a string.
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
