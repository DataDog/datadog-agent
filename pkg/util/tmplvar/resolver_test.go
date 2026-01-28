// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResolvable is a mock implementation of the Resolvable interface for testing
type mockResolvable struct {
	serviceID   string
	hosts       map[string]string
	ports       []ContainerPort
	pid         int
	hostname    string
	extraConfig map[string]string
}

func (m *mockResolvable) GetServiceID() string {
	return m.serviceID
}

func (m *mockResolvable) GetHosts() (map[string]string, error) {
	if m.hosts == nil {
		return nil, fmt.Errorf("no hosts available")
	}
	return m.hosts, nil
}

func (m *mockResolvable) GetPorts() ([]ContainerPort, error) {
	if m.ports == nil {
		return nil, fmt.Errorf("no ports available")
	}
	return m.ports, nil
}

func (m *mockResolvable) GetPid() (int, error) {
	if m.pid == 0 {
		return 0, fmt.Errorf("no PID available")
	}
	return m.pid, nil
}

func (m *mockResolvable) GetHostname() (string, error) {
	if m.hostname == "" {
		return "", fmt.Errorf("no hostname available")
	}
	return m.hostname, nil
}

func (m *mockResolvable) GetExtraConfig(key string) (string, error) {
	if m.extraConfig == nil {
		return "", fmt.Errorf("no extra config available")
	}
	value, ok := m.extraConfig[key]
	if !ok {
		return "", fmt.Errorf("extra config key %q not found", key)
	}
	return value, nil
}

func TestResolveDataWithTemplateVars_JSON(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-service",
		hosts:     map[string]string{"pod": "10.0.0.5"},
		hostname:  "my-pod-123",
		extraConfig: map[string]string{
			"namespace": "default",
			"pod_name":  "my-pod-123",
			"pod_uid":   "abc-def-ghi",
		},
	}

	tests := []struct {
		name     string
		svc      *mockResolvable
		input    string
		expected string
	}{
		{
			name:     "simple host resolution",
			svc:      svc,
			input:    `{"host": "%%host%%"}`,
			expected: `{"host":"10.0.0.5"}`,
		},
		{
			name:     "hostname resolution",
			svc:      svc,
			input:    `{"pod": "%%hostname%%"}`,
			expected: `{"pod":"my-pod-123"}`,
		},
		{
			name:     "kube namespace resolution",
			svc:      svc,
			input:    `{"namespace": "%%kube_namespace%%"}`,
			expected: `{"namespace":"default"}`,
		},
		{
			name:     "multiple variables",
			svc:      svc,
			input:    `{"host": "%%host%%", "pod": "%%kube_pod_name%%", "uid": "%%kube_pod_uid%%"}`,
			expected: `{"host":"10.0.0.5","pod":"my-pod-123","uid":"abc-def-ghi"}`,
		},
		{
			name:     "variable in string value",
			svc:      svc,
			input:    `{"url": "http://%%host%%:9400/metrics"}`,
			expected: `{"url":"http://10.0.0.5:9400/metrics"}`,
		},
		{
			name:     "mixed static and template",
			svc:      svc,
			input:    `{"static": "value", "dynamic": "%%hostname%%"}`,
			expected: `{"dynamic":"my-pod-123","static":"value"}`,
		},
		{
			name: "ipv6 host in url gets bracketed",
			svc: &mockResolvable{
				serviceID: "test-service",
				hosts:     map[string]string{"pod": "::1"},
			},
			input:    `{"url": "http://%%host%%:8080/metrics"}`,
			expected: `{"url":"http://[::1]:8080/metrics"}`,
		},
		{
			name: "ipv6 host not in url is not bracketed",
			svc: &mockResolvable{
				serviceID: "test-service",
				hosts:     map[string]string{"pod": "2001:db8::1"},
			},
			input:    `{"host": "%%host%%"}`,
			expected: `{"host":"2001:db8::1"}`,
		},
		{
			name: "port resolution",
			svc: &mockResolvable{
				ports: []ContainerPort{
					{Port: 6379, Name: "redis"},
					{Port: 9400, Name: "metrics"},
					{Port: 8080, Name: "http"},
				},
			},
			input:    `{"port": "%%port%%", "port_0": "%%port_0%%", "port_metrics": "%%port_metrics%%"}`,
			expected: `{"port":8080, "port_0":6379, "port_metrics":9400}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testSvc := svc
			if tt.svc != nil {
				testSvc = tt.svc
			}
			resolved, err := ResolveDataWithTemplateVars([]byte(tt.input), testSvc, JSONParser, nil)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(resolved))
		})
	}
}

func TestResolveDataWithTemplateVars_YAML(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-service",
		hosts:     map[string]string{"pod": "10.0.0.5"},
		ports:     []ContainerPort{{Port: 8080, Name: "http"}},
		hostname:  "my-pod",
	}

	input := `
host: %%host%%
port: %%port%%
tags:
  - static
  - pod:%%hostname%%
`

	resolved, err := ResolveDataWithTemplateVars([]byte(input), svc, YAMLParser, nil)
	require.NoError(t, err)

	// Parse back to check values
	var result map[interface{}]interface{}
	err = YAMLParser.Unmarshal(resolved, &result)
	require.NoError(t, err)

	assert.Equal(t, "10.0.0.5", result["host"])
	assert.Equal(t, 8080, result["port"]) // Should be int, not string!

	tags, ok := result["tags"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, "static", tags[0])
	assert.Equal(t, "pod:my-pod", tags[1])
}

func TestResolveDataWithTemplateVars_TypeCoercion(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-service",
		ports:     []ContainerPort{{Port: 8080, Name: "http"}},
		pid:       1234,
	}

	input := `{
		"port": %%port%%,
		"pid": %%pid%%,
		"port_string": "port is %%port%%"
	}`

	resolved, err := ResolveDataWithTemplateVars([]byte(input), svc, JSONParser, nil)
	require.NoError(t, err)

	// Port and PID should be integers when they're the only value
	// But "port is 8080" should be a string
	expected := `{
		"port": 8080,
		"pid": 1234,
		"port_string": "port is 8080"
	}`

	assert.JSONEq(t, expected, string(resolved))
}

func TestGetHost(t *testing.T) {
	tests := []struct {
		name        string
		tplVar      string
		hosts       map[string]string
		expected    string
		shouldError bool
	}{
		{
			name:     "single network",
			tplVar:   "",
			hosts:    map[string]string{"pod": "10.0.0.5"},
			expected: "10.0.0.5",
		},
		{
			name:     "specific network",
			tplVar:   "custom",
			hosts:    map[string]string{"custom": "192.168.1.1", "bridge": "172.17.0.1"},
			expected: "192.168.1.1",
		},
		{
			name:     "fallback to bridge",
			tplVar:   "",
			hosts:    map[string]string{"custom": "192.168.1.1", "bridge": "172.17.0.1"},
			expected: "172.17.0.1",
		},
		{
			name:        "no hosts",
			tplVar:      "",
			hosts:       map[string]string{},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockResolvable{
				serviceID: "test-service",
				hosts:     tt.hosts,
			}

			result, err := GetHost(tt.tplVar, svc)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetFallbackHost(t *testing.T) {
	ip, err := getFallbackHost(map[string]string{"bridge": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bridge": "172.17.0.2"})
	assert.Equal(t, "172.17.0.2", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bar": "172.17.0.2"})
	assert.Equal(t, "", ip)
	assert.NotNil(t, err)
}

func TestResolveDataWithTemplateVars_WithPostProcessor(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-service",
		hosts:     map[string]string{"pod": "10.0.0.5"},
	}

	input := `{"host": "%%host%%", "tags": ["existing:tag"]}`

	// Post-processor that adds additional tags
	postProcessor := func(tree interface{}) error {
		if typedTree, ok := tree.(map[string]interface{}); ok {
			if tags, ok := typedTree["tags"].([]interface{}); ok {
				tags = append(tags, "injected:tag")
				typedTree["tags"] = tags
			}
		}
		return nil
	}

	resolved, err := ResolveDataWithTemplateVars([]byte(input), svc, JSONParser, postProcessor)
	require.NoError(t, err)

	expected := `{"host":"10.0.0.5","tags":["existing:tag","injected:tag"]}`
	assert.JSONEq(t, expected, string(resolved))
}

func TestResolveDataWithTemplateVars_InvalidJSON(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-service",
	}

	input := `{invalid json`

	_, err := ResolveDataWithTemplateVars([]byte(input), svc, JSONParser, nil)
	assert.Error(t, err)
}

func TestResolveDataWithTemplateVars_Errors(t *testing.T) {
	tests := []struct {
		name        string
		svc         *mockResolvable
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name: "invalid variable name",
			svc: &mockResolvable{
				serviceID: "test-service",
			},
			input:       `{"value": "%%invalid_var%%"}`,
			shouldError: true,
			errorMsg:    "invalid %%invalid_var%% tag",
		},
		{
			name: "missing host",
			svc: &mockResolvable{
				serviceID: "test-service",
				hosts:     nil,
			},
			input:       `{"ip": "%%host%%"}`,
			shouldError: true,
		},
		{
			name: "missing port",
			svc: &mockResolvable{
				serviceID: "test-service",
				ports:     nil,
			},
			input:       `{"port": %%port%%}`,
			shouldError: true,
		},
		{
			name:        "nil service",
			svc:         nil,
			input:       `{"host": "%%host%%"}`,
			shouldError: true,
			errorMsg:    "no service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveDataWithTemplateVars([]byte(tt.input), tt.svc, JSONParser, nil)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
