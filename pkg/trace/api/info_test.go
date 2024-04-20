// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// ensureKeys takes 2 maps, expect and result, and ensures that the set of keys in expect and
// result match. For each key (k) in expect, if expect[k] is of type map[string]interface{}, then
// ensureKeys recurses on expect[k], result[k], prefix + "." + k.
//
// This should ensure that whatever keys and maps are defined in expect are exactly mirrored in
// result, but without checking for specific values in result.
func ensureKeys(expect, result map[string]interface{}, prefix string) error {
	for k, ev := range expect {
		rv, ok := result[k]
		if !ok {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			return fmt.Errorf("expected key %s, but it is not present in the output", path)
		}

		if em, ok := ev.(map[string]interface{}); ok {
			rm, ok := rv.(map[string]interface{})
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
		expect map[string]interface{}
		result map[string]interface{}
		err    bool
	}{
		{
			expect: map[string]interface{}{
				"one": nil,
				"two": nil,
			},
			result: map[string]interface{}{
				"one": 1,
				"two": "two",
			},
		},
		{
			expect: map[string]interface{}{
				"one":   nil,
				"two":   nil,
				"three": nil,
			},
			result: map[string]interface{}{
				"one": 1,
				"two": "two",
			},
			err: true,
		},
		{
			expect: map[string]interface{}{
				"one": nil,
				"two": nil,
			},
			result: map[string]interface{}{
				"one":   1,
				"two":   "two",
				"three": 3,
			},
			err: true,
		},
		{
			expect: map[string]interface{}{
				"one": nil,
				"two": nil,
				"sub": map[string]interface{}{
					"subone": nil,
					"subtwo": nil,
				},
			},
			result: map[string]interface{}{
				"one": 1,
				"two": "two",
				"sub": map[string]interface{}{
					"subone": 1,
					"subtwo": 2,
				},
			},
		},
		{
			expect: map[string]interface{}{
				"one": nil,
				"two": nil,
				"sub": map[string]interface{}{
					"subone": nil,
					"subtwo": nil,
				},
			},
			result: map[string]interface{}{
				"one": 1,
				"two": map[string]interface{}{ // Map values not described in expect are NOT checked, so this is OK.
					"subone": 1,
					"subtwo": 2,
				},
				"sub": map[string]interface{}{
					"subone": 1,
					"subtwo": 2,
				},
			},
		},
		{
			expect: map[string]interface{}{
				"one": nil,
				"two": nil,
				"sub": map[string]interface{}{
					"subone":   nil,
					"subtwo":   nil,
					"subthree": nil,
				},
			},
			result: map[string]interface{}{
				"one": 1,
				"two": map[string]interface{}{ // Map values not described in expect are NOT checked, so this is OK.
					"subone": 1,
					"subtwo": 2,
				},
				"sub": map[string]interface{}{
					"subone": 1,
					"subtwo": 2,
				},
			},
			err: true,
		},
		{
			expect: map[string]interface{}{
				"one": nil,
				"two": nil,
				"sub": map[string]interface{}{
					"subone": nil,
					"subtwo": nil,
				},
			},
			result: map[string]interface{}{
				"one": 1,
				"two": map[string]interface{}{ // Map values not described in expect are NOT checked, so this is OK.
					"subone": 1,
					"subtwo": 2,
				},
				"sub": map[string]interface{}{
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
		Memcached:         obfuscate.MemcachedConfig{Enabled: false},
	}
	conf := &config.AgentConfig{
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
		Ignore:                      map[string][]string{"K": {"1", "2"}},
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

	expectedKeys := map[string]interface{}{
		"version":                   nil,
		"git_commit":                nil,
		"endpoints":                 nil,
		"feature_flags":             nil,
		"client_drop_p0s":           nil,
		"span_meta_structs":         nil,
		"long_running_spans":        nil,
		"evp_proxy_allowed_headers": nil,
		"config": map[string]interface{}{
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
			"obfuscation": map[string]interface{}{
				"elastic_search":          nil,
				"mongo":                   nil,
				"sql_exec_plan":           nil,
				"sql_exec_plan_normalize": nil,
				"http": map[string]interface{}{
					"remove_query_string": nil,
					"remove_path_digits":  nil,
				},
				"remove_stack_traces": nil,
				"redis":               nil,
				"memcached":           nil,
			},
		},
	}

	rcv := newTestReceiverFromConfig(conf)
	_, h := rcv.makeInfoHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/info", nil)
	h.ServeHTTP(rec, req)
	var m map[string]interface{}
	if !assert.NoError(t, json.NewDecoder(rec.Body).Decode(&m)) {
		return
	}
	assert.NoError(t, ensureKeys(expectedKeys, m, ""))
}
