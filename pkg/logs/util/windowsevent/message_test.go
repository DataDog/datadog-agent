// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windowsevent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasTruncatedFlag(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "empty message",
			message: "",
			want:    false,
		},
		{
			name:    "short message",
			message: "short",
			want:    false,
		},
		{
			name:    "normal message",
			message: "This is a normal log message",
			want:    false,
		},
		{
			name:    "message truncated at end",
			message: "This is a very long message that gets truncated...TRUNCATED...",
			want:    true,
		},
		{
			name:    "message truncated at beginning",
			message: "...TRUNCATED...This is a message that was truncated at the beginning",
			want:    true,
		},
		{
			name:    "message exactly truncated flag",
			message: "...TRUNCATED...",
			want:    true,
		},
		{
			name:    "message with truncated flag at both ends",
			message: "...TRUNCATED...content...TRUNCATED...",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTruncatedFlag(tt.message)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetAttribute(t *testing.T) {
	const eventXML = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Level>4</Level><Channel>System</Channel><Computer>windows-n7iefg2</Computer></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`

	m, err := NewMapXML([]byte(eventXML))
	require.NoError(t, err)
	require.NoError(t, m.SetLevel("Warning"))
	require.NoError(t, m.SetMessage("Some message"))

	msg := &Message{data: m}

	tests := []struct {
		name  string
		path  string
		want  string
		found bool
	}{
		{"system scalar", "Event.System.EventID", "7036", true},
		{"nested attribute", "Event.System.Provider.Name", "Service Control Manager", true},
		{"normalized qualifier", "Event.System.EventIDQualifier", "16384", true},
		{"named event data", "Event.EventData.Data.param1", "Windows Event Log", true},
		{"datadog level field", "level", "Warning", true},
		{"datadog message field", "message", "Some message", true},
		{"missing path", "Event.System.DoesNotExist", "", false},
		{"subtree is not a scalar", "Event.System", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := msg.GetAttribute(tt.path)
			assert.Equal(t, tt.found, ok)
			assert.Equal(t, tt.want, val)
		})
	}
}
