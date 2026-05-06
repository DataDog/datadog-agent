// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
)

func makeTestProvider(t *testing.T, serverURL string, checkTemplate string) *PrometheusHTTPSDConfigProvider {
	t.Helper()

	var tmpl httpSDCheckTemplate
	require.NoError(t, json.Unmarshal([]byte(checkTemplate), &tmpl))

	return &PrometheusHTTPSDConfigProvider{
		url:           serverURL,
		client:        http.DefaultClient,
		checkTemplate: tmpl,
		configErrors:  make(map[string]types.ErrorMsgSet),
	}
}

func defaultCheckTemplate() string {
	return `{"name":"openmetrics","init_config":{},"instances":[{"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}]}`
}

func TestCollectBasicTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"10.0.0.1:9090", "10.0.0.2:9090"},
				Labels:  map[string]string{"job": "web", "__meta_environment": "prod"},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 2)

	// Verify first config
	assert.Equal(t, "openmetrics", configs[0].Name)
	assert.True(t, configs[0].ClusterCheck)
	assert.Equal(t, "prometheus_http_sd:"+server.URL, configs[0].Source)

	// Verify instance has substituted values
	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))
	assert.Equal(t, "http://10.0.0.1:9090/metrics", instance["openmetrics_endpoint"])

	var instance2 map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[1].Instances[0], &instance2))
	assert.Equal(t, "http://10.0.0.2:9090/metrics", instance2["openmetrics_endpoint"])
}

func TestCollectMultipleTargetGroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"10.0.0.1:8080"}, Labels: map[string]string{"job": "web"}},
			{Targets: []string{"10.0.0.2:5432", "10.0.0.3:5432"}, Labels: map[string]string{"job": "db"}},
		})
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 3)
}

func TestCollectEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{})
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestCollectServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs, err := provider.Collect(context.Background())
	assert.Error(t, err)
	assert.Nil(t, configs)
	assert.Contains(t, err.Error(), "unexpected status 500")

	// Verify config errors are tracked
	errs := provider.GetConfigErrors()
	assert.NotEmpty(t, errs["fetch"])
}

func TestLabelsToTags(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected []string
	}{
		{
			name:     "plain labels become tags",
			labels:   map[string]string{"job": "web", "env": "prod"},
			expected: []string{"env:prod", "job:web"},
		},
		{
			name:     "__meta_ prefix is stripped",
			labels:   map[string]string{"__meta_environment": "staging", "__meta_team": "backend"},
			expected: []string{"environment:staging", "team:backend"},
		},
		{
			name:     "__ internal labels are skipped",
			labels:   map[string]string{"__scheme__": "https", "__address__": "10.0.0.1:9090", "job": "web"},
			expected: []string{"job:web"},
		},
		{
			name:     "mixed labels",
			labels:   map[string]string{"__meta_dc": "us-east", "__scheme__": "http", "app": "nginx"},
			expected: []string{"app:nginx", "dc:us-east"},
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: nil,
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := labelsToTags(tt.labels)
			assert.Equal(t, tt.expected, tags)
		})
	}
}

func TestLabelsToTagsStableOrder(t *testing.T) {
	labels := map[string]string{
		"z_label": "last",
		"a_label": "first",
		"m_label": "middle",
	}

	// Run multiple times to verify deterministic ordering
	for i := 0; i < 10; i++ {
		tags := labelsToTags(labels)
		assert.Equal(t, []string{"a_label:first", "m_label:middle", "z_label:last"}, tags)
	}
}

func TestSubstituteTemplateVars(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		host     string
		port     string
		expected interface{}
	}{
		{
			name:     "string substitution",
			input:    "http://%%host%%:%%port%%/metrics",
			host:     "10.0.0.1",
			port:     "9090",
			expected: "http://10.0.0.1:9090/metrics",
		},
		{
			name:     "no placeholders",
			input:    "static_value",
			host:     "10.0.0.1",
			port:     "9090",
			expected: "static_value",
		},
		{
			name:     "non-string passthrough",
			input:    42,
			host:     "10.0.0.1",
			port:     "9090",
			expected: 42,
		},
		{
			name:     "empty port",
			input:    "http://%%host%%:%%port%%/metrics",
			host:     "10.0.0.1",
			port:     "",
			expected: "http://10.0.0.1:/metrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteTemplateVars(tt.input, tt.host, tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildConfigTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"10.0.0.1:9090"},
				Labels: map[string]string{
					"__meta_environment": "production",
					"__meta_team":        "platform",
					"__scheme__":         "https",
					"job":                "api-server",
				},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))

	// Verify tags: __meta_ stripped, __scheme__ skipped, plain labels kept
	tags, ok := instance["tags"].([]interface{})
	require.True(t, ok)

	tagStrings := make([]string, len(tags))
	for i, t := range tags {
		tagStrings[i] = t.(string)
	}
	assert.Contains(t, tagStrings, "environment:production")
	assert.Contains(t, tagStrings, "team:platform")
	assert.Contains(t, tagStrings, "job:api-server")
	// __scheme__ should NOT be present
	for _, tag := range tagStrings {
		assert.NotContains(t, tag, "scheme")
	}
}

func TestCollectTargetWithoutPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"myhost"}},
		})
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))
	assert.Equal(t, "http://myhost:/metrics", instance["openmetrics_endpoint"])
}

func TestCollectDigestStability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"10.0.0.1:9090"},
				Labels:  map[string]string{"z": "last", "a": "first", "m": "middle"},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProvider(t, server.URL, defaultCheckTemplate())

	configs1, err := provider.Collect(context.Background())
	require.NoError(t, err)

	configs2, err := provider.Collect(context.Background())
	require.NoError(t, err)

	// Digests should be identical across polls (stable tag ordering)
	require.Len(t, configs1, 1)
	require.Len(t, configs2, 1)
	assert.Equal(t, configs1[0].Digest(), configs2[0].Digest())
}

func TestCollectCustomCheckTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"db.example.com:5432"}},
		})
	}))
	defer server.Close()

	tmpl := `{"name":"postgres","init_config":{"dbm":true},"instances":[{"host":"%%host%%","port":"%%port%%","dbname":"mydb"}]}`
	provider := makeTestProvider(t, server.URL, tmpl)

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "postgres", configs[0].Name)

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))
	assert.Equal(t, "db.example.com", instance["host"])
	assert.Equal(t, "5432", instance["port"])
	assert.Equal(t, "mydb", instance["dbname"])

	var initConfig map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].InitConfig, &initConfig))
	assert.Equal(t, true, initConfig["dbm"])
}
