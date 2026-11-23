// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsissue

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short names pass through", "nginx", "nginx"},
		{"long ASCII is truncated", strings.Repeat("a", 200),
			strings.Repeat("a", MaxNameLen-len(ellipsis)) + ellipsis},
		{"multi-byte cuts on a rune boundary", strings.Repeat("é", 200),
			strings.Repeat("é", MaxNameLen-len(ellipsis)) + ellipsis},
		{"control characters are dropped", "ng\ninx\tweb\r", "nginxweb"},
		{"a name of only control characters degrades", "\n\t\r", Unknown},
		{"an empty name degrades", "", Unknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeName(tc.in)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, utf8.RuneCountInString(got), MaxNameLen)
			assert.True(t, utf8.ValidString(got), "a multi-byte name must not be split into invalid UTF-8")
		})
	}
}

func TestHostIssueIDIsStableAndHostScoped(t *testing.T) {
	a := HostIssueID("logs-x", "host-a")
	assert.Equal(t, a, HostIssueID("logs-x", "host-a"))
	assert.NotEqual(t, a, HostIssueID("logs-x", "host-b"))
	assert.True(t, strings.HasPrefix(a, "logs-x:"))
}

func TestLogsEnabled(t *testing.T) {
	assert.False(t, LogsEnabled(config.NewMockFromYAML(t, "logs_enabled: false")))
	assert.True(t, LogsEnabled(config.NewMockFromYAML(t, "logs_enabled: true")))
	assert.True(t, LogsEnabled(config.NewMockFromYAML(t, "log_enabled: true")))
}

func TestHumanize(t *testing.T) {
	assert.Equal(t, "0 B", HumanizeBytes(-5))
	assert.Equal(t, "12,400", HumanizeCount(12400))
	assert.Equal(t, "log", Pluralize(1, "log"))
	assert.Equal(t, "logs", Pluralize(0, "log"))
}
