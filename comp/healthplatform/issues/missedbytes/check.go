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

// errLogsAgentNotRunning means the check cannot establish state either way.
var errLogsAgentNotRunning = errors.New("missedbytes: logs agent not running")

// maxBreakdownSources caps the tuples listed individually; totals cover them all.
const maxBreakdownSources = 10

type checker struct {
	hostname hostnameinterface.Component
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return &checker{hostname: hostname}
}

// Run summarises every tuple in the tracker's window into one issue: the backend
// keeps a single row per {org, issue_type}. Always empty on Windows, where the
// tailer holds no os.File to size a loss with.
func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	// Error means "state unknown" and leaves the scheduler's active ids alone; nil
	// would resolve the running agent's issue. See MarkLogsAgentRunning.
	if !logsmetrics.LogsAgentRunning() {
		return nil, errLogsAgentNotRunning
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

// rankSources keeps the maxBreakdownSources largest tuples and returns how many it
// dropped. Ties break on source then service so ticks stay byte-identical.
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

// hostIssueID scopes IssueID to this host. The backend dedups on id alone, so
// without the digest one host going clean resolves the issue for every host.
func hostIssueID(hostname string) string {
	h := fnv.New64a()
	h.Write([]byte(hostname)) // never returns an error for hash.Hash
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}
