// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityevents

import "testing"

func TestSeverityLevelNames(t *testing.T) {
	cases := []struct {
		level SeverityLevel
		name  string
	}{
		{SeverityLow, "low"},
		{SeverityMedium, "medium"},
		{SeverityHigh, "high"},
		{SeverityLevel(-1), "unknown"},
		{SeverityLevel(NumSeverityLevels), "unknown"},
	}

	for _, tc := range cases {
		if got := tc.level.String(); got != tc.name {
			t.Errorf("SeverityLevel(%d).String() = %q, want %q", tc.level, got, tc.name)
		}
	}
}
