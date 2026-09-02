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
	"math"
	"sort"
	"strconv"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

var errLogsAgentNotRunning = errors.New("missedbytes: logs agent not running")

// maxBreakdownSources caps the tuples listed individually; totals cover them all.
const maxBreakdownSources = 10

// maxBackpressureComponents caps the components listed individually; a 4-pipeline agent with
// several destinations produces roughly twice this many rows.
const maxBackpressureComponents = 10

// ratioDecimals rounds the ratios so an unchanged pipeline encodes identically each tick.
const ratioDecimals = 3

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
	// Counted here, not in BuildIssue, which only receives the capped breakdown.
	distinctSources := make(map[string]struct{}, len(summaries))
	for _, s := range summaries {
		totalBytes += s.Bytes
		totalRotations += s.Rotations
		distinctSources[s.Source] = struct{}{}
		if s.LastLossAt.After(lastLossAt) {
			lastLossAt = s.LastLossAt
		}
	}

	top, omitted := rankSources(summaries)
	encoded, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("missedbytes: encode breakdown: %w", err)
	}

	issueContext := map[string]string{
		contextKeyBytes:        strconv.FormatInt(totalBytes, 10),
		contextKeyRotations:    strconv.FormatInt(totalRotations, 10),
		contextKeySourceCount:  strconv.Itoa(len(distinctSources)),
		contextKeyPairsOmitted: strconv.Itoa(omitted),
		contextKeyLastLossAt:   lastLossAt.UTC().Format(time.RFC3339),
		contextKeySources:      string(encoded),
	}

	// Enrichment only: an unreadable pipeline drops the key rather than failing the check.
	if encodedBP, ok := encodeBackpressure(logsmetrics.BackpressureSnapshot()); ok {
		issueContext[contextKeyBackpressure] = encodedBP
	}

	hostname := c.hostname.GetSafe(context.Background())
	return []runnerdef.IssueReport{{
		IssueID:   hostIssueID(hostname),
		IssueName: IssueName,
		Source:    issueSource,
		Context:   issueContext,
	}}, nil
}

// encodeBackpressure ranks, caps and rounds the snapshot for the wire. Reports false when no
// monitor answered, which is not the same as a healthy pipeline.
func encodeBackpressure(summary logsmetrics.BackpressureSummary) (string, bool) {
	if summary.State == "" {
		return "", false
	}

	// Already ranked worst-first, so truncating keeps the saturated rows.
	components := summary.Components
	omitted := 0
	if len(components) > maxBackpressureComponents {
		omitted = len(components) - maxBackpressureComponents
		components = components[:maxBackpressureComponents]
	}

	wire := backpressureWire{
		State:             summary.State,
		Components:        make([]logsmetrics.ComponentBackpressure, 0, len(components)),
		ComponentsOmitted: omitted,
	}
	for _, c := range components {
		wire.Components = append(wire.Components, roundComponent(c))
	}
	if summary.Bottleneck != nil {
		rounded := roundComponent(*summary.Bottleneck)
		wire.Bottleneck = &rounded
	}

	encoded, err := json.Marshal(wire)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func roundComponent(c logsmetrics.ComponentBackpressure) logsmetrics.ComponentBackpressure {
	c.AvgRatio = roundRatio(c.AvgRatio)
	c.Max5m = roundRatio(c.Max5m)
	c.Max30m = roundRatio(c.Max30m)
	c.Max2h = roundRatio(c.Max2h)
	c.Max5h = roundRatio(c.Max5h)
	c.Max10h = roundRatio(c.Max10h)
	return c
}

func roundRatio(v float64) float64 {
	scale := math.Pow(10, ratioDecimals)
	return math.Round(v*scale) / scale
}

// dominantBottleneck names the stage blamed for most of a tuple's rotations, and how many.
// Ties break on the stage name so consecutive ticks stay byte-identical.
func dominantBottleneck(counts map[string]int64) (string, int64) {
	var name string
	var max int64
	for component, count := range counts {
		if count > max || (count == max && component < name) {
			name, max = component, count
		}
	}
	return name, max
}

// rankSources keeps the maxBreakdownSources largest tuples and returns how many it
// dropped. Ties break on source then service so ticks stay byte-identical.
func rankSources(summaries []logsmetrics.MissedBytesSummary) ([]sourceLoss, int) {
	ranked := make([]sourceLoss, 0, len(summaries))
	for _, s := range summaries {
		bottleneck, bottleneckRotations := dominantBottleneck(s.Bottlenecks)
		ranked = append(ranked, sourceLoss{
			Source:              s.Source,
			Service:             s.Service,
			Bytes:               s.Bytes,
			Rotations:           s.Rotations,
			Bottleneck:          bottleneck,
			BottleneckRotations: bottleneckRotations,
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
