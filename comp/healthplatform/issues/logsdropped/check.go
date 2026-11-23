// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"context"
	"errors"
	"sync"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

var errLogsAgentNotRunning = errors.New("logsdropped: logs agent not running")

// processedRetention covers the longest incident: the tracker rolls an incident
// over after 24 hours, and its last drop can trail a check by up to an hour.
const processedRetention = 25 * time.Hour

type processedSample struct {
	at    time.Time
	count int64
}

type checker struct {
	hostname hostnameinterface.Component

	// processed and now are swapped in tests.
	processed func() int64
	now       func() time.Time

	// Samples of the logs-processed counter, one per Run plus the one taken at
	// construction, so each check knows how many logs the window covers.
	mu      sync.Mutex
	samples []processedSample
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return newCheckerWith(hostname, logsmetrics.LogsProcessed.Value, time.Now)
}

func newCheckerWith(hostname hostnameinterface.Component, processed func() int64, now func() time.Time) *checker {
	// The module is built when the agent starts, so this is the baseline the first
	// check measures from.
	return &checker{
		hostname:  hostname,
		processed: processed,
		now:       now,
		samples:   []processedSample{{at: now(), count: processed()}},
	}
}

// sample records the logs-processed counter and drops samples no incident can
// still need, keeping the newest one at or before the cutoff as a base.
func (c *checker) sample() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.samples = append(c.samples, processedSample{at: now, count: c.processed()})
	cutoff := now.Add(-processedRetention)
	base := 0
	for i, s := range c.samples {
		if s.at.After(cutoff) {
			break
		}
		base = i
	}
	c.samples = c.samples[base:]
}

// processedSince returns how many logs were processed since t, measured from the
// newest sample at or before t. Samples are one check apart, so the count can start
// up to one check early, which only lowers the loss ratio slightly.
func (c *checker) processedSince(t time.Time) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	latest := c.samples[len(c.samples)-1].count
	base := c.samples[0].count
	for i := len(c.samples) - 1; i >= 0; i-- {
		if !c.samples[i].at.After(t) {
			base = c.samples[i].count
			break
		}
	}
	return latest - base
}

// mainIncidentStart is when the earliest ongoing incident at the main endpoint
// started: the stretch the loss ratio covers. Zero when the main endpoint lost
// nothing, since severity then does not depend on the ratio.
func mainIncidentStart(drops []logsmetrics.DroppedLogsSummary) time.Time {
	var start time.Time
	for _, d := range drops {
		if d.Target.Main && (start.IsZero() || d.StartedAt.Before(start)) {
			start = d.StartedAt
		}
	}
	return start
}

// Run folds every ongoing incident into one issue per host: the backend keeps a
// single row per {org, issue_type}. The issue resolves once every incident has gone
// an hour without a new drop, because the tracker then returns nothing.
func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	// Error means "state unknown" and leaves the scheduler's active ids alone; nil
	// would resolve the running agent's issue. See MarkLogsAgentRunning.
	if !logsmetrics.LogsAgentRunning() {
		return nil, errLogsAgentNotRunning
	}

	// Sampled every run, issue or not, so a new incident always has a sample at or
	// before its start.
	c.sample()

	drops := logsmetrics.DroppedLogsSnapshot()
	if len(drops) == 0 {
		return nil, nil
	}

	var processed int64
	if start := mainIncidentStart(drops); !start.IsZero() {
		processed = c.processedSince(start)
	}

	return []runnerdef.IssueReport{{
		IssueID:   logsissue.HostIssueID(IssueID, c.hostname.GetSafe(context.Background())),
		IssueName: IssueName,
		Source:    logsissue.Source,
		Context:   summarize(drops, processed).context(),
	}}, nil
}
