package api

import (
	"log"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
)

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
	rcv := newTestReceiverFromConfig(&config.AgentConfig{
		Enabled:    true,
		Hostname:   "test.host.name",
		DefaultEnv: "prod",
		ConfigPath: "/path/to/config",
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
		LogLevel:                    "WARN",
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
	})
	defer func(old string) { info.Version = old }(info.Version)
	defer func(old string) { info.GitCommit = old }(info.GitCommit)
	defer func(old string) { info.BuildDate = old }(info.BuildDate)
	defer testutil.WithFeatures("feature_flag")()
	info.Version = "0.99.0"
	info.GitCommit = "fab047e10"
	info.BuildDate = "2020-12-04 15:57:06.74187 +0200 EET m=+0.029001792"
	_, h := rcv.makeInfoHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/info", nil)
	h.ServeHTTP(rec, req)
	if rec.Body.String() != `{
	"version": "0.99.0",
	"git_commit": "fab047e10",
	"build_date": "2020-12-04 15:57:06.74187 +0200 EET m=+0.029001792",
	"endpoints": [
		"/v0.3/traces",
		"/v0.3/services",
		"/v0.4/traces",
		"/v0.4/services",
		"/v0.5/traces",
		"/profiling/v1/input",
		"/v0.6/stats"
	],
	"feature_flags": [
		"feature_flag"
	],
	"client_drop_p0s": true,
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
}` {
		t.Fatal("Output of /info has changed. Changing the keys "+
			"is not allowed because the client rely on them and "+
			"is considered a breaking change:\n\n%f", rec.Body.String())
	}
}
