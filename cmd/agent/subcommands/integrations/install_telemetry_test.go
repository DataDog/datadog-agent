// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// fakeAgentTelemetry records the events sent through it, optionally blocking each send.
type fakeAgentTelemetry struct {
	events  map[string][][]byte
	unblock chan struct{}
}

func (f *fakeAgentTelemetry) SendEvent(eventType string, eventPayload []byte) error {
	if f.unblock != nil {
		<-f.unblock
	}
	if f.events == nil {
		f.events = map[string][][]byte{}
	}
	f.events[eventType] = append(f.events[eventType], eventPayload)
	return nil
}

func (f *fakeAgentTelemetry) SubmitErrorLog(errortracking.ErrorLog) {}

func (f *fakeAgentTelemetry) StartStartupSpan(string) (*installertelemetry.Span, context.Context) {
	return &installertelemetry.Span{}, context.Background()
}

func TestReadDownloaderResult(t *testing.T) {
	tests := []struct {
		name     string
		content  *string
		expected downloaderResult
	}{
		{
			name:     "fallback to v1",
			content:  ptr(`{"repository": "v1", "v2_failure": "network error"}`),
			expected: downloaderResult{Repository: "v1", V2Failure: "network error"},
		},
		{
			name:     "failure",
			content:  ptr(`{"repository": "v2", "error_type": "DigestMismatch"}`),
			expected: downloaderResult{Repository: "v2", ErrorType: "DigestMismatch"},
		},
		{name: "downloader that doesn't write the file", content: nil},
		{name: "empty file", content: ptr("")},
		{name: "malformed file", content: ptr(`{"repository": `)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "result.json")
			if tt.content != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.content), 0o600))
			}
			assert.Equal(t, tt.expected, readDownloaderResult(path))
		})
	}
}

// An unreachable intake must never hold the install for longer than the send budget.
func TestSendInstallEventGivesUpAfterTimeout(t *testing.T) {
	atel := &fakeAgentTelemetry{unblock: make(chan struct{})}
	defer close(atel.unblock)

	start := time.Now()
	sendInstallEvent(atel, installEvent{Integration: "datadog-postgres"}, 50*time.Millisecond)

	assert.Less(t, time.Since(start), time.Second)
}

func TestInstallSendsOneEventWhenTheInstallFails(t *testing.T) {
	atel := &fakeAgentTelemetry{}
	params := &cliParams{args: []string{"datadog-postgres==18.0.0"}}

	err := install(params, nil, atel)

	// Outside an Agent installation, the command fails before it parses its arguments.
	require.Error(t, err)
	require.Len(t, atel.events[installEventName], 1)
	var ev map[string]any
	require.NoError(t, json.Unmarshal(atel.events[installEventName][0], &ev))
	assert.Equal(t, "failure", ev["result"])
	assert.Equal(t, "validate", ev["stage"])
	assert.Equal(t, "core", ev["source"])
}

func ptr(s string) *string {
	return &s
}
