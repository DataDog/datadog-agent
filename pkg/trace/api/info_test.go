// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"log"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// TestInfoHandler ensures that the keys returned by the /info handler do not
// change from one release to another to ensure consistency. Tracing clients
// depend on these keys to be the same. The chances of them changing are quite
// high if anyone ever modifies a field name in the (*AgentConfig).Config structure.
//
// * In case a field name gets modified, the `json:""` struct field tag
// should be used to ensure the old key is marshalled for this endpoint.
func TestInfoHandler(t *testing.T) {
	t.Skip("https://github.com/DataDog/datadog-agent/issues/13569")
	u, err := url.Parse("http://localhost:8888/proxy")
	if err != nil {
		log.Fatal(err)
	}
	jsonObfCfg := config.JSONObfuscationConfig{
		Enabled:            true,
		KeepValues:         []string{"a", "b", "c"},
		ObfuscateSQLValues: []string{"x", "y"},
	}
	obfCfg := &config.ObfuscationConfig{
		ES:                   jsonObfCfg,
		Mongo:                jsonObfCfg,
		SQLExecPlan:          jsonObfCfg,
		SQLExecPlanNormalize: jsonObfCfg,
		HTTP: config.HTTPObfuscationConfig{
			RemoveQueryString: true,
			RemovePathDigits:  true,
		},
		RemoveStackTraces: false,
		Redis:             config.Enablable{Enabled: true},
		Memcached:         config.Enablable{Enabled: false},
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
		LogThrottling:               false,
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
	}

	var testCases = []struct {
		name     string
		expected string
	}{
		{
			name: "default",
			expected: `{
	"version": "0.99.0",
	"git_commit": "fab047e10",
	"endpoints": [
		"/v0.3/traces",
		"/v0.3/services",
		"/v0.4/traces",
		"/v0.4/services",
		"/v0.5/traces",
		"/v0.7/traces",
		"/profiling/v1/input",
		"/telemetry/proxy/",
		"/v0.6/stats",
		"/v0.1/pipeline_stats",
		"/evp_proxy/v1/",
		"/evp_proxy/v2/",
		"/debugger/v1/input"
		"/dogstatsd/v1/proxy"
	],
	"feature_flags": [
		"feature_flag"
	],
	"client_drop_p0s": true,
	"span_meta_structs": true,
	"long_running_spans": true,
	"config": {
		"default_env": "prod",
		"target_tps": 11,
		"max_eps": 12,
		"receiver_port": 8111,
		"receiver_socket": "/sock/path",
		"connection_limit": 12,
		"receiver_timeout": 100,
		"max_request_bytes": 123,
		"statsd_port": 123,
		"max_memory": 1000000,
		"max_cpu": 12345,
		"analyzed_spans_by_service": {
			"X": {
				"Y": 2.4
			}
		},
		"obfuscation": {
			"elastic_search": true,
			"mongo": true,
			"sql_exec_plan": true,
			"sql_exec_plan_normalize": true,
			"http": {
				"remove_query_string": true,
				"remove_path_digits": true
			},
			"remove_stack_traces": false,
			"redis": true,
			"memcached": false
		}
	}
}`,
		},
		{
			name: "debug",
			expected: `{
	"version": "0.99.0",
	"git_commit": "fab047e10",
	"endpoints": [
		"/v0.3/traces",
		"/v0.3/services",
		"/v0.4/traces",
		"/v0.4/services",
		"/v0.5/traces",
		"/v0.7/traces",
		"/profiling/v1/input",
		"/telemetry/proxy/",
		"/v0.6/stats",
		"/v0.1/pipeline_stats",
		"/evp_proxy/v1/",
		"/evp_proxy/v2/",
		"/debugger/v1/input"
		"/dogstatsd/v1/proxy"
	],
	"feature_flags": [
		"feature_flag"
	],
	"client_drop_p0s": true,
	"span_meta_structs": true,
	"long_running_spans": true,
	"config": {
		"default_env": "prod",
		"target_tps": 11,
		"max_eps": 12,
		"receiver_port": 8111,
		"receiver_socket": "/sock/path",
		"connection_limit": 12,
		"receiver_timeout": 100,
		"max_request_bytes": 123,
		"statsd_port": 123,
		"max_memory": 1000000,
		"max_cpu": 12345,
		"analyzed_spans_by_service": {
			"X": {
				"Y": 2.4
			}
		},
		"obfuscation": {
			"elastic_search": true,
			"mongo": true,
			"sql_exec_plan": true,
			"sql_exec_plan_normalize": true,
			"http": {
				"remove_query_string": true,
				"remove_path_digits": true
			},
			"remove_stack_traces": false,
			"redis": true,
			"memcached": false
		}
	}
}`,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			rcv := newTestReceiverFromConfig(conf)
			defer testutil.WithFeatures("feature_flag")()
			_, h := rcv.makeInfoHandler()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/info", nil)
			h.ServeHTTP(rec, req)
			assert.Equal(t, tt.expected, rec.Body.String())
			if rec.Body.String() != tt.expected {
				t.Fatalf("Output of /info has changed. Changing the keys "+
					"is not allowed because the client rely on them and "+
					"is considered a breaking change:\n\n%v", rec.Body.String())
			}
		})
	}
}
