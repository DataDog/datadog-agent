// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowsagentinstability

import (
	"fmt"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

const (
	// crashThreshold is the minimum number of crashes in the time window that triggers an issue
	crashThreshold = 2

	// timeWindow is the duration to look back for crash events
	timeWindow = 24 * time.Hour
)

// Check counts recent service failures recorded by servicemain and returns an IssueReport
// when the count exceeds crashThreshold within timeWindow.
func Check() (*healthplatform.IssueReport, error) {
	count, err := servicemain.ReadRecentCrashCount(timeWindow)
	if err != nil {
		// Graceful degradation: if the counter is unreadable, don't report a false positive.
		return nil, nil //nolint:nilerr
	}

	if count <= crashThreshold {
		return nil, nil
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"crashCount": fmt.Sprintf("%d", count),
			"timeWindow": "24h",
		},
		Tags: []string{"windows", "service-crash", "stability"},
	}, nil
}
