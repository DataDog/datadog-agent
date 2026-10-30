// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logsignature

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/tokenizer"
)

// TestGenerator_Generate pins the signatures that name the
// log.pattern.<hash>.count series: if one changes, the series is renamed.
func TestGenerator_Generate(t *testing.T) {
	assert.Equal(t, 1, tokenizer.ProfileVersion, "the Logs tokenizer output changed, which renames log.pattern series: tell AAD, then update the signatures below and this pin")

	// One generator for every case, so its tokenizer is reused across logs.
	g := NewGenerator(0)
	for _, tt := range []struct{ content, want string }{
		{"", ""},
		{"Request completed in 45ms", "CCCCCCC CCCCCCCCC CC DDCC"},
		{"Mon Jan 15 14:30:00 UTC 2024", "DAY MTH DD DD:DD:DD ZONE DDDD"},
		{"ERROR: connection failed", "ERROR: CCCCCCCCCC FAILURE"},
		{"WARNING timeout", "WARN TIMEOUT"},
		{"accepted from 10.0.0.1", "CCCCCCCC CCCC IPv4"},
		{"accepted from 192.168.100.200:8080", "CCCCCCCC CCCC IPv4:DDDD"},
		{"v1.2.3.4.5 error0.0.0.0", "CIPv4.D ERRORIPv4"},
		{"voilà ok", "CCCCCC CC"},
	} {
		assert.Equal(t, tt.want, g.Generate(tt.content), "content %q", tt.content)
	}

	capped := NewGenerator(8)
	assert.Equal(t, "IPv4", capped.Generate("10.0.0.1 x"))
	assert.Equal(t, "CCCCCCCC", capped.Generate("abcdefgh 123"))
}

// TestGenerator_GenerateAllocations pins Generate's per-log cost on the shared
// tokenizer: one allocation, the signature string, at every line length. Each
// call gets a log it has not seen, so a cache keyed by content would count too.
func TestGenerator_GenerateAllocations(t *testing.T) {
	g := NewGenerator(4096)
	for _, line := range []string{
		"short",
		"2024-01-15 10:30:45 ERROR request from 10.0.0.1 failed after 3 retries",
		strings.Repeat("user_id=1234567 status=500 path=/api/v1/charge ", 200),
	} {
		logs := make([]string, 101) // AllocsPerRun's warm-up call plus its 100 runs
		for i := range logs {
			logs[i] = strconv.Itoa(i) + " " + line
		}
		n := 0
		allocs := testing.AllocsPerRun(100, func() {
			_ = g.Generate(logs[n])
			n++
		})
		assert.Equal(t, 1.0, allocs, "%d-byte log", len(line))
	}
}
