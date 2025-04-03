// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

type mockSecretResolver struct {
	enabled               bool
	backendCommand        string
	commandAllowGroupExec bool
	origin                map[string][]secretContext
	permissionError       error
}

func (r *mockSecretResolver) GetDebugInfo(b *bytes.Buffer) {
	b.WriteString("Mock debug info")
}

func TestSecretStatusOutput(t *testing.T) {
	require := require.New(t)

	tests := []struct {
		name       string
		assertFunc func(provider status.Provider)
	}{
		{"JSON", func(provider status.Provider) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			require.NotEmpty(stats)
			require.Contains(stats, "enabled")
		}},
		{"Text", func(provider status.Provider) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			require.NoError(err)
			require.NotEmpty(b.String())
		}},
		{"HTML", func(provider status.Provider) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			require.NoError(err)
			require.NotEmpty(b.String())
		}},
	}

	mockResolver := &mockSecretResolver{
		enabled:               true,
		backendCommand:        "/path/to/command",
		commandAllowGroupExec: false,
		origin: map[string][]secretContext{
			"handle1": {
				{
					origin: "config.yaml",
					path:   []string{"path", "to", "secret"},
				},
			},
			"handle2": {
				{
					origin: "another_config.yaml",
					path:   []string{"another", "path"},
				},
			},
		},
		permissionError: nil,
	}

	provider := &testSecretsStatus{
		resolver: mockResolver,
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			test.assertFunc(provider)
		})
	}
}

func TestSecretStatusWithDisabled(t *testing.T) {
	require := require.New(t)

	mockResolver := &mockSecretResolver{
		enabled: false,
	}

	provider := &testSecretsStatus{
		resolver: mockResolver,
	}

	disabledStats := make(map[string]interface{})
	err := provider.JSON(false, disabledStats)
	require.NoError(err)
	require.Contains(disabledStats, "enabled")
	require.Equal(false, disabledStats["enabled"])
	require.NotContains(disabledStats, "executable")
}

func TestSecretStatusWithNoBackendCommand(t *testing.T) {
	require := require.New(t)

	mockResolver := &mockSecretResolver{
		enabled:        true,
		backendCommand: "",
	}

	provider := &testSecretsStatus{
		resolver: mockResolver,
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(err)
	require.Contains(stats, "enabled")
	require.Equal(true, stats["enabled"])
	require.Contains(stats, "message")
	require.Equal("No secret_backend_command set: secrets feature is not enabled", stats["message"])
}

func TestSecretStatusHandles(t *testing.T) {
	require := require.New(t)

	mockResolver := &mockSecretResolver{
		enabled:               true,
		backendCommand:        "/path/to/command",
		commandAllowGroupExec: false,
		origin: map[string][]secretContext{
			"handle1": {
				{
					origin: "config.yaml",
					path:   []string{"path", "to", "secret"},
				},
			},
			"handle2": {
				{
					origin: "another_config.yaml",
					path:   []string{"another", "path"},
				},
				{
					origin: "third_config.yaml",
					path:   []string{"third", "path"},
				},
			},
		},
	}

	provider := &testSecretsStatus{
		resolver: mockResolver,
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(err)

	require.Contains(stats, "handles")
	handles, ok := stats["handles"].(map[string][][]string)
	require.True(ok)

	require.Contains(handles, "handle1")
	require.Len(handles["handle1"], 1)
	require.Equal("config.yaml", handles["handle1"][0][0])
	require.Equal("path/to/secret", handles["handle1"][0][1])

	require.Contains(handles, "handle2")
	require.Len(handles["handle2"], 2)
}

func TestSecretStatusWithPermissions(t *testing.T) {
	require := require.New(t)

	mockResolver := &mockSecretResolver{
		enabled:               true,
		backendCommand:        "/path/to/command",
		commandAllowGroupExec: false,
		permissionError:       nil,
	}

	provider := &testSecretsStatus{
		resolver: mockResolver,
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(err)
	require.Contains(stats, "executable_correct_permissions")
	require.Contains(stats, "executable_permissions_message")
	require.Equal(true, stats["executable_correct_permissions"])
	require.Equal("OK, the executable has the correct permissions", stats["executable_permissions_message"])

	mockResolver.permissionError = errors.New("permission denied")

	statsWithError := make(map[string]interface{})
	err = provider.JSON(false, statsWithError)
	require.NoError(err)
	require.Contains(statsWithError, "executable_permissions_message")
	require.Equal(false, statsWithError["executable_correct_permissions"])
	require.Equal("error: permission denied", statsWithError["executable_permissions_message"])
}

type testSecretsStatus struct {
	resolver *mockSecretResolver
}

func (s *testSecretsStatus) Name() string {
	return "Secrets"
}

func (s *testSecretsStatus) Section() string {
	return "secrets"
}

func (s *testSecretsStatus) populateStatus(stats map[string]interface{}) {
	r := s.resolver

	stats["enabled"] = r.enabled

	if !r.enabled {
		return
	}

	if r.backendCommand == "" {
		stats["message"] = "No secret_backend_command set: secrets feature is not enabled"
		return
	}

	stats["executable"] = r.backendCommand

	correctPermission := true
	permissionMsg := "OK, the executable has the correct permissions"
	if r.permissionError != nil {
		correctPermission = false
		permissionMsg = "error: " + r.permissionError.Error()
	}
	stats["executable_correct_permissions"] = correctPermission
	stats["executable_permissions_message"] = permissionMsg

	handleMap := make(map[string][][]string)
	orderedHandles := make([]string, 0, len(r.origin))
	for handle := range r.origin {
		orderedHandles = append(orderedHandles, handle)
	}

	for _, handle := range orderedHandles {
		contexts := r.origin[handle]
		details := make([][]string, 0, len(contexts))
		for _, context := range contexts {
			details = append(details, []string{context.origin, stringJoin(context.path, "/")})
		}
		handleMap[handle] = details
	}
	stats["handles"] = handleMap
}

func (s *testSecretsStatus) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

func (s *testSecretsStatus) Text(_ bool, buffer io.Writer) error {
	s.resolver.GetDebugInfo(buffer.(*bytes.Buffer))
	return nil
}

func (s *testSecretsStatus) HTML(_ bool, buffer io.Writer) error {
	s.resolver.GetDebugInfo(buffer.(*bytes.Buffer))
	return nil
}

func stringJoin(parts []string, separator string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += separator
		}
		result += part
	}
	return result
}
