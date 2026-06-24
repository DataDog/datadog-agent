// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDockerConfig(t *testing.T) {
	a := &InjectorInstaller{
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
		output, err := a.setDockerConfigContent(context.TODO(), []byte(input))
		assert.Nil(t, err)
		assert.Equal(t, expected, string(output))
	}
}

func TestSetDockerConfigWithSanitizedJSON(t *testing.T) {
	a := &InjectorInstaller{
		installPath: "/tmp/stable",
	}

	expected := `{
    "default-runtime": "dd-shim",
    "raw-logs": false,
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`
	emptyExpected := `{
    "default-runtime": "dd-shim",
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "UTF8BOM",
			input: "\xEF\xBB\xBF" + `{
    "raw-logs": false
}`,
			expected: expected,
		},
		{
			name: "LineComments",
			input: `{
    // Docker 28 supports line comments in daemon.json
    "raw-logs": false // inline comments are also accepted
}`,
			expected: expected,
		},
		{
			name: "BlockComments",
			input: `{
    /*
     * Docker 28 supports block comments in daemon.json
     */
    "raw-logs": false
}`,
			expected: expected,
		},
		{
			name: "BOMAndComments",
			input: "\xEF\xBB\xBF" + `{
    // Comments outside strings should be stripped
    "raw-logs": false,
    "registry-mirrors": [
        "https://mirror.example.com/path//value/*literal*/"
    ] /* while comment markers inside strings are preserved */
}`,
			expected: `{
    "default-runtime": "dd-shim",
    "raw-logs": false,
    "registry-mirrors": [
        "https://mirror.example.com/path//value/*literal*/"
    ],
    "runtimes": {
        "dd-shim": {
            "path": "/tmp/stable/inject/auto_inject_runc"
        }
    }
}`,
		},
		{
			name: "BOMAndOnlyComments",
			input: "\xEF\xBB\xBF" + `// Docker 28 supports comment-only daemon.json files
/*
 * Empty config expressed with comments only.
 */`,
			expected: emptyExpected,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, err := a.setDockerConfigContent(context.TODO(), []byte(tc.input))
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, string(output))
		})
	}
}

func TestRemoveDockerConfig(t *testing.T) {
	a := &InjectorInstaller{
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
		{
			name: "FileWithBOMAndComments",
			input: "\xEF\xBB\xBF" + `{
    // Docker 28 supports comments in daemon.json
    "default-runtime": "dd-shim",
    "runtimes": {
        /* injected runtime */
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
			name: "FileWithOnlyBOMAndComments",
			input: "\xEF\xBB\xBF" + `// Docker 28 supports comment-only daemon.json files
/*
 * Empty config expressed with comments only.
 */`,
			expected: `{
    "default-runtime": "runc",
    "runtimes": {}
}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, err := a.deleteDockerConfigContent(context.TODO(), []byte(tc.input))
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, string(output))
		})
	}
}

func TestDockerConfigInvalidJSONError(t *testing.T) {
	a := &InjectorInstaller{
		installPath: "/tmp/stable",
	}

	tests := []struct {
		name  string
		input []byte
	}{
		{
			name: "comment stripped but invalid JSON",
			input: []byte(`{
    // The comment is stripped, but the config is still invalid.
    "raw-logs":
}`),
		},
		{
			name:  "unterminated block comment",
			input: []byte(`{"raw-logs": false /* unterminated`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("setDockerConfigContent", func(t *testing.T) {
				_, err := a.setDockerConfigContent(context.TODO(), tc.input)
				assertInvalidDockerConfigError(t, err)
			})
			t.Run("deleteDockerConfigContent", func(t *testing.T) {
				_, err := a.deleteDockerConfigContent(context.TODO(), tc.input)
				assertInvalidDockerConfigError(t, err)
			})
		})
	}
}

func assertInvalidDockerConfigError(t *testing.T, err error) {
	t.Helper()
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "/etc/docker/daemon.json appears to contain invalid JSON")
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		assert.True(t, errors.As(err, &syntaxErr) || errors.As(err, &typeErr),
			"expected wrapped *json.SyntaxError or *json.UnmarshalTypeError")
	}
}
