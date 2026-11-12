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
	assert.Equal(t, Unknown, SanitizeName(""))
	assert.Equal(t, Unknown, SanitizeName("\n\t"))
	assert.Equal(t, "nginx", SanitizeName("ngi\nnx"))

	long := SanitizeName(strings.Repeat("é", 200))
	assert.Equal(t, MaxNameLen, utf8.RuneCountInString(long))
	assert.True(t, strings.HasSuffix(long, ellipsis))
	assert.True(t, utf8.ValidString(long))

	assert.Equal(t, 10, utf8.RuneCountInString(SanitizeTo(strings.Repeat("a", 50), 10)))
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
