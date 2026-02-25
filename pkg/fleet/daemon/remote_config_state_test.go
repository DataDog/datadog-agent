// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// TestValidatePackage tests the validatePackage function comprehensively
func TestValidatePackage(t *testing.T) {
	tests := []struct {
		name    string
		pkg     Package
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid http package",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "https://example.com/datadog-agent-7.50.0.tar",
				SHA256:  "abc123",
			},
			wantErr: false,
		},
		{
			name: "valid oci package with digest",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "oci://example.com/datadog-agent@sha256:2a5ca68f1f0a088cdf1cd1efa086ffe0ca80f8339c7fa12a7f41bbe9d1527cb6",
				SHA256:  "abc123",
			},
			wantErr: false,
		},
		{
			name: "valid oci package with registry and namespace",
			pkg: Package{
				Name:    "dd-trace-py",
				Version: "1.31.0",
				URL:     "oci://gcr.io/datadoghq/dd-trace-py@sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
			},
			wantErr: false,
		},
		{
			name: "empty package name",
			pkg: Package{
				Name:    "",
				Version: "7.50.0",
				URL:     "https://example.com/package.tar",
			},
			wantErr: true,
			errMsg:  "package name is empty",
		},
		{
			name: "empty package version",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "",
				URL:     "https://example.com/package.tar",
			},
			wantErr: true,
			errMsg:  "package version is empty",
		},
		{
			name: "empty URL",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "",
			},
			wantErr: true,
			errMsg:  "package URL is empty",
		},
		{
			name: "invalid URL format",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "://invalid-url",
			},
			wantErr: true,
			errMsg:  "could not parse package URL",
		},
		{
			name: "oci package with tag (should fail)",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "oci://example.com/datadog-agent:7.50.0",
			},
			wantErr: true,
			errMsg:  "could not parse oci digest URL",
		},
		{
			name: "oci package with tag and latest (should fail)",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "oci://example.com/datadog-agent:latest",
			},
			wantErr: true,
			errMsg:  "could not parse oci digest URL",
		},
		{
			name: "oci package without digest or tag (should fail)",
			pkg: Package{
				Name:    "datadog-agent",
				Version: "7.50.0",
				URL:     "oci://example.com/datadog-agent",
			},
			wantErr: true,
			errMsg:  "could not parse oci digest URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePackage(tt.pkg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCatalogGetPackage tests the catalog.getPackage method
func TestCatalogGetPackage(t *testing.T) {
	c := catalog{
		Packages: []Package{
			{
				Name:     "datadog-agent",
				Version:  "7.50.0",
				Arch:     "amd64",
				Platform: "linux",
				URL:      "https://example.com/agent-linux-amd64.tar",
			},
			{
				Name:     "datadog-agent",
				Version:  "7.50.0",
				Arch:     "arm64",
				Platform: "linux",
				URL:      "https://example.com/agent-linux-arm64.tar",
			},
			{
				Name:     "datadog-agent",
				Version:  "7.51.0",
				Arch:     "amd64",
				Platform: "linux",
				URL:      "https://example.com/agent-linux-amd64-7.51.tar",
			},
			{
				Name:     "dd-trace-py",
				Version:  "1.0.0",
				Arch:     "", // Empty arch means any
				Platform: "", // Empty platform means any
				URL:      "https://example.com/dd-trace-py.tar",
			},
			{
				Name:     "datadog-agent",
				Version:  "7.50.0",
				Arch:     "",
				Platform: "windows",
				URL:      "https://example.com/agent-windows.msi",
			},
		},
	}

	tests := []struct {
		name     string
		pkg      string
		version  string
		arch     string
		platform string
		wantPkg  Package
		wantOk   bool
	}{
		{
			name:     "exact match with arch and platform",
			pkg:      "datadog-agent",
			version:  "7.50.0",
			arch:     "amd64",
			platform: "linux",
			wantPkg: Package{
				Name:     "datadog-agent",
				Version:  "7.50.0",
				Arch:     "amd64",
				Platform: "linux",
				URL:      "https://example.com/agent-linux-amd64.tar",
			},
			wantOk: true,
		},
		{
			name:     "match with different arch",
			pkg:      "datadog-agent",
			version:  "7.50.0",
			arch:     "arm64",
			platform: "linux",
			wantPkg: Package{
				Name:     "datadog-agent",
				Version:  "7.50.0",
				Arch:     "arm64",
				Platform: "linux",
				URL:      "https://example.com/agent-linux-arm64.tar",
			},
			wantOk: true,
		},
		{
			name:     "match with empty arch and platform",
			pkg:      "dd-trace-py",
			version:  "1.0.0",
			arch:     "amd64",
			platform: "linux",
			wantPkg: Package{
				Name:     "dd-trace-py",
				Version:  "1.0.0",
				Arch:     "",
				Platform: "",
				URL:      "https://example.com/dd-trace-py.tar",
			},
			wantOk: true,
		},
		{
			name:     "match with empty arch in catalog",
			pkg:      "datadog-agent",
			version:  "7.50.0",
			arch:     "amd64",
			platform: "windows",
			wantPkg: Package{
				Name:     "datadog-agent",
				Version:  "7.50.0",
				Arch:     "",
				Platform: "windows",
				URL:      "https://example.com/agent-windows.msi",
			},
			wantOk: true,
		},
		{
			name:     "package not found",
			pkg:      "non-existent",
			version:  "1.0.0",
			arch:     "amd64",
			platform: "linux",
			wantPkg:  Package{},
			wantOk:   false,
		},
		{
			name:     "wrong version",
			pkg:      "datadog-agent",
			version:  "7.49.0",
			arch:     "amd64",
			platform: "linux",
			wantPkg:  Package{},
			wantOk:   false,
		},
		{
			name:     "wrong arch",
			pkg:      "datadog-agent",
			version:  "7.51.0",
			arch:     "arm64",
			platform: "linux",
			wantPkg:  Package{},
			wantOk:   false,
		},
		{
			name:     "wrong platform",
			pkg:      "datadog-agent",
			version:  "7.51.0",
			arch:     "amd64",
			platform: "darwin",
			wantPkg:  Package{},
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, ok := c.getPackage(tt.pkg, tt.version, tt.arch, tt.platform)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantPkg, pkg)
			}
		})
	}
}

// TestHandleInstallerConfigUpdate tests the installer config update handler
func TestHandleInstallerConfigUpdate(t *testing.T) {
	t.Run("valid installer config", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs["test-config-id"]
			require.True(t, ok)
			assert.Equal(t, "test-config-id", cfg.ID)
			assert.Len(t, cfg.FileOperations, 2)
			assert.Equal(t, "merge-patch", cfg.FileOperations[0].FileOperationType)
			assert.Equal(t, "/datadog.yaml", cfg.FileOperations[0].FilePath)
			return nil
		})

		configJSON := []byte(`{
			"id": "test-config-id",
			"file_operations": [
				{
					"file_op": "merge-patch",
					"file_path": "/datadog.yaml",
					"patch": {"log_level": "debug"}
				},
				{
					"file_op": "replace",
					"file_path": "/system-probe.yaml",
					"patch": {"enabled": true}
				}
			]
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(_ map[string]installerConfig) error {
			t.Fatal("should not be called")
			return nil
		})

		callback.On("applyStateCallback", "test", matchErrorStatus()).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: []byte("invalid json")},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("legacy config format - datadog.yaml", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs[""]
			require.True(t, ok)
			require.Len(t, cfg.FileOperations, 1)
			assert.Equal(t, "merge-patch", cfg.FileOperations[0].FileOperationType)
			assert.Equal(t, "/datadog.yaml", cfg.FileOperations[0].FilePath)
			assert.JSONEq(t, `{"log_level":"info"}`, string(cfg.FileOperations[0].Patch))
			return nil
		})

		configJSON := []byte(`{
			"configs": {
				"datadog.yaml": {"log_level":"info"}
			}
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("legacy config format - multiple files", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs[""]
			require.True(t, ok)
			require.Len(t, cfg.FileOperations, 3)

			// Check all three configs are present
			paths := make([]string, len(cfg.FileOperations))
			for i, op := range cfg.FileOperations {
				paths[i] = op.FilePath
				assert.Equal(t, "merge-patch", op.FileOperationType)
			}
			assert.Contains(t, paths, "/datadog.yaml")
			assert.Contains(t, paths, "/security-agent.yaml")
			assert.Contains(t, paths, "/system-probe.yaml")
			return nil
		})

		configJSON := []byte(`{
			"configs": {
				"datadog.yaml": {"log_level":"info"},
				"security-agent.yaml": {"runtime_security_config":{"enabled":true}},
				"system-probe.yaml": {"network_config":{"enabled":true}}
			}
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("legacy config format - files array", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs[""]
			require.True(t, ok)
			require.Len(t, cfg.FileOperations, 2)
			assert.Equal(t, "/custom/path.yaml", cfg.FileOperations[0].FilePath)
			assert.Equal(t, "/another/config.yaml", cfg.FileOperations[1].FilePath)
			return nil
		})

		configJSON := []byte(`{
			"files": [
				{
					"path": "/custom/path.yaml",
					"contents": {"key":"value1"}
				},
				{
					"path": "/another/config.yaml",
					"contents": {"key":"value2"}
				}
			]
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("legacy config format - all config types", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs[""]
			require.True(t, ok)
			require.Len(t, cfg.FileOperations, 5)

			paths := make([]string, len(cfg.FileOperations))
			for i, op := range cfg.FileOperations {
				paths[i] = op.FilePath
			}
			assert.Contains(t, paths, "/datadog.yaml")
			assert.Contains(t, paths, "/security-agent.yaml")
			assert.Contains(t, paths, "/system-probe.yaml")
			assert.Contains(t, paths, "/application_monitoring.yaml")
			assert.Contains(t, paths, "/otel-config.yaml")
			return nil
		})

		configJSON := []byte(`{
			"configs": {
				"datadog.yaml": {"log_level":"info"},
				"security-agent.yaml": {"enabled":true},
				"system-probe.yaml": {"enabled":true},
				"application_monitoring.yaml": {"apm":true},
				"otel-config.yaml": {"service":{"name":"test"}}
			}
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("mixed legacy and new format", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs["mixed-config"]
			require.True(t, ok)
			// Should have both explicit file_operations and legacy configs
			require.Greater(t, len(cfg.FileOperations), 1)
			return nil
		})

		configJSON := []byte(`{
			"id": "mixed-config",
			"file_operations": [
				{
					"file_op": "replace",
					"file_path": "/custom.yaml",
					"patch": {"custom":"value"}
				}
			],
			"configs": {
				"datadog.yaml": {"log_level":"debug"}
			}
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("handler returns error", func(t *testing.T) {
		callback := &callbackMock{}
		testErr := assert.AnError
		handler := handleInstallerConfigUpdate(func(_ map[string]installerConfig) error {
			return testErr
		})

		configJSON := []byte(`{
			"id": "test-config",
			"file_operations": []
		}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{
			State: state.ApplyStateError,
			Error: testErr.Error(),
		}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})

	t.Run("empty config", func(t *testing.T) {
		callback := &callbackMock{}
		handler := handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
			require.Len(t, configs, 1)
			cfg, ok := configs[""]
			require.True(t, ok)
			assert.Len(t, cfg.FileOperations, 0)
			return nil
		})

		configJSON := []byte(`{}`)

		callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

		handler(map[string]state.RawConfig{
			"test": {Config: configJSON},
		}, callback.applyStateCallback)

		callback.AssertExpectations(t)
	})
}

// matchErrorStatus returns a matcher for error status
func matchErrorStatus() interface{} {
	return mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	})
}
