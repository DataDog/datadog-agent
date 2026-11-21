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

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
)

const (
	contextKeyBytes     = "bytes_missed_24h"
	contextKeyRotations = "rotation_count_24h"

	// Distinct source names, not tuples: this is the count that reaches the prose.
	contextKeySourceCount = "source_count"

	contextKeyPairsOmitted = "source_service_pairs_omitted"

	contextKeyLastLossAt = "last_loss_at"

	// Context is map[string]string, so the breakdown travels as encoded sourceLoss.
	contextKeySources = "sources"
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
	omitted, _ := strconv.ParseInt(ctx[contextKeyPairsOmitted], 10, 64)

	lastLossAt := ctx[contextKeyLastLossAt]
	if lastLossAt == "" {
		lastLossAt = logsissue.Unknown
	}

	sources := decodeSources(ctx[contextKeySources])

	// Nameable only when one source and one service are the entire loss.
	named := sourceCount == 1 && len(sources) == 1 && omitted == 0

	scope := fmt.Sprintf("%d %s", sourceCount, logsissue.Pluralize(sourceCount, "source"))
	subject := "Logs from " + scope
	file := "a file"
	breakdown := describeSources(sources, omitted)
	if named {
		scope = "source " + sources[0].Source
		subject = fmt.Sprintf("Logs from source %q (service %q)", sources[0].Source, sources[0].Service)
		file = "the file"
		breakdown = ""
	}

	sentences := []string{
		fmt.Sprintf("%s never reached Datadog: %d log %s closed %s before the Agent finished reading it.",
			subject, rotations, logsissue.Pluralize(rotations, "rotation"), file),
	}
	if breakdown != "" {
		sentences = append(sentences, breakdown)
	}

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyBytes:        bytesLost,
		contextKeyRotations:    rotations,
		contextKeySourceCount:  sourceCount,
		contextKeyPairsOmitted: omitted,
		contextKeyLastLossAt:   lastLossAt,
		contextKeySources:      sourcesAsExtra(sources),
	})
	if err != nil {
		return nil, fmt.Errorf("missedbytes: build issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName: IssueName,
		IssueType: IssueType,
		Title: fmt.Sprintf("Lost %s of logs from %s in the last 24 hours",
			logsissue.HumanizeBytes(bytesLost), scope),
		// One block, no newlines or markdown: `agent diagnose` prints this verbatim
		// behind a fixed prefix (comp/core/diagnose/format/format.go).
		Description: strings.Join(sentences, " "),
		Category:    logsissue.Category,
		Location:    logsissue.Location,
		// HIGH lands in the Fleet UI's "Agent Health: Broken" bucket.
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:   logsissue.Source,
		Extra:    extra,
		Tags:     []string{"logs", "file-tailing", "rotation", "data-loss"},
		Remediation: &healthplatform.Remediation{
			Summary: remediationSummary,
			Steps:   logsissue.Steps(remediationSteps...),
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
		sources[i].Source = logsissue.SanitizeName(sources[i].Source)
		sources[i].Service = logsissue.SanitizeName(sources[i].Service)
	}
	return sources
}

// describeSources renders the breakdown ranked by bytes, plus a count of what the
// cap left out. Rotation counts are left to Extra.
func describeSources(sources []sourceLoss, omitted int64) string {
	if len(sources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sources)+1)
	for _, s := range sources {
		parts = append(parts, fmt.Sprintf("%s/%s %s", s.Source, s.Service, logsissue.HumanizeBytes(s.Bytes)))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, logsissue.Pluralize(omitted, "source/service pair")))
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
