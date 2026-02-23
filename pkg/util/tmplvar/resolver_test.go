// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"errors"
	"fmt"
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResolvable is a mock implementation of the Resolvable interface for testing
type mockResolvable struct {
	serviceID   string
	hosts       map[string]string
	ports       []workloadmeta.ContainerPort
	pid         int
	hostname    string
	extraConfig map[string]string
}

func (m *mockResolvable) GetServiceID() string {
	return m.serviceID
}

func (m *mockResolvable) GetHosts() (map[string]string, error) {
	if m.hosts == nil {
		return nil, errors.New("no hosts available")
	}
	return m.hosts, nil
}

func (m *mockResolvable) GetPorts() ([]workloadmeta.ContainerPort, error) {
	if m.ports == nil {
		return nil, errors.New("no ports available")
	}
	return m.ports, nil
}

func (m *mockResolvable) GetPid() (int, error) {
	if m.pid == 0 {
		return 0, errors.New("no PID available")
	}
	return m.pid, nil
}

func (m *mockResolvable) GetHostname() (string, error) {
	if m.hostname == "" {
		return "", errors.New("no hostname available")
	}
	return m.hostname, nil
}

func (m *mockResolvable) GetExtraConfig(key string) (string, error) {
	if m.extraConfig == nil {
		return "", errors.New("no extra config available")
	}
	value, ok := m.extraConfig[key]
	if !ok {
		return "", fmt.Errorf("extra config key %q not found", key)
	}
	return value, nil
}

func TestResolveDataWithTemplateVars_JSON(t *testing.T) {
	t.Setenv("test_envvar", "test_value")
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
		name          string
		svc           *mockResolvable
		input         string
		postProcessor func(interface{}) error
		envEnabled    bool
		expected      string
		expectedErr   bool
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
			name:       "environment variable resolution",
			svc:        svc,
			envEnabled: true,
			input:      `{"env": "%%env_test_envvar%%"}`,
			expected:   `{"env": "test_value"}`,
		},
		{
			name:        "environment variable resolution disabled",
			svc:         svc,
			envEnabled:  false,
			input:       `{"test": "%%env_test_envvar%%"}`,
			expected:    `{"test": "%%env_test_envvar%%"}`,
			expectedErr: true,
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
				ports: []workloadmeta.ContainerPort{
					{Port: 6379, Name: "redis"},
					{Port: 9400, Name: "metrics"},
					{Port: 8080, Name: "http"},
				},
			},
			input:    `{"port": "%%port%%", "port_0": "%%port_0%%", "port_metrics": "%%port_metrics%%"}`,
			expected: `{"port":8080, "port_0":6379, "port_metrics":9400}`,
		},
		{
			name: "host with specific network",
			svc: &mockResolvable{
				serviceID: "test-service",
				hosts:     map[string]string{"custom": "192.168.1.1", "bridge": "172.17.0.1"},
			},
			input:    `{"host": "%%host_custom%%"}`,
			expected: `{"host":"192.168.1.1"}`,
		},
		{
			name: "host fallback to bridge",
			svc: &mockResolvable{
				serviceID: "test-service",
				hosts:     map[string]string{"custom": "192.168.1.1", "bridge": "172.17.0.1"},
			},
			input:    `{"host": "%%host%%"}`,
			expected: `{"host":"172.17.0.1"}`,
		},
		{
			name: "resolved types are non-string",
			svc: &mockResolvable{
				serviceID: "test-service",
				ports:     []workloadmeta.ContainerPort{{Port: 8080, Name: "http"}},
				pid:       1234,
			},
			input:    `{"port": "%%port%%", "pid": "%%pid%%", "port_string": "port is %%port%%"}`,
			expected: `{"port":8080, "pid":1234, "port_string":"port is 8080"}`,
		},
		{
			name:     "resolved with post-processing",
			svc:      svc,
			input:    `{"host": "%%host%%", "tags": ["existing:tag"]}`,
			expected: `{"host":"10.0.0.5", "tags":["existing:tag","injected:tag"]}`,
			postProcessor: func(tree interface{}) error {
				if typedTree, ok := tree.(map[string]interface{}); ok {
					if tags, ok := typedTree["tags"].([]interface{}); ok {
						tags = append(tags, "injected:tag")
						typedTree["tags"] = tags
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testSvc := tt.svc
			resolver := NewTemplateResolver(JSONParser, tt.postProcessor, tt.envEnabled)
			resolved, err := resolver.ResolveDataWithTemplateVars([]byte(tt.input), testSvc)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.JSONEq(t, tt.expected, string(resolved))
			}
		})
	}
}

func TestResolveDataWithTemplateVars_YAML(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-service",
		hosts:     map[string]string{"pod": "10.0.0.5"},
		ports:     []workloadmeta.ContainerPort{{Port: 8080, Name: "http"}},
		hostname:  "my-pod",
	}

	input := `
host: %%host%%
port: %%port%%
tags:
  - static
  - pod:%%hostname%%
`

	resolver := NewTemplateResolver(YAMLParser, nil, true)
	resolved, err := resolver.ResolveDataWithTemplateVars([]byte(input), svc)
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

// TestEnvVarTemplateVarsAlwaysResolveAsStrings ensures that %%env_*%% template variables resolve to strings
func TestEnvVarTemplateVarsAlwaysResolveAsStrings(t *testing.T) {
	cases := []struct {
		envValue string
		desc     string
	}{
		{"123456", "all-numeric password remains a string"},
		{"0123456", "leading-zero value is NOT silently converted to octal (42798)"},
		{"0", "zero remains a string"},
		{"true", "bool-looking value remains a string"},
		{"false", "false-looking value remains a string"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Setenv("DD_TEST_PW", tc.envValue)
			input := []byte("password: \"%%env_DD_TEST_PW%%\"\nhost: localhost\n")
			resolver := NewTemplateResolver(YAMLParser, nil, true)
			resolved, err := resolver.ResolveDataWithTemplateVars(input, nil)
			require.NoError(t, err)

			var result map[interface{}]interface{}
			require.NoError(t, YAMLParser.Unmarshal(resolved, &result))

			pw := result["password"]
			assert.IsType(t, "", pw,
				"password should be string, got %T = %v\nResolved YAML:\n%s", pw, pw, resolved)
			assert.Equal(t, tc.envValue, pw,
				"password must equal env var value exactly (no octal/type conversion)")
		})
	}
}

func TestGetHostNilResolver(t *testing.T) {
	_, err := GetHost("", nil)
	assert.Error(t, err)
	var noResolver *NoResolverError
	assert.True(t, errors.As(err, &noResolver))
}

func TestGetHostNoNetworks(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-svc",
		hosts:     map[string]string{},
	}
	_, err := GetHost("", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no network found")
}

func TestGetHostError(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-svc",
		hosts:     nil, // will trigger error in GetHosts()
	}
	_, err := GetHost("", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extract IP address")
}

func TestGetPortNilResolver(t *testing.T) {
	_, err := GetPort("", nil)
	assert.Error(t, err)
	var noResolver *NoResolverError
	assert.True(t, errors.As(err, &noResolver))
}

func TestGetPortNoPorts(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-svc",
		ports:     []workloadmeta.ContainerPort{},
	}
	_, err := GetPort("", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no port found")
}

func TestGetPortIndexOutOfRange(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-svc",
		ports:     []workloadmeta.ContainerPort{{Port: 8080}},
	}
	_, err := GetPort("5", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index given for the port template var is too big")
}

func TestGetPortNamedNotFound(t *testing.T) {
	svc := &mockResolvable{
		serviceID: "test-svc",
		ports:     []workloadmeta.ContainerPort{{Port: 8080, Name: "http"}},
	}
	_, err := GetPort("nonexistent", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port nonexistent not found")
}

func TestGetPidNilResolver(t *testing.T) {
	_, err := GetPid("", nil)
	assert.Error(t, err)
	var noResolver *NoResolverError
	assert.True(t, errors.As(err, &noResolver))
}

func TestGetPidError(t *testing.T) {
	svc := &mockResolvable{serviceID: "test-svc", pid: 0} // pid=0 triggers error
	_, err := GetPid("", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get pid")
}

func TestGetHostnameNilResolver(t *testing.T) {
	_, err := GetHostname("", nil)
	assert.Error(t, err)
	var noResolver *NoResolverError
	assert.True(t, errors.As(err, &noResolver))
}

func TestGetAdditionalTplVariablesNilResolver(t *testing.T) {
	_, err := GetAdditionalTplVariables("key", nil)
	assert.Error(t, err)
	var noResolver *NoResolverError
	assert.True(t, errors.As(err, &noResolver))
}

func TestGetEnvvarEmptyName(t *testing.T) {
	_, err := GetEnvvar("", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envvar name is missing")
}

func TestGetEnvvarEmptyNameWithResolver(t *testing.T) {
	svc := &mockResolvable{serviceID: "test-svc"}
	_, err := GetEnvvar("", svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envvar name is missing")
	assert.Contains(t, err.Error(), "test-svc")
}

func TestGetEnvvarNotFound(t *testing.T) {
	_, err := GetEnvvar("NONEXISTENT_VAR_FOR_TESTS_12345", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to retrieve envvar")
}

func TestResolveDataWithTemplateVarsEmptyData(t *testing.T) {
	resolver := NewTemplateResolver(JSONParser, nil, false)
	result, err := resolver.ResolveDataWithTemplateVars(nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveDataWithTemplateVarsInvalidVar(t *testing.T) {
	resolver := NewTemplateResolver(JSONParser, nil, false)
	svc := &mockResolvable{serviceID: "test-svc"}
	_, err := resolver.ResolveDataWithTemplateVars([]byte(`{"v": "%%unknown_var%%"}`), svc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestNoResolverError(t *testing.T) {
	err := noResolverError("test message")
	assert.Equal(t, "test message", err.Error())
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
