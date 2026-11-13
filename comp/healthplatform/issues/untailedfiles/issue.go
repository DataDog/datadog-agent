// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package untailedfiles

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
	issueName = IssueName
	issueType = IssueType

	// logs_config.open_files_limit as the scan saw it.
	contextKeyLimit = "open_files_limit"

	contextKeyTailed   = "tailed_files"
	contextKeyUntailed = "untailed_files"

	// Distinct sources that lost files, listed or not.
	contextKeySourceCount    = "source_count"
	contextKeySourcesOmitted = "sources_omitted"

	contextKeyLastSeenAt = "last_seen_at"

	// Context is map[string]string, so the breakdown travels as encoded sourceFiles.
	contextKeySources = "sources"

	// A path pattern is user config and legitimately longer than a name.
	maxPathLen = 256
)

// sourceFiles is one source's share of the untailed files. Its JSON tags are the
// wire contract with the check.
type sourceFiles struct {
	Source   string `json:"source"`
	Service  string `json:"service"`
	Path     string `json:"path"`
	Matched  int    `json:"matched"`
	Untailed int    `json:"untailed"`
}

// UntailedFilesIssue is the template for "log-files-not-tailed" issues.
type UntailedFilesIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (UntailedFilesIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	limit, _ := strconv.ParseInt(ctx[contextKeyLimit], 10, 64)
	tailed, _ := strconv.ParseInt(ctx[contextKeyTailed], 10, 64)
	untailed, _ := strconv.ParseInt(ctx[contextKeyUntailed], 10, 64)
	sourceCount, _ := strconv.ParseInt(ctx[contextKeySourceCount], 10, 64)
	omitted, _ := strconv.ParseInt(ctx[contextKeySourcesOmitted], 10, 64)
	matched := tailed + untailed

	lastSeenAt := ctx[contextKeyLastSeenAt]
	if lastSeenAt == "" {
		lastSeenAt = logsissue.Unknown
	}

	sources := decodeSources(ctx[contextKeySources])

	sentences := []string{fmt.Sprintf(
		"The Agent tails at most %s log files at once (logs_config.open_files_limit), and the last file scan to reach that limit matched %s, so %s %s from %d %s got no tailer: logs written to them are not collected, and are lost if those files rotate away before a slot frees up.",
		logsissue.HumanizeCount(limit), logsissue.HumanizeCount(matched),
		logsissue.HumanizeCount(untailed), logsissue.Pluralize(untailed, "file"),
		sourceCount, logsissue.Pluralize(sourceCount, "source"),
	)}
	if breakdown := describeSources(sources, omitted); breakdown != "" {
		sentences = append(sentences, breakdown)
	}

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyLimit:          limit,
		contextKeyTailed:         tailed,
		contextKeyUntailed:       untailed,
		contextKeySourceCount:    sourceCount,
		contextKeySourcesOmitted: omitted,
		contextKeyLastSeenAt:     lastSeenAt,
		contextKeySources:        sourcesAsExtra(sources),
	})
	if err != nil {
		return nil, fmt.Errorf("untailedfiles: build issue extra: %w", err)
	}

	raiseTo := "the number of files you need to tail"
	if matched > 0 {
		raiseTo = "at least " + strconv.FormatInt(matched, 10)
	}

	return &healthplatform.Issue{
		IssueName: issueName,
		IssueType: issueType,
		Title: fmt.Sprintf("Not tailing %s of %s matching log files: logs_config.open_files_limit (%s) reached",
			logsissue.HumanizeCount(untailed), logsissue.HumanizeCount(matched), logsissue.HumanizeCount(limit)),
		// One block, no newlines or markdown: `agent diagnose` prints this verbatim
		// behind a fixed prefix (comp/core/diagnose/format/format.go).
		Description: strings.Join(sentences, " "),
		Category:    logsissue.Category,
		Location:    logsissue.Location,
		// MEDIUM lands in "Warning": logs are not collected, but nothing is known lost
		// until a file rotates away untailed.
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:   logsissue.Source,
		Extra:    extra,
		Tags:     []string{"logs", "file-tailing", "open-files-limit"},
		// Ordered cheapest to most invasive, per the component's remediation rules.
		Remediation: &healthplatform.Remediation{
			// Plain text: Summary is not rendered as markdown.
			Summary: "Let the Agent tail every file your log configurations match, by raising the open files limit or matching fewer files.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Run `sudo datadog-agent status`. In the Logs Agent section, each wildcard source shows how many of its matching files are tailed, and a warning says the open files limit was reached."},
				{Order: 2, Text: fmt.Sprintf("Raise `logs_config.open_files_limit` (DD_LOGS_CONFIG_OPEN_FILES_LIMIT) from %d to %s. On Linux and macOS, also keep the Agent process's file descriptor limit (`ulimit -n`, or `LimitNOFILE` for its service) above that value.", limit, raiseTo)},
				{Order: 3, Text: "Narrow wildcard `path` patterns to the files you need, or skip rotated and archived files with `exclude_paths` (for example `/var/log/app/*.gz`)."},
				{Order: 4, Text: "If the limit cannot go higher, set `logs_config.file_wildcard_selection_mode` to `by_modification_time` so the most recently written files are tailed first."},
				{Order: 5, Text: "Run `sudo datadog-agent status` again and confirm every matching file is tailed. This issue clears an hour after the limit was last reached."},
			},
		},
	}, nil
}

// decodeSources tolerates a malformed value: totals stay usable, so a failure only
// degrades the wording. Every consumer reads names through here, so it sanitizes.
func decodeSources(encoded string) []sourceFiles {
	if encoded == "" {
		return nil
	}
	var sources []sourceFiles
	if err := json.Unmarshal([]byte(encoded), &sources); err != nil {
		return nil
	}
	for i := range sources {
		sources[i].Source = logsissue.SanitizeName(sources[i].Source)
		sources[i].Service = logsissue.SanitizeName(sources[i].Service)
		sources[i].Path = logsissue.SanitizeTo(sources[i].Path, maxPathLen)
	}
	return sources
}

// describeSources renders the breakdown, largest first, plus a count of what the
// cap left out.
func describeSources(sources []sourceFiles, omitted int64) string {
	if len(sources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sources)+1)
	for _, s := range sources {
		parts = append(parts, fmt.Sprintf("%s/%s (%s) %d of %d %s",
			s.Source, s.Service, s.Path, s.Untailed, s.Matched, logsissue.Pluralize(int64(s.Matched), "file")))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, logsissue.Pluralize(omitted, "source")))
	}
	return "Most affected: " + strings.Join(parts, ", ") + "."
}

// sourcesAsExtra reshapes the breakdown so the backend receives objects, not a string.
func sourcesAsExtra(sources []sourceFiles) []any {
	values := make([]any, 0, len(sources))
	for _, s := range sources {
		values = append(values, map[string]any{
			"source":   s.Source,
			"service":  s.Service,
			"path":     s.Path,
			"matched":  s.Matched,
			"untailed": s.Untailed,
		})
	}
	return values
}
