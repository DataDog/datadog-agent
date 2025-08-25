// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestDockerFileFormat(t *testing.T) {
	parser := New()

	logMessage := message.NewMessage([]byte(`{"log":"a message","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte("a message"), msg.GetContent())
	assert.Equal(t, message.StatusError, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte(`{"log":"a second message","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte("a second message"), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte(`{"log":"a third message\n","stream":"stdout","time":"2019-06-06T16:35:55.930852913Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte("a third message"), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte("a wrong message"))
	msg, err = parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, []byte("a wrong message"), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)

	logMessage.SetContent([]byte(`{"log":"","stream":"stdout","time":"2019-06-06T16:35:55.930852914Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte(""), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852914Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte(`{"log":"\n","stream":"stdout","time":"2019-06-06T16:35:55.930852915Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte(""), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852915Z", msg.ParsingExtra.Timestamp)
}

func TestDockerFileFormatNullJSON(t *testing.T) {
	parser := New()

	// This test reproduces the bug found by fuzzing - JSON unmarshal succeeds
	// but returns nil, causing a panic when accessing log.Stream
	// Examples: the JSON literal null, arrays, primitives, etc.
	testCases := []struct {
		name  string
		input []byte
	}{
		{
			name:  "JSON literal null",
			input: []byte(`null`),
		},
		{
			name:  "JSON array",
			input: []byte(`[]`),
		},
		{
			name:  "JSON string",
			input: []byte(`"string"`),
		},
		{
			name:  "JSON number",
			input: []byte(`123`),
		},
		{
			name:  "JSON boolean true",
			input: []byte(`true`),
		},
		{
			name:  "JSON boolean false",
			input: []byte(`false`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logMessage := message.NewMessage(tc.input, nil, "", 0)

			// This should not panic
			assert.NotPanics(t, func() {
				msg, err := parser.Parse(logMessage)
				// Should return an error for these cases
				assert.NotNil(t, err)
				assert.Equal(t, message.StatusInfo, msg.Status)
				assert.Equal(t, tc.input, msg.GetContent())
			})
		})
	}
}

func TestDockerFileFormatMultipleNewlines(t *testing.T) {
	parser := New()

	// This test documents the parser's behavior with newlines. The parser
	// strips exactly ONE trailing newline (the line terminator). Multiple
	// newlines represent actual content (empty lines).
	testCases := []struct {
		name            string
		input           []byte
		expectedContent []byte
		expectedPartial bool
	}{
		{
			name:            "empty log without newline",
			input:           []byte(`{"log":"","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte(""),
			expectedPartial: false, // Empty content is not partial
		},
		{
			name:            "single newline",
			input:           []byte(`{"log":"\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte(""),
			expectedPartial: false,
		},
		{
			name:            "double newline",
			input:           []byte(`{"log":"\n\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte("\n"),
			expectedPartial: false,
		},
		{
			name:            "triple newline",
			input:           []byte(`{"log":"\n\n\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte("\n\n"),
			expectedPartial: false,
		},
		{
			name:            "text with trailing newline",
			input:           []byte(`{"log":"hello\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte("hello"),
			expectedPartial: false,
		},
		{
			name:            "text with multiple trailing newlines",
			input:           []byte(`{"log":"hello\n\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte("hello\n"),
			expectedPartial: false,
		},
		{
			name:            "text without trailing newline",
			input:           []byte(`{"log":"hello","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`),
			expectedContent: []byte("hello"),
			expectedPartial: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logMessage := message.NewMessage(tc.input, nil, "", 0)
			msg, err := parser.Parse(logMessage)
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedContent, msg.GetContent())
			assert.Equal(t, tc.expectedPartial, msg.ParsingExtra.IsPartial)
			assert.Equal(t, message.StatusInfo, msg.Status)
		})
	}
}
