// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package untailedfiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

var errLogsAgentNotRunning = errors.New("untailedfiles: logs agent not running")

// maxBreakdownSources caps the sources listed individually; totals cover them all.
const maxBreakdownSources = 10

type checker struct {
	hostname hostnameinterface.Component
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return &checker{hostname: hostname}
}

// Run reports the last scan that hit the open files limit within the last hour,
// as one issue per host.
func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	// Error means "state unknown" and leaves the scheduler's active ids alone; nil
	// would resolve the running agent's issue. See MarkLogsAgentRunning.
	if !logsmetrics.LogsAgentRunning() {
		return nil, errLogsAgentNotRunning
	}

	state, ok := logsmetrics.CurrentUntailedFiles()
	if !ok {
		return nil, nil
	}

	// Already sorted largest first by the tracker.
	top := make([]sourceFiles, 0, min(len(state.Sources), maxBreakdownSources))
	for _, s := range state.Sources[:min(len(state.Sources), maxBreakdownSources)] {
		top = append(top, sourceFiles{
			Source:   s.Source,
			Service:  s.Service,
			Path:     s.Path,
			Matched:  s.Matched,
			Untailed: s.Untailed,
		})
	}
	omitted := len(state.Sources) - len(top) + state.SourcesOmitted
	encoded, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("untailedfiles: encode breakdown: %w", err)
	}

	hostname := c.hostname.GetSafe(context.Background())
	return []runnerdef.IssueReport{{
		IssueID:   logsissue.HostIssueID(IssueID, hostname),
		IssueName: IssueName,
		Source:    logsissue.Source,
		Context: map[string]string{
			contextKeyLimit:          strconv.Itoa(state.Limit),
			contextKeyTailed:         strconv.Itoa(state.Tailed),
			contextKeyUntailed:       strconv.Itoa(state.Untailed),
			contextKeySourceCount:    strconv.Itoa(len(state.Sources) + state.SourcesOmitted),
			contextKeySourcesOmitted: strconv.Itoa(omitted),
			contextKeyLastSeenAt:     state.LastSeenAt.UTC().Format(time.RFC3339),
			contextKeySources:        string(encoded),
		},
	}}, nil
}
