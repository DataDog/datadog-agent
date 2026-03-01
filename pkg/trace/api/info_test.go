// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// ensureKeys takes 2 maps, expect and result, and ensures that the set of keys in expect and
// result match. For each key (k) in expect, if expect[k] is of type map[string]any, then
// ensureKeys recurses on expect[k], result[k], prefix + "." + k.
//
// This should ensure that whatever keys and maps are defined in expect are exactly mirrored in
// result, but without checking for specific values in result.
func ensureKeys(expect, result map[string]any, prefix string) error {
	for k, ev := range expect {
		rv, ok := result[k]
		if !ok {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			return fmt.Errorf("expected key %s, but it is not present in the output", path)
		}

		if em, ok := ev.(map[string]any); ok {
			rm, ok := rv.(map[string]any)
			if !ok {
				return fmt.Errorf("expected key %s to be a map, but it is '%#v'", k, rv)
			}
			if prefix != "" {
				prefix = prefix + "." + k
			} else {
				prefix = k
			}
			if err := ensureKeys(em, rm, prefix); err != nil {
				return err
			}
		}
	}
	for k := range result {
		_, ok := expect[k]
		if !ok {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			return fmt.Errorf("found key %s, but it is not expected in the output. If you've added a new key to the /info endpoint, please add it to the tests", path)
		}
	}
	return nil
}

func TestEnsureKeys(t *testing.T) {
	for _, tt := range []struct {
		expect map[string]any
		result map[string]any
		err    bool
	}{
		{
			expect: map[string]any{
				"one": nil,
				"two": nil,
			},
			result: map[string]any{
				"one": 1,
				"two": "two",
			},
		},
		{
			expect: map[string]any{
				"one":   nil,
				"two":   nil,
				"three": nil,
			},
			result: map[string]any{
				"one": 1,
				"two": "two",
			},
			err: true,
		},
		{
			expect: map[string]any{
				"one": nil,
				"two": nil,
			},
			result: map[string]any{
				"one":   1,
				"two":   "two",
				"three": 3,
			},
			err: true,
		},
		{
			expect: map[string]any{
				"one": nil,
				"two": nil,
				"sub": map[string]any{
					"subone": nil,
					"subtwo": nil,
				},
			},
			result: map[string]any{
				"one": 1,
				"two": "two",
				"sub": map[string]any{
					"subone": 1,
					"subtwo": 2,
				},
			},
		},
		{
			expect: map[string]any{
				"one": nil,
				"two": nil,
				"sub": map[string]any{
					"subone": nil,
					"subtwo": nil,
				},
			},
			result: map[string]any{
				"one": 1,
				"two": map[string]any{ // Map values not described in expect are NOT checked, so this is OK.
					"subone": 1,
					"subtwo": 2,
				},
				"sub": map[string]any{
					"subone": 1,
					"subtwo": 2,
				},
			},
		},
		{
			expect: map[string]any{
				"one": nil,
				"two": nil,
				"sub": map[string]any{
					"subone":   nil,
					"subtwo":   nil,
					"subthree": nil,
				},
			},
			result: map[string]any{
				"one": 1,
				"two": map[string]any{ // Map values not described in expect are NOT checked, so this is OK.
					"subone": 1,
					"subtwo": 2,
				},
				"sub": map[string]any{
					"subone": 1,
					"subtwo": 2,
				},
			},
			err: true,
		},
		{
			expect: map[string]any{
				"one": nil,
				"two": nil,
				"sub": map[string]any{
					"subone": nil,
					"subtwo": nil,
				},
			},
			result: map[string]any{
				"one": 1,
				"two": map[string]any{ // Map values not described in expect are NOT checked, so this is OK.
					"subone": 1,
					"subtwo": 2,
				},
				"sub": map[string]any{
					"subone":   1,
					"subtwo":   2,
					"subthree": 3,
				},
			},
			err: true,
		},
	} {
		t.Run("", func(t *testing.T) {
			err := ensureKeys(tt.expect, tt.result, "")
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestInfoHandler ensures that the keys returned by the /info handler do not
// change from one release to another to ensure consistency. Tracing clients
// depend on these keys to be the same. The chances of them changing are quite
// high if anyone ever modifies a field name in the (*AgentConfig).Config structure.
//
// * In case a field name gets modified, the `json:""` struct field tag
// should be used to ensure the old key is marshalled for this endpoint.
func TestInfoHandler(t *testing.T) {
	u, err := url.Parse("http://localhost:8888/proxy")
	if err != nil {
		log.Fatal(err)
	}
	jsonObfCfg := obfuscate.JSONConfig{
		Enabled:            true,
		KeepValues:         []string{"a", "b", "c"},
		ObfuscateSQLValues: []string{"x", "y"},
	}
	obfCfg := &config.ObfuscationConfig{
		ES:                   jsonObfCfg,
		Mongo:                jsonObfCfg,
		SQLExecPlan:          jsonObfCfg,
		SQLExecPlanNormalize: jsonObfCfg,
		HTTP: obfuscate.HTTPConfig{
			RemoveQueryString: true,
			RemovePathDigits:  true,
		},
		RemoveStackTraces: false,
		Redis:             obfuscate.RedisConfig{Enabled: true},
		Valkey:            obfuscate.ValkeyConfig{Enabled: true},
		Memcached:         obfuscate.MemcachedConfig{Enabled: false},
	}
	conf := &config.AgentConfig{
		ContainerTags: func(cid string) ([]string, error) {
			if cid == "id1" {
				return []string{"kube_cluster_name:clusterA", "kube_namespace:namespace1", "pod_name:pod1"}, nil
			}
			return nil, fmt.Errorf("container tags not found for %s", cid)
		},
		Enabled:      true,
		AgentVersion: "0.99.0",
		GitCommit:    "fab047e10",
		Hostname:     "test.host.name",
		DefaultEnv:   "prod",
		ConfigPath:   "/path/to/config",
		Endpoints: []*config.Endpoint{{
			APIKey:  "123",
			Host:    "https://target-intake.datadoghq.com",
			NoProxy: true,
		}},
		BucketInterval:   time.Second,
		ExtraAggregators: []string{"agg:val"},
		ExtraSampleRate:  2.4,
		TargetTPS:        11,
		MaxEPS:           12,
		ReceiverHost:     "localhost",
		ReceiverPort:     8111,
		ReceiverSocket:   "/sock/path",
		ConnectionLimit:  12,
		ReceiverTimeout:  100,
		MaxRequestBytes:  123,
		StatsWriter: &config.WriterConfig{
			ConnectionLimit:    20,
			QueueSize:          12,
			FlushPeriodSeconds: 14.4,
		},
		TraceWriter: &config.WriterConfig{
			ConnectionLimit:    21,
			QueueSize:          13,
			FlushPeriodSeconds: 15.4,
		},
		StatsdHost:                  "stastd.localhost",
		StatsdPort:                  123,
		LogFilePath:                 "/path/to/logfile",
		MaxMemory:                   1000000,
		MaxCPU:                      12345,
		WatchdogInterval:            time.Minute,
		ProxyURL:                    u,
		SkipSSLValidation:           false,
		Ignore:                      map[string][]string{"resource": {"(GET|POST) /healthcheck", "GET /ping"}},
		RejectTags:                  []*config.Tag{{K: "env", V: "test"}, {K: "debug", V: ""}},
		RequireTags:                 []*config.Tag{{K: "env", V: "prod"}},
		RejectTagsRegex:             []*config.TagRegex{{K: "version", V: regexp.MustCompile(`.*-beta`)}},
		RequireTagsRegex:            []*config.TagRegex{{K: "version", V: regexp.MustCompile(`v1\\..*`)}},
		ReplaceTags:                 []*config.ReplaceRule{{Name: "a", Pattern: "*", Repl: "b"}},
		AnalyzedRateByServiceLegacy: map[string]float64{"X": 1.2},
		AnalyzedSpansByService:      map[string]map[string]float64{"X": {"Y": 2.4}},
		DDAgentBin:                  "/path/to/core/agent",
		Obfuscation:                 obfCfg,
		TelemetryConfig: &config.TelemetryConfig{
			Enabled: true,
			Endpoints: []*config.Endpoint{
				{
					APIKey:  "123",
					Host:    "https://telemetry-intake.datadoghq.com",
					NoProxy: true,
				},
			},
		},
		Features: map[string]struct{}{"feature_flag": {}},
	}

	expectedKeys := map[string]any{
		"version":                   nil,
		"git_commit":                nil,
		"endpoints":                 nil,
		"feature_flags":             nil,
		"client_drop_p0s":           nil,
		"span_meta_structs":         nil,
		"long_running_spans":        nil,
		"span_events":               nil,
		"evp_proxy_allowed_headers": nil,
		"peer_tags":                 nil,
		"span_kinds_stats_computed": nil,
		"obfuscation_version":       nil,
		"opm":                       nil,
		"filter_tags": map[string]any{
			"require": nil,
			"reject":  nil,
		},
		"filter_tags_regex": map[string]any{
			"require": nil,
			"reject":  nil,
		},
		"ignore_resources": nil,
		"config": map[string]any{
			"default_env":               nil,
			"target_tps":                nil,
			"max_eps":                   nil,
			"receiver_port":             nil,
			"receiver_socket":           nil,
			"connection_limit":          nil,
			"receiver_timeout":          nil,
			"max_request_bytes":         nil,
			"statsd_port":               nil,
			"max_memory":                nil,
			"max_cpu":                   nil,
			"analyzed_spans_by_service": nil,
			"obfuscation": map[string]any{
				"elastic_search":          nil,
				"mongo":                   nil,
				"sql_exec_plan":           nil,
				"sql_exec_plan_normalize": nil,
				"http": map[string]any{
					"remove_query_string": nil,
					"remove_path_digits":  nil,
				},
				"remove_stack_traces": nil,
				"redis":               nil,
				"valkey":              nil,
				"memcached":           nil,
			},
		},
	}

	rcv := newTestReceiverFromConfig(conf)
	// Simulate a successfully fetched org UUID so that "opm" appears in the response.
	testUUID := "test-org-uuid-1234"
	testHash := sha256.Sum256([]byte(testUUID))
	expectedOPM := base64.RawURLEncoding.EncodeToString(testHash[:8])[:10]
	rcv.orgUUIDOPM.Store(expectedOPM)
	_, h := rcv.makeInfoHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/info", nil)
	req.Header.Add("Datadog-Container-ID", "id1")
	h.ServeHTTP(rec, req)
	var m map[string]any
	if !assert.NoError(t, json.NewDecoder(rec.Body).Decode(&m)) {
		return
	}
	assert.NoError(t, ensureKeys(expectedKeys, m, ""))
	assert.Equal(t, expectedOPM, m["opm"])
	expectedContainerHash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{"kube_cluster_name:clusterA", "kube_namespace:namespace1"}, ","))))
	assert.Equal(t, expectedContainerHash, rec.Header().Get(containerTagsHashHeader))
}

func TestInfoHandlerFilterTags(t *testing.T) {
	conf := config.New()
	conf.Endpoints = []*config.Endpoint{{Host: "http://localhost:8126", APIKey: "test"}}
	conf.RequireTags = []*config.Tag{
		{K: "env", V: "prod"},
		{K: "team", V: "backend"},
	}
	conf.RejectTags = []*config.Tag{
		{K: "env", V: "test"},
		{K: "debug", V: ""},
		{K: "internal", V: "true"},
	}
	conf.RequireTagsRegex = []*config.TagRegex{
		{K: "service", V: regexp.MustCompile("^api-.*")},
	}
	conf.RejectTagsRegex = []*config.TagRegex{
		{K: "version", V: regexp.MustCompile(".*-beta")},
		{K: "experimental_.*", V: nil},
	}
	conf.Ignore = map[string][]string{
		"resource": {"(GET|POST) /healthcheck", "GET /ping"},
	}

	rcv := newTestReceiverFromConfig(conf)
	_, h := rcv.makeInfoHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/info", nil)
	h.ServeHTTP(rec, req)

	var result map[string]any
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&result))

	// Check filter_tags
	filterTags, ok := result["filter_tags"].(map[string]any)
	assert.True(t, ok, "filter_tags should be present and should be a map")

	requireTags, ok := filterTags["require"].([]any)
	assert.True(t, ok, "filter_tags.require should be an array")
	assert.Len(t, requireTags, 2)
	assert.Contains(t, requireTags, "env:prod")
	assert.Contains(t, requireTags, "team:backend")

	rejectTags, ok := filterTags["reject"].([]any)
	assert.True(t, ok, "filter_tags.reject should be an array")
	assert.Len(t, rejectTags, 3)
	assert.Contains(t, rejectTags, "env:test")
	assert.Contains(t, rejectTags, "debug")
	assert.Contains(t, rejectTags, "internal:true")

	// Check filter_tags_regex
	filterTagsRegex, ok := result["filter_tags_regex"].(map[string]any)
	assert.True(t, ok, "filter_tags_regex should be present and should be a map")

	requireTagsRegex, ok := filterTagsRegex["require"].([]any)
	assert.True(t, ok, "filter_tags_regex.require should be an array")
	assert.Len(t, requireTagsRegex, 1)
	assert.Contains(t, requireTagsRegex, "service:^api-.*")

	rejectTagsRegex, ok := filterTagsRegex["reject"].([]any)
	assert.True(t, ok, "filter_tags_regex.reject should be an array")
	assert.Len(t, rejectTagsRegex, 2)
	assert.Contains(t, rejectTagsRegex, "version:.*-beta")
	assert.Contains(t, rejectTagsRegex, "experimental_.*")

	// Check ignore_resources
	ignoreResources, ok := result["ignore_resources"].([]any)
	assert.True(t, ok, "ignore_resources should be an array")
	assert.Len(t, ignoreResources, 2)
	assert.Contains(t, ignoreResources, "(GET|POST) /healthcheck")
	assert.Contains(t, ignoreResources, "GET /ping")
}
