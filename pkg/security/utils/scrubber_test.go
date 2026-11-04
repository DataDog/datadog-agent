// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubber(t *testing.T) {
	t.Run("cmdline-default", func(t *testing.T) {
		scrubber, err := NewScrubber(nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"cmd --secret 1234567890 --token 1234567890 --jwt abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--jwt", "********"}, scrubbed)

		scrubbed = scrubber.ScrubCommand([]string{"cmd", "--secret", "1234567890", "--token", "1234567890", "--jwt", "abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--jwt", "********"}, scrubbed)
	})

	t.Run("cmdline-custom-word", func(t *testing.T) {
		scrubber, err := NewScrubber([]string{"custom"}, nil)
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"cmd --secret 1234567890 --token 1234567890 --custom abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--custom", "********"}, scrubbed)

		scrubbed = scrubber.ScrubCommand([]string{"cmd", "--secret", "1234567890", "--token", "1234567890", "--custom", "abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--custom", "********"}, scrubbed)
	})

	t.Run("cmdline-custom-regexp", func(t *testing.T) {
		scrubber, err := NewScrubber(nil, []string{"a[a-z]*c", "t.st"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"--abc 1234567890 --test 1234567890"})
		assert.Equal(t, []string{"--***** 1234567890 --***** 1234567890"}, scrubbed)
	})

	t.Run("line-custom-regexp", func(t *testing.T) {
		scrubber, err := NewScrubber(nil, []string{"a[a-z]*c", "t.st"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubLine("abc 1234567890 test 1234567890")
		assert.Equal(t, "***** 1234567890 ***** 1234567890", scrubbed)
	})
}

func TestScrubberFirstArgument(t *testing.T) {
	scrubber, err := NewScrubber(nil, nil)
	assert.NoError(t, err)

	assert.Equal(t, []string{"--password", "********"}, scrubber.ScrubCommand([]string{"--password", "hunter2"}))
	assert.Equal(t, []string{"-H", "Content-Type: application/json"}, scrubber.ScrubCommand([]string{"-H", "Content-Type: application/json"}))
	assert.Empty(t, scrubber.ScrubCommand([]string{}))
	assert.Equal(t, []string{"secrets.py", "********", "********"}, scrubber.ScrubCommand([]string{"secrets.py", "--token", "abc123"}))
	assert.Equal(t, []string{"-u", "secrets.py", "********", "********"}, scrubber.ScrubCommand([]string{"-u", "secrets.py", "--token", "abc123"}))
	assert.Equal(t, []string{"--password", "********", "-al"}, scrubber.ScrubCommand([]string{"--password", "secret", "-al"}))
}

func TestScrubberAPIKeyValues(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	const sha = "0123456789abcdef0123456789abcdef01234567"
	scrubber, err := NewScrubber(nil, nil)
	assert.NoError(t, err)

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"curl api key header",
			[]string{"-X", "POST", "https://api.datadoghq.com/api/v1/series", "-H", "Content-Type: application/json", "-H", "DD-API-KEY: " + key, "-d", "{}"},
			[]string{"-X", "POST", "https://api.datadoghq.com/api/v1/series", "-H", "Content-Type: application/json", "-H", "DD-API-KEY: ********", "-d", "{}"}},
		{"app key header without space", []string{"-H", "DD-APPLICATION-KEY:" + key}, []string{"-H", "DD-APPLICATION-KEY:********"}},
		{"lowercase header", []string{"-H", "dd-api-key: " + key}, []string{"-H", "dd-api-key: ********"}},
		{"intake query param",
			[]string{"https://http-intake.logs.datadoghq.com/api/v2/logs?dd-api-key=" + key + "&ddsource=curl"},
			[]string{"https://http-intake.logs.datadoghq.com/api/v2/logs?dd-api-key=********&ddsource=curl"}},
		{"app key assignments", []string{"run", "-e", "DD_APP_KEY=" + key, "-e", "DD_APPLICATION_KEY=" + key}, []string{"run", "-e", "DD_APP_KEY=********", "-e", "DD_APPLICATION_KEY=********"}},
		{"hyphenated flag with equals", []string{"--api-key=" + key}, []string{"--api-key=********"}},
		{"header inside sh -c script", []string{"-c", `curl -H "DD-API-KEY: ` + key + `" https://x`}, []string{"-c", `curl -H "DD-API-KEY: ********" https://x`}},
		{"windows single-string cmdline", []string{`"C:\Program Files\curl.exe" -H "DD-API-KEY: ` + key + `"`}, []string{`"C:\Program Files\curl.exe" -H "DD-API-KEY: ********"`}},
		{"header survives keyword re-split", []string{"--password", "P", "-H", "DD-API-KEY: " + key}, []string{"--password", "********", "-H", "DD-API-KEY:", "********"}},
		{"key file path untouched", []string{"--api-key-file", "/etc/datadog/key"}, []string{"--api-key-file", "/etc/datadog/key"}},
		{"bare git sha untouched", []string{"checkout", sha}, []string{"checkout", sha}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scrubber.ScrubCommand(tt.in))
		})
	}

	assert.Equal(t, "curl -H DD-API-KEY: ********", scrubber.ScrubLine("curl -H DD-API-KEY: "+key))
}
