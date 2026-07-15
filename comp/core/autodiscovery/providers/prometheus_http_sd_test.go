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
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

type entrySpec struct {
	url           string
	template      string
	excludeFilter string
}

func makeTestProvider(t *testing.T, serverURL string, checkTemplate string) *PrometheusHTTPSDConfigProvider {
	t.Helper()
	return makeTestProviderWithEntries(t, []entrySpec{{url: serverURL, template: checkTemplate}})
}

func makeTestProviderWithEntries(t *testing.T, specs []entrySpec) *PrometheusHTTPSDConfigProvider {
	t.Helper()

	entries := make([]*httpSDEntry, len(specs))
	for i, s := range specs {
		var tmpl httpSDCheckTemplate
		require.NoError(t, json.Unmarshal([]byte(s.template), &tmpl))
		filterProg, err := compileExcludeFilter(s.excludeFilter)
		require.NoError(t, err)
		entries[i] = &httpSDEntry{
			url:           s.url,
			client:        http.DefaultClient,
			checkTemplate: tmpl,
			filterProgram: filterProg,
		}
	}

	return &PrometheusHTTPSDConfigProvider{
		entries:      entries,
		configErrors: make(map[string]types.ErrorMsgSet),
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

	// Verify config errors are tracked under the per-entry key
	errs := provider.GetConfigErrors()
	assert.NotEmpty(t, errs["fetch:"+server.URL])
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

func TestBuildEntriesMultiple(t *testing.T) {
	entries, errs := buildEntries(
		[]httpSDConfigEntry{
			{URL: "http://a/sd", CheckTemplate: defaultCheckTemplate()},
			{URL: "http://b/sd", CheckTemplate: defaultCheckTemplate()},
		},
		http.DefaultClient,
	)
	assert.Empty(t, errs)
	require.Len(t, entries, 2)
	assert.Equal(t, "http://a/sd", entries[0].url)
	assert.Equal(t, "http://b/sd", entries[1].url)
}

func TestBuildEntriesEmpty(t *testing.T) {
	entries, errs := buildEntries(nil, http.DefaultClient)
	assert.Empty(t, entries)
	assert.Empty(t, errs)
}

func TestBuildEntriesMissingURL(t *testing.T) {
	_, errs := buildEntries(
		[]httpSDConfigEntry{{URL: "", CheckTemplate: defaultCheckTemplate()}},
		http.DefaultClient,
	)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "url")
}

func TestBuildEntriesMissingTemplate(t *testing.T) {
	_, errs := buildEntries(
		[]httpSDConfigEntry{{URL: "http://x/sd", CheckTemplate: ""}},
		http.DefaultClient,
	)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "check_template")
}

func TestBuildEntriesPartialFailure(t *testing.T) {
	// One valid entry and one with a missing URL — only the valid one is returned.
	entries, errs := buildEntries(
		[]httpSDConfigEntry{
			{URL: "http://a/sd", CheckTemplate: defaultCheckTemplate()},
			{URL: "", CheckTemplate: defaultCheckTemplate()},
		},
		http.DefaultClient,
	)
	require.Len(t, entries, 1)
	assert.Equal(t, "http://a/sd", entries[0].url)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "url")
}

func TestCollectPerEntryErrorIsolation(t *testing.T) {
	// good server returns one target
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"healthy:9090"}},
		})
	}))
	defer good.Close()

	// bad server always 500s
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{
		{url: good.URL, template: defaultCheckTemplate()},
		{url: bad.URL, template: defaultCheckTemplate()},
	})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err, "Collect must succeed when at least one entry succeeds")
	require.Len(t, configs, 1, "only the healthy entry's target should be returned")

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))
	assert.Equal(t, "http://healthy:9090/metrics", instance["openmetrics_endpoint"])

	// failing entry is surfaced via GetConfigErrors under its URL-keyed slot
	errs := provider.GetConfigErrors()
	assert.NotEmpty(t, errs["fetch:"+bad.URL])
	assert.Empty(t, errs["fetch:"+good.URL])
}

func TestCollectAllEntriesFail(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{
		{url: bad.URL, template: defaultCheckTemplate()},
		{url: bad.URL, template: defaultCheckTemplate()},
	})

	configs, err := provider.Collect(context.Background())
	assert.Error(t, err)
	assert.Nil(t, configs)
	assert.Contains(t, err.Error(), "all 2 endpoint(s) failed")
}

func TestCollectMergesEntries(t *testing.T) {
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"a1:9090"}, Labels: map[string]string{"src": "A"}},
		})
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"b1:9090", "b2:9090"}, Labels: map[string]string{"src": "B"}},
		})
	}))
	defer serverB.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{
		{url: serverA.URL, template: defaultCheckTemplate()},
		{url: serverB.URL, template: defaultCheckTemplate()},
	})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 3)

	endpoints := make([]string, len(configs))
	for i, cfg := range configs {
		var instance map[string]interface{}
		require.NoError(t, yaml.Unmarshal(cfg.Instances[0], &instance))
		endpoints[i] = instance["openmetrics_endpoint"].(string)
	}
	assert.Contains(t, endpoints, "http://a1:9090/metrics")
	assert.Contains(t, endpoints, "http://b1:9090/metrics")
	assert.Contains(t, endpoints, "http://b2:9090/metrics")
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

func TestExcludeFilterByLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"host1:9100"},
				Labels:  map[string]string{"__meta_service_type": "ray_worker"},
			},
			{
				Targets: []string{"host2:9100"},
				Labels:  map[string]string{"__meta_service_type": "api"},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{{
		url:           server.URL,
		template:      defaultCheckTemplate(),
		excludeFilter: `target.labels["__meta_service_type"].startsWith("ray")`,
	}})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))
	assert.Equal(t, "http://host2:9100/metrics", instance["openmetrics_endpoint"])
}

func TestExcludeFilterCompoundExpression(t *testing.T) {
	// Exclude targets that match ALL of: specific host, specific port, AND a label value.
	// Only "10.0.0.2:9100" with service_type "worker" should be excluded; the
	// other targets fail at least one condition so they are kept.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"10.0.0.1:9100", "10.0.0.2:9100", "10.0.0.3:9200"},
				Labels:  map[string]string{"__meta_service_type": "worker"},
			},
			{
				Targets: []string{"10.0.0.4:9100"},
				Labels:  map[string]string{"__meta_service_type": "api"},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{{
		url:      server.URL,
		template: defaultCheckTemplate(),
		excludeFilter: `target.host == "10.0.0.2" &&
			target.port == "9100" &&
			target.labels["__meta_service_type"] == "worker"`,
	}})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 3)

	endpoints := make([]string, 0, 3)
	for _, cfg := range configs {
		var inst map[string]interface{}
		require.NoError(t, yaml.Unmarshal(cfg.Instances[0], &inst))
		endpoints = append(endpoints, inst["openmetrics_endpoint"].(string))
	}
	assert.Contains(t, endpoints, "http://10.0.0.1:9100/metrics")    // same port, wrong host
	assert.Contains(t, endpoints, "http://10.0.0.3:9200/metrics")    // right host, wrong port
	assert.Contains(t, endpoints, "http://10.0.0.4:9100/metrics")    // right host+port, wrong label
	assert.NotContains(t, endpoints, "http://10.0.0.2:9100/metrics") // all three match → excluded
}

func TestExcludeFilterNoMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"host1:9090", "host2:9090"},
				Labels:  map[string]string{"__meta_service_type": "api"},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{{
		url:           server.URL,
		template:      defaultCheckTemplate(),
		excludeFilter: `target.labels["__meta_service_type"].startsWith("ray")`,
	}})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 2)
}

// TestExcludeFilterMissingLabelKey documents the fail-open behavior when a CEL
// expression accesses a label key that is absent from some targets.
//
// CEL returns a "no such key" error for map indexing on a missing key, so
// targets without the referenced label are collected rather than excluded.
// Users who want safe filtering should guard with the "in" operator:
//
//	'"__meta_service_type" in target.labels && target.labels["__meta_service_type"].startsWith("ray")'
func TestExcludeFilterMissingLabelKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			// This target has the label — filter applies, target IS excluded.
			{
				Targets: []string{"host1:9100"},
				Labels:  map[string]string{"__meta_service_type": "ray_worker"},
			},
			// This target is missing the label entirely — CEL returns "no such key",
			// the error is logged at debug level, and the target is collected (fail open).
			{
				Targets: []string{"host2:9100"},
				Labels:  map[string]string{"other_label": "value"},
			},
			// This target has no labels at all — same fail-open behavior.
			{
				Targets: []string{"host3:9100"},
			},
		})
	}))
	defer server.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{{
		url:           server.URL,
		template:      defaultCheckTemplate(),
		excludeFilter: `target.labels["__meta_service_type"].startsWith("ray")`,
	}})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	// host1 is excluded; host2 and host3 fail open (missing key → error → collected).
	require.Len(t, configs, 2)

	endpoints := make([]string, 0, 2)
	for _, cfg := range configs {
		var inst map[string]interface{}
		require.NoError(t, yaml.Unmarshal(cfg.Instances[0], &inst))
		endpoints = append(endpoints, inst["openmetrics_endpoint"].(string))
	}
	assert.Contains(t, endpoints, "http://host2:9100/metrics")
	assert.Contains(t, endpoints, "http://host3:9100/metrics")
	assert.NotContains(t, endpoints, "http://host1:9100/metrics")
}

// TestExcludeFilterSafeGuardWithIn documents the idiomatic CEL pattern for safely
// filtering on a label that may not be present on all targets.
func TestExcludeFilterSafeGuardWithIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"host1:9100"}, Labels: map[string]string{"__meta_service_type": "ray_worker"}},
			{Targets: []string{"host2:9100"}, Labels: map[string]string{"other_label": "value"}},
			{Targets: []string{"host3:9100"}},
		})
	}))
	defer server.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{{
		url:      server.URL,
		template: defaultCheckTemplate(),
		// Safe pattern: guard with "in" before indexing to avoid "no such key" errors.
		excludeFilter: `"__meta_service_type" in target.labels && target.labels["__meta_service_type"].startsWith("ray")`,
	}})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	// host1 excluded (has label, matches filter); host2 and host3 pass (label absent → "in" guard returns false).
	require.Len(t, configs, 2)

	endpoints := make([]string, 0, 2)
	for _, cfg := range configs {
		var inst map[string]interface{}
		require.NoError(t, yaml.Unmarshal(cfg.Instances[0], &inst))
		endpoints = append(endpoints, inst["openmetrics_endpoint"].(string))
	}
	assert.Contains(t, endpoints, "http://host2:9100/metrics")
	assert.Contains(t, endpoints, "http://host3:9100/metrics")
	assert.NotContains(t, endpoints, "http://host1:9100/metrics")
}

func TestExcludeFilterEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{Targets: []string{"host1:9090", "host2:9090"}},
		})
	}))
	defer server.Close()

	provider := makeTestProviderWithEntries(t, []entrySpec{{
		url:      server.URL,
		template: defaultCheckTemplate(),
		// no excludeFilter — all targets collected
	}})

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 2)
}

func TestExcludeFilterInvalidExpression(t *testing.T) {
	_, err := compileExcludeFilter(`target.labels["__meta_service_type" + + +`)
	assert.Error(t, err)
}

func TestExcludeFilterNonBoolExpression(t *testing.T) {
	// Non-bool output (string) is now caught at compile time via ast.OutputType().
	_, err := compileExcludeFilter(`target.host`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bool")
}

func TestCompileExcludeFilterEmpty(t *testing.T) {
	prog, err := compileExcludeFilter("")
	require.NoError(t, err)
	assert.Nil(t, prog)
}

func TestIsExcludedNilProgram(t *testing.T) {
	entry := &httpSDEntry{filterProgram: nil}
	excluded, err := entry.isExcluded("host", "9090", map[string]string{"k": "v"})
	require.NoError(t, err)
	assert.False(t, excluded)
}

func TestExcludeFilterUnknownFieldCompilesError(t *testing.T) {
	// With NativeTypes, referencing a non-existent field is caught at compile time.
	_, err := compileExcludeFilter(`target.nonexistent == "value"`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestBuildEntriesInvalidExcludeFilter(t *testing.T) {
	_, errs := buildEntries(
		[]httpSDConfigEntry{{
			URL:           "http://x/sd",
			CheckTemplate: defaultCheckTemplate(),
			ExcludeFilter: `!!!invalid cel`,
		}},
		http.DefaultClient,
	)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "exclude_filter")
}

func TestNewPrometheusHTTPSDConfigProviderNoConfig(t *testing.T) {
	// Nothing set — prometheus_http_sd.configs is absent entirely.
	pkgconfigmock.New(t)

	_, err := NewPrometheusHTTPSDConfigProvider(nil, nil)
	assert.Error(t, err)
}

// TestNewPrometheusHTTPSDConfigProviderFromConfig exercises the full initialization
// path — config parsing, CEL compilation, and collection — using the mock config
// component rather than constructing httpSDEntry directly.
func TestNewPrometheusHTTPSDConfigProviderFromConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]httpSDTargetGroup{
			{
				Targets: []string{"host1:9100"},
				Labels:  map[string]string{"__meta_service_type": "ray_worker"},
			},
			{
				Targets: []string{"host2:9100"},
				Labels:  map[string]string{"__meta_service_type": "api"},
			},
		})
	}))
	defer server.Close()

	cfg := pkgconfigmock.New(t)
	cfg.SetInTest("prometheus_http_sd.configs", []interface{}{
		map[string]interface{}{
			"url":            server.URL,
			"check_template": defaultCheckTemplate(),
			"exclude_filter": `target.labels["__meta_service_type"].startsWith("ray")`,
		},
	})

	p, err := NewPrometheusHTTPSDConfigProvider(nil, nil)
	require.NoError(t, err)
	require.NotNil(t, p)

	provider, ok := p.(types.CollectingConfigProvider)
	require.True(t, ok, "expected PrometheusHTTPSDConfigProvider to implement CollectingConfigProvider")

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(configs[0].Instances[0], &instance))
	assert.Equal(t, "http://host2:9100/metrics", instance["openmetrics_endpoint"])
}
