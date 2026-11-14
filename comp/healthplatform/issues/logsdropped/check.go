// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

var errLogsAgentNotRunning = errors.New("logsdropped: logs agent not running")

const (
	// maxBreakdownRows caps the per-source rows listed individually; totals cover all.
	maxBreakdownRows = 10
	// maxCauses caps the (reason, destination) groups listed individually.
	maxCauses = 5
)

// processedWindow matches the tracker's: drops stay in its snapshot for up to 24
// hours plus the partial bucket, so the denominator spans the same stretch.
const processedWindow = 25 * time.Hour

type processedSample struct {
	at    time.Time
	count int64
}

type checker struct {
	hostname hostnameinterface.Component

	// processed and now are swapped in tests.
	processed func() int64
	now       func() time.Time

	// samples of the logs-processed counter, one per Run plus the one taken at
	// construction, so each check knows how many logs the window covers.
	mu      sync.Mutex
	samples []processedSample
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return newCheckerWith(hostname, logsmetrics.LogsProcessed.Value, time.Now)
}

func newCheckerWith(hostname hostnameinterface.Component, processed func() int64, now func() time.Time) *checker {
	c := &checker{hostname: hostname, processed: processed, now: now}
	// The module is built when the agent starts, so this is the baseline the first
	// check measures from.
	c.samples = []processedSample{{at: now(), count: processed()}}
	return c
}

// processedInWindow records the current counter and returns how many logs were
// processed since the oldest sample still inside processedWindow.
func (c *checker) processedInWindow() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.samples = append(c.samples, processedSample{at: now, count: c.processed()})
	// Keep the newest sample at or before the cutoff as the base, drop older ones.
	cutoff := now.Add(-processedWindow)
	base := 0
	for i, s := range c.samples {
		if s.at.After(cutoff) {
			break
		}
		base = i
	}
	c.samples = c.samples[base:]
	return c.samples[len(c.samples)-1].count - c.samples[0].count
}

// Run summarises every drop in the tracker's window into one issue per host: the
// backend keeps a single row per {org, issue_type}.
func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	// Error means "state unknown" and leaves the scheduler's active ids alone; nil
	// would resolve the running agent's issue. See MarkLogsAgentRunning.
	if !logsmetrics.LogsAgentRunning() {
		return nil, errLogsAgentNotRunning
	}

	// Sampled every run, issue or not, so the window is complete when one appears.
	processed := c.processedInWindow()

	drops := logsmetrics.DroppedLogsSnapshot()
	if len(drops) == 0 {
		return nil, nil
	}

	// Totals count copies: a payload lost at two endpoints is two rows. The
	// main-endpoint totals count logs, so they are what the title reports when the
	// main endpoint lost any.
	var totalLogs, totalBytes, primaryLogs, primaryBytes int64
	var lastDropAt time.Time
	primaryAffected := false
	sources := make(map[string]struct{}, len(drops))
	primarySources := make(map[string]struct{}, len(drops))
	reasons := make(map[string]struct{}, 8)
	causes := make(map[causeKey]*cause)
	for _, d := range drops {
		totalLogs += d.Logs
		totalBytes += d.Bytes
		sources[d.Source] = struct{}{}
		reasons[string(d.Reason)] = struct{}{}
		if d.LastDropAt.After(lastDropAt) {
			lastDropAt = d.LastDropAt
		}
		// The customer's own org reads the primary endpoint; logs dropped before
		// sending never reached it either.
		if d.Kind == logsmetrics.DropTargetPrimary || d.Kind == logsmetrics.DropTargetAll {
			primaryAffected = true
			primaryLogs += d.Logs
			primaryBytes += d.Bytes
			primarySources[d.Source] = struct{}{}
		}

		key := causeKey{Reason: string(d.Reason), Destination: d.Destination, Kind: string(d.Kind)}
		cs, ok := causes[key]
		if !ok {
			cs = &cause{Reason: key.Reason, Destination: key.Destination, Kind: key.Kind}
			causes[key] = cs
		}
		cs.Logs += d.Logs
		cs.Bytes += d.Bytes
	}

	topCauses, causesOmitted := rankCauses(causes)
	topRows, rowsOmitted := rankRows(drops)

	encodedCauses, err := json.Marshal(topCauses)
	if err != nil {
		return nil, fmt.Errorf("logsdropped: encode causes: %w", err)
	}
	encodedRows, err := json.Marshal(topRows)
	if err != nil {
		return nil, fmt.Errorf("logsdropped: encode breakdown: %w", err)
	}

	hostname := c.hostname.GetSafe(context.Background())
	return []runnerdef.IssueReport{{
		IssueID:   logsissue.HostIssueID(IssueID, hostname),
		IssueName: IssueName,
		Source:    logsissue.Source,
		Context: map[string]string{
			contextKeyLogs:            strconv.FormatInt(totalLogs, 10),
			contextKeyBytes:           strconv.FormatInt(totalBytes, 10),
			contextKeySourceCount:     strconv.Itoa(len(sources)),
			contextKeyRowsOmitted:     strconv.Itoa(rowsOmitted),
			contextKeyCausesOmitted:   strconv.Itoa(causesOmitted),
			contextKeyPrimaryAffected: strconv.FormatBool(primaryAffected),
			contextKeyPrimaryLogs:     strconv.FormatInt(primaryLogs, 10),
			contextKeyPrimaryBytes:    strconv.FormatInt(primaryBytes, 10),
			contextKeyPrimarySources:  strconv.Itoa(len(primarySources)),
			// Every reason, not just the ranked causes': each one adds a remediation step.
			contextKeyReasons:    joinSorted(reasons),
			contextKeyProcessed:  strconv.FormatInt(processed, 10),
			contextKeyLastDropAt: lastDropAt.UTC().Format(time.RFC3339),
			contextKeyCauses:     string(encodedCauses),
			contextKeyDrops:      string(encodedRows),
		},
	}}, nil
}

func joinSorted(set map[string]struct{}) string {
	values := make([]string, 0, len(set))
	for v := range set {
		values = append(values, v)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

type causeKey struct {
	Reason      string
	Destination string
	Kind        string
}

// rankCauses keeps the maxCauses largest (reason, destination) groups by logs
// dropped. Ties break on reason then destination so ticks stay byte-identical.
func rankCauses(causes map[causeKey]*cause) ([]cause, int) {
	ranked := make([]cause, 0, len(causes))
	for _, c := range causes {
		ranked = append(ranked, *c)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.Logs != b.Logs {
			return a.Logs > b.Logs
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		if a.Destination != b.Destination {
			return a.Destination < b.Destination
		}
		return a.Kind < b.Kind
	})
	if len(ranked) <= maxCauses {
		return ranked, 0
	}
	return ranked[:maxCauses], len(ranked) - maxCauses
}

// rankRows keeps the maxBreakdownRows largest rows by logs dropped. The snapshot
// arrives sorted by key, and the stable sort keeps that order between ties.
func rankRows(drops []logsmetrics.DroppedLogsSummary) ([]sourceDrop, int) {
	ranked := make([]sourceDrop, 0, len(drops))
	for _, d := range drops {
		ranked = append(ranked, sourceDrop{
			Source:      d.Source,
			Service:     d.Service,
			Reason:      string(d.Reason),
			Destination: d.Destination,
			Kind:        string(d.Kind),
			Logs:        d.Logs,
			Bytes:       d.Bytes,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Logs > ranked[j].Logs
	})
	if len(ranked) <= maxBreakdownRows {
		return ranked, 0
	}
	return ranked[:maxBreakdownRows], len(ranked) - maxBreakdownRows
}
