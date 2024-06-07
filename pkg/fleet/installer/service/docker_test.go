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

func TestSetDockerConfig(t *testing.T) {
	a := &apmInjectorInstaller{
		installPath: "/tmp/stable",
	}

	for input, expected := range map[string]string{
		// File doesn't exist
		"": `{
    "default-runtime": "dd-shim",
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
		// File contains unrelated entries
		`{
    "cgroup-parent": "abc",
    "raw-logs": false
}`: `{
    "cgroup-parent": "abc",
    "default-runtime": "dd-shim",
    "raw-logs": false,
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
		// File has already overridden the default runtime
		`{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        }
    }
}`: `{
    "default-runtime": "dd-shim",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        },
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
	} {
		output, err := a.setDockerConfigContent([]byte(input))
		assert.Nil(t, err)
		assert.Equal(t, expected, string(output))
	}
}

func TestRemoveDockerConfig(t *testing.T) {
	a := &apmInjectorInstaller{
		installPath: "/tmp/stable",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:  "EmptyFile",
			input: "",
			expected: `{
    "default-runtime": "runc",
    "runtimes": {}
}`,
		},
		{
			name: "FileOnlyWithInjectedContent",
			input: `{
    "default-runtime": "dd-shim",
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
			expected: `{
    "default-runtime": "runc",
    "runtimes": {}
}`,
		},
		{
			name: "FileWithUnrelatedEntries",
			input: `{
    "cgroup-parent": "abc",
    "default-runtime": "dd-shim",
    "raw-logs": false,
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
			expected: `{
    "cgroup-parent": "abc",
    "default-runtime": "runc",
    "raw-logs": false,
    "runtimes": {}
}`,
		},
		{
			name: "FileWithOverriddenDefaultRuntime",
			input: `{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        },
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
			expected: `{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        }
    }
}`,
		},
		{
			name: "ReplaceOldInjectedContent",
			input: `{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        },
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
			expected: `{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        }
    }
}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, err := a.deleteDockerConfigContent([]byte(tc.input))
			assert.Nil(t, err)
			assert.Equal(t, tc.expected, string(output))
		})
	}
}
