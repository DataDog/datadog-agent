// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetLDPreloadConfig(t *testing.T) {
	a := &apmInjectorInstaller{
		installPath: "/tmp/stable",
	}
	testCases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "File doesn't exist",
			input:    nil,
			expected: []byte("/tmp/stable/inject/launcher.preload.so\n"),
		},
		{
			name:     "Don't reuse the input buffer",
			input:    make([]byte, 2, 1000),
			expected: append([]byte{0x0, 0x0}, []byte("\n/tmp/stable/inject/launcher.preload.so\n")...),
		},
		{
			name:     "File contains unrelated entries",
			input:    []byte("/abc/def/preload.so\n"),
			expected: []byte("/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n"),
		},
		{
			name:     "File contains unrelated entries with no newline",
			input:    []byte("/abc/def/preload.so"),
			expected: []byte("/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n"),
		},
		{
			name:     "File contains old preload instructions",
			input:    []byte("banana\n/opt/datadog/apm/inject/launcher.preload.so\ntomato"),
			expected: []byte("banana\n/tmp/stable/inject/launcher.preload.so\ntomato"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := a.setLDPreloadConfigContent(tc.input)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected, output)
			if len(tc.input) > 0 {
				assert.False(t, &tc.input[0] == &output[0])
			}
		})
	}
}

func TestRemoveLDPreloadConfig(t *testing.T) {
	a := &apmInjectorInstaller{
		installPath: "/tmp/stable",
	}

	for input, expected := range map[string]string{
		// File doesn't exist
		"": "",
		// File only contains the entry to remove
		"/tmp/stable/inject/launcher.preload.so\n": "",
		// File only contains the entry to remove without newline
		"/tmp/stable/inject/launcher.preload.so": "",
		// File contains unrelated entries
		"/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n": "/abc/def/preload.so\n",
		// File contains unrelated entries at the end
		"/tmp/stable/inject/launcher.preload.so\n/def/abc/preload.so": "/def/abc/preload.so",
		// File contains multiple unrelated entries
		"/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n/def/abc/preload.so": "/abc/def/preload.so\n/def/abc/preload.so",
		// File contains unrelated entries with no newline (reformatted by customer?)
		"/abc/def/preload.so /tmp/stable/inject/launcher.preload.so": "/abc/def/preload.so",
		// File contains unrelated entries with no newline (reformatted by customer?)
		"/abc/def/preload.so /tmp/stable/inject/launcher.preload.so /def/abc/preload.so": "/abc/def/preload.so /def/abc/preload.so",
		// File contains unrelated entries with no newline (reformatted by customer?)
		"/tmp/stable/inject/launcher.preload.so /def/abc/preload.so": "/def/abc/preload.so",
		// File doesn't contain the entry to remove (removed by customer?)
		"/abc/def/preload.so /def/abc/preload.so": "/abc/def/preload.so /def/abc/preload.so",
	} {
		output, err := a.deleteLDPreloadConfigContent([]byte(input))
		assert.Nil(t, err)
		assert.Equal(t, expected, string(output))
	}

	// File is badly formatted (non-breaking space instead of space)
	input := "/tmp/stable/inject/launcher.preload.so\u00a0/def/abc/preload.so"
	output, err := a.deleteLDPreloadConfigContent([]byte(input))
	assert.NotNil(t, err)
	assert.Equal(t, "", string(output))
	assert.NotEqual(t, input, string(output))
}

func TestSetAgentConfig(t *testing.T) {
	a := &apmInjectorInstaller{
		installPath: "/tmp/stable",
	}

	for input, expected := range map[string]string{
		// File doesn't exist
		"": `
# BEGIN LD PRELOAD CONFIG
apm_config:
  receiver_socket: /tmp/stable/inject/run/apm.socket
use_dogstatsd: true
dogstatsd_socket: /tmp/stable/inject/run/dsd.socket
# END LD PRELOAD CONFIG
`,
		// File contains unrelated entries
		`api_key: 000000000
site: datad0g.com`: `api_key: 000000000
site: datad0g.com
# BEGIN LD PRELOAD CONFIG
apm_config:
  receiver_socket: /tmp/stable/inject/run/apm.socket
use_dogstatsd: true
dogstatsd_socket: /tmp/stable/inject/run/dsd.socket
# END LD PRELOAD CONFIG
`,
		// File already contains the agent config
		`# BEGIN LD PRELOAD CONFIG
apm_config:
  receiver_socket: /tmp/stable/inject/run/apm.socket
use_dogstatsd: true
dogstatsd_socket: /tmp/stable/inject/run/dsd.socket
# END LD PRELOAD CONFIG`: `# BEGIN LD PRELOAD CONFIG
apm_config:
  receiver_socket: /tmp/stable/inject/run/apm.socket
use_dogstatsd: true
dogstatsd_socket: /tmp/stable/inject/run/dsd.socket
# END LD PRELOAD CONFIG`,
	} {
		output, _ := a.setAgentConfigContent([]byte(input))
		assert.Equal(t, expected, string(output))
	}
}
