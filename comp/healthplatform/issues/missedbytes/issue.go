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

	subject := fmt.Sprintf("from %d %s", sourceCount, pluralize(sourceCount, "source"))
	origin := fmt.Sprintf("across %d %s", sourceCount, pluralize(sourceCount, "source"))
	breakdown := describeSources(sources, omitted)
	if named {
		subject = "from source " + sources[0].Source
		origin = fmt.Sprintf("from source %q (service %q)", sources[0].Source, sources[0].Service)
		breakdown = ""
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
		Title: fmt.Sprintf("Lost %s of logs %s across %d %s in the last 24 hours",
			humanizeBytes(bytesLost), subject, rotations, pluralize(rotations, "rotation")),
		// Repeats the breakdown because Description is the only field confirmed
		// to render to a customer.
		Description: fmt.Sprintf("The logs agent lost %s of logs %s in the last 24 hours because %d log %s closed a file before the tailer finished reading it.%s The most recent loss was at %s.",
			humanizeBytes(bytesLost), origin, rotations, pluralize(rotations, "rotation"), breakdown, lastLossAt),
		Category: issueCategory,
		Location: "logs-agent",
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
	return sources
}

// describeSources renders the breakdown as a sentence, including a count of the
// tuples the cap left out.
func describeSources(sources []sourceLoss, omitted int64) string {
	if len(sources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sources)+1)
	for _, s := range sources {
		parts = append(parts, fmt.Sprintf("%s/%s %s (%d %s)",
			s.Source, s.Service, humanizeBytes(s.Bytes), s.Rotations, pluralize(s.Rotations, "rotation")))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, pluralize(omitted, "source")))
	}
	return " Most affected: " + strings.Join(parts, ", ") + "."
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
