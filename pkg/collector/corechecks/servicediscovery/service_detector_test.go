// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_serviceDetector(t *testing.T) {
	// no need to test many cases here, just ensuring the process data is properly passed down is enough.
	tests := []struct {
		name  string
		pInfo processInfo
		want  serviceMetadata
	}{
		{
			name: "basic",
			pInfo: processInfo{
				PID:     100,
				CmdLine: []string{"my-service.py"},
				Env:     []string{"PATH=testdata/test-bin", "DD_INJECTION_ENABLED=tracer"},
				Cwd:     "",
				Stat:    procStat{},
				Ports:   []int{5432},
			},
			want: serviceMetadata{
				Name:               "my-service",
				Language:           "python",
				Type:               "db",
				APMInstrumentation: "injected",
			},
		},
		{
			// pass in nil slices and see if anything blows up
			name: "empty",
			pInfo: processInfo{
				PID:     0,
				CmdLine: nil,
				Env:     nil,
				Cwd:     "",
				Stat:    procStat{},
				Ports:   nil,
			},
			want: serviceMetadata{
				Name:               "",
				Language:           "UNKNOWN",
				Type:               "web_service",
				APMInstrumentation: "none",
				FromDDService:      false,
			},
		},
		{
			name: "set_pwd_from_cwd",
			pInfo: processInfo{
				PID:     100,
				CmdLine: []string{"node", "index.js"},
				Env:     []string{},
				Cwd:     "./testdata/node",
				Stat:    procStat{},
				Ports:   []int{8000},
			},
			want: serviceMetadata{
				Name:               "node-service",
				Language:           "nodejs",
				Type:               "web_service",
				APMInstrumentation: "none",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sd := newServiceDetector()
			got := sd.Detect(tc.pInfo)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_ensureEnvWithPWD(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		cwd  string
		want []string
	}{
		{
			name: "no_pwd",
			env:  []string{"PATH=/some/path", "FOO=bar"},
			cwd:  "/current/workdir",
			want: []string{"PATH=/some/path", "FOO=bar", "PWD=/current/workdir"},
		},
		{
			name: "empty_env",
			env:  []string{},
			cwd:  "/current/workdir",
			want: []string{"PWD=/current/workdir"},
		},
		{
			name: "has_pwd",
			env:  []string{"PATH=/some/path", "FOO=bar", "PWD=/existing/pwd"},
			cwd:  "/current/workdir",
			want: []string{"PATH=/some/path", "FOO=bar", "PWD=/existing/pwd"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureEnvWithPWD(tc.env, tc.cwd)
			assert.Equal(t, tc.want, got)
		})
	}
}
