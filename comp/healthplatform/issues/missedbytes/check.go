// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// errFileTailingInactive means the tracker carries no information in this
// process, so the check cannot establish state either way.
var errFileTailingInactive = errors.New("missedbytes: file launcher not running")

// maxBreakdownSources caps how many tuples the issue enumerates individually.
// Totals still cover every tuple.
const maxBreakdownSources = 10

// checker turns the logs agent's missed-bytes tracker into issue reports.
type checker struct {
	hostname hostnameinterface.Component
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return &checker{hostname: hostname}
}

// Run reports a single issue summarising every (source, service) tuple that lost
// bytes to a rotation inside the tracker's trailing window. One issue rather than
// one per tuple because the backend keeps a single row per {org, issue_type}.
func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	// One-shot commands (flare, jmx, diagnose, analyze-logs, check) wire the
	// health platform without the logs agent, and share the running agent's
	// on-disk issue store. An error says "state unknown", which leaves the
	// scheduler's active-id set alone; nil would resolve the running agent's
	// issue from a process that never tailed anything.
	//
	// This is reached only when logs_enabled is true — see the module's
	// BuiltInPeriodicHealthCheck — so it is a one-shot command or the window
	// before the file launcher has started, never a steady state.
	if !logsmetrics.FileTailingActive() {
		return nil, errFileTailingInactive
	}

	summaries := logsmetrics.MissedBytesSnapshot()
	if len(summaries) == 0 {
		return nil, nil
	}

	var totalBytes, totalRotations int64
	var lastLossAt time.Time
	for _, s := range summaries {
		totalBytes += s.Bytes
		totalRotations += s.Rotations
		if s.LastLossAt.After(lastLossAt) {
			lastLossAt = s.LastLossAt
		}
	}

	top, omitted := rankSources(summaries)
	encoded, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("missedbytes: encode breakdown: %w", err)
	}

	hostname := c.hostname.GetSafe(context.Background())
	return []runnerdef.IssueReport{{
		IssueID:   hostIssueID(hostname),
		IssueName: IssueName,
		Source:    issueSource,
		Context: map[string]string{
			contextKeyBytes:          strconv.FormatInt(totalBytes, 10),
			contextKeyRotations:      strconv.FormatInt(totalRotations, 10),
			contextKeySourceCount:    strconv.Itoa(len(summaries)),
			contextKeySourcesOmitted: strconv.Itoa(omitted),
			contextKeyLastLossAt:     lastLossAt.UTC().Format(time.RFC3339),
			contextKeySources:        string(encoded),
		},
	}}, nil
}

// rankSources orders tuples by bytes lost, largest first, keeps at most
// maxBreakdownSources of them and returns how many it dropped. Ties break on
// source then service so the breakdown is byte-identical between ticks.
func rankSources(summaries []logsmetrics.MissedBytesSummary) ([]sourceLoss, int) {
	ranked := make([]sourceLoss, 0, len(summaries))
	for _, s := range summaries {
		ranked = append(ranked, sourceLoss{
			Source:    s.Source,
			Service:   s.Service,
			Bytes:     s.Bytes,
			Rotations: s.Rotations,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Bytes != ranked[j].Bytes {
			return ranked[i].Bytes > ranked[j].Bytes
		}
		if ranked[i].Source != ranked[j].Source {
			return ranked[i].Source < ranked[j].Source
		}
		return ranked[i].Service < ranked[j].Service
	})

	if len(ranked) <= maxBreakdownSources {
		return ranked, 0
	}
	return ranked[:maxBreakdownSources], len(ranked) - maxBreakdownSources
}

// hostIssueID scopes IssueID to this host. The backend dedups on id alone and
// ignores hostname, so without the hostname in the digest the first host whose
// window goes clean would resolve the issue for every other affected host.
func hostIssueID(hostname string) string {
	h := fnv.New64a()
	// Write never returns an error for hash.Hash.
	h.Write([]byte(hostname))
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}
