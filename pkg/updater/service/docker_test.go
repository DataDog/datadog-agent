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
		// File has already overriden the default runtime
		`{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        }
    }
}`: `{
    "default-runtime": "dd-shim",
    "default-runtime-backup": "containerd",
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

	for input, expected := range map[string]string{
		// Empty file, shouldn't happen but still tested
		"": `{
    "default-runtime": "runc",
    "runtimes": {}
}`,
		// File only contains the injected content
		`{
			"default-runtime": "dd-shim",
			"runtimes": {
				"dd-shim": {
					"path": "/tmp/stable/inject/auto_inject_runc"
				}
			}
		}`: `{
    "default-runtime": "runc",
    "runtimes": {}
}`,
		// File contained unrelated entries
		`{
    "cgroup-parent": "abc",
    "default-runtime": "dd-shim",
    "raw-logs": false,
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`: `{
    "cgroup-parent": "abc",
    "default-runtime": "runc",
    "raw-logs": false,
    "runtimes": {}
}`,
		// File had already overriden the default runtime
		`{
    "default-runtime": "dd-shim",
	"default-runtime-backup": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        },
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`: `{
    "default-runtime": "containerd",
    "runtimes": {
        "containerd": {
            "path": "/usr/bin/containerd"
        }
    }
}`,
	} {
		output, err := a.deleteDockerConfigContent([]byte(input))
		assert.Nil(t, err)
		assert.Equal(t, expected, string(output))
	}
}
