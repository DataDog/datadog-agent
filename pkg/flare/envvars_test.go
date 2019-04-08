// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package flare

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvvarWhitelisting(t *testing.T) {
	testCases := []struct {
		name string
		in   map[string]string
		out  []string
	}{
		{
			name: "empty envvars",
			in:   map[string]string{},
			out:  []string{},
		},
		{
			name: "nominal case",
			in: map[string]string{
				"DOCKER_HOST":   "tcp://10.0.0.10:8888",
				"SECRET_ENVVAR": "don't pickup",
				"GOGC":          "120",
			},
			out: []string{
				"GOGC=120",
				"DOCKER_HOST=tcp://10.0.0.10:8888",
			},
		},
		{
			name: "_key env var case",
			in: map[string]string{
				"DOCKER_HOST": "tcp://10.0.0.10:8888",
				"DD_API_KEY":  "don't pickup",
				"GOGC":        "120",
			},
			out: []string{
				"GOGC=120",
				"DOCKER_HOST=tcp://10.0.0.10:8888",
			},
		},
		{
			name: "process config options",
			in: map[string]string{
				"DOCKER_HOST":              "tcp://10.0.0.10:8888",
				"DD_PROCESS_AGENT_ENABLED": "true",
				"GOGC":                     "120",
			},
			out: []string{
				"DOCKER_HOST=tcp://10.0.0.10:8888",
				"DD_PROCESS_AGENT_ENABLED=true",
				"GOGC=120",
			},
		},
		{
			name: "bound env var config",
			in: map[string]string{
				"DOCKER_HOST":                 "tcp://10.0.0.10:8888",
				"DD_HPA_WATCHER_POLLING_FREQ": "12",
				"GOGC":                        "120",
			},
			out: []string{
				"DOCKER_HOST=tcp://10.0.0.10:8888",
				"DD_HPA_WATCHER_POLLING_FREQ=12",
				"GOGC=120",
			},
		},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, test.name), func(t *testing.T) {
			os.Clearenv()
			for k, v := range test.in {
				os.Setenv(k, v)
			}

			result := getWhitelistedEnvvars()

			assert.Equal(t, len(test.out), len(result))
			for _, v := range test.out {
				assert.Contains(t, result, v)
			}
			os.Clearenv()
		})
	}
}
