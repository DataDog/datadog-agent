// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package receivers

import (
	"strings"
	"testing"
)

func TestEndpointNetworkMatrix(t *testing.T) {
	for _, tc := range []struct {
		url             string
		separate, valid bool
	}{
		{"http://dev-fakeintake:80", true, true}, {"http://192.0.2.10:8080", true, true}, {"https://cloud.example.test", false, true},
		{"http://127.0.0.1:8080", true, false}, {"http://[::1]:8080", true, false}, {"http://localhost", true, false}, {"http://0.0.0.0:8080", true, false},
		{"http://127.0.0.1:8080", false, true}, {"https://[2001:db8::1]:443/", true, true},
		{"https://key:secret@example.test", false, false}, {"https://example.test/path", false, false}, {"https://example.test?api_key=secret", false, false},
		{"ftp://example.test", false, false}, {"https://example.test:99999", false, false}, {"https://example.test:", false, false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			_, err := AgentURL(tc.url, tc.separate)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("endpoint error leaked input")
			}
		})
	}
}

func TestConfigOwnedKeysAndAliases(t *testing.T) {
	for _, raw := range []string{
		"dd_url: https://other.test", "api_key: secret", "API_KEY: secret", "additional_endpoints: {https://other: [dummy]}", "apm_config: {additional_endpoints: {https://other: [dummy]}}", "logs_config:\n  logs_dd_url: http://other.test", "process_config.orchestrator_dd_url: http://other.test",
		"container_image.additional_endpoints: []", "remote_configuration.enabled: true", "vector.logs.enabled: true", "observability_pipelines_worker.metrics.url: http://other.test",
		"external_metrics_provider.enabled: true", "network_devices.snmp_traps.enabled: true",
		"logs_config: {logs_no_ssl: true}\nlogs_config.logs_no_ssl: false", "log_level: info\nlog_level: debug", "x: &x {y: *x}", "x: 1\n---\ny: 2",
	} {
		if _, err := ValidateConfig(raw); err == nil {
			t.Errorf("accepted conflicting config %q", raw)
		} else if strings.Contains(err.Error(), "secret") {
			t.Fatal("error leaked credential")
		}
	}
	for _, name := range []string{"DD_URL", "DD_DD_URL", "DD_PROCESS_AGENT_URL", "DD_PROCESS_CONFIG_URL", "DD_PROCESS_AGENT_PROCESS_DD_URL", "DD_PROCESS_CONFIG_PROCESS_DD_URL", "DD_PROCESS_AGENT_ORCHESTRATOR_DD_URL", "DD_PROCESS_AGENT_ADDITIONAL_ENDPOINTS", "DD_PROCESS_ADDITIONAL_ENDPOINTS", "DD_APM_ADDITIONAL_ENDPOINTS", "DD_APM_TELEMETRY_ADDITIONAL_ENDPOINTS", "DD_ORCHESTRATOR_URL", "DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"} {
		if !Owns(EnvKey(name)) {
			t.Errorf("missed source alias %s -> %s", name, EnvKey(name))
		}
	}
	m, err := ValidateConfig("tags: [custom:tag]\nlogs_enabled: true\nkubelet_tls_verify: false\nconfig_providers: [{name: etcd, template_url: 'http://etcd:2379'}]")
	if err != nil || len(m) != 4 {
		t.Fatalf("non-route config not preserved: %#v %v", m, err)
	}
}

func TestNativePlanHasNoCaptureOverrides(t *testing.T) {
	p, err := Datadog("datadoghq.eu", "runner/api_key")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Settings()) != 2 || p.Settings()["site"] != "datadoghq.eu" {
		t.Fatalf("native plan leaked capture settings: %#v", p.Settings())
	}
	if _, err := Datadog("datadoghq.eu", "inline-secret"); err == nil {
		t.Fatal("accepted unbounded reference")
	}
}

func TestSourceNamedAPMEnvironmentAliases(t *testing.T) {
	for key, want := range map[string]string{
		"apm_config.apm_dd_url":                  "DD_APM_DD_URL",
		"apm_config.telemetry.dd_url":            "DD_APM_TELEMETRY_DD_URL",
		"apm_config.profiling_dd_url":            "DD_APM_PROFILING_DD_URL",
		"apm_config.debugger_dd_url":             "DD_APM_DEBUGGER_DD_URL",
		"apm_config.debugger_diagnostics_dd_url": "DD_APM_DEBUGGER_DIAGNOSTICS_DD_URL",
		"apm_config.symdb_dd_url":                "DD_APM_SYMDB_DD_URL",
	} {
		if EnvName(key) != want || EnvKey(want) != key {
			t.Fatalf("source env_vars for %s is %s, got %s/%s", key, want, EnvName(key), EnvKey(want))
		}
	}
}

func TestUnsupportedEndpointFamilies(t *testing.T) {
	for _, raw := range []string{
		"multi_region_failover: {enabled: true, failover_metrics: true, site: datadoghq.eu, api_key: dummy-secondary-key}",
		"multi_region_failover.enabled: true",
		"multi_region_failover: {api_key: dummy-secondary-key}",
		"fips: {enabled: true}", "fips.enabled: true",
		"evp_proxy_config: {enabled: true}", "evp_proxy_config.dd_url: example.test",
		"ol_proxy_config.enabled: true", "ol_proxy_config: {api_key: dummy-key}",
	} {
		if _, err := ValidateConfig(raw); err == nil {
			t.Fatalf("accepted unsupported endpoint family: %s", raw)
		}
	}
	for _, name := range []string{"DD_MULTI_REGION_FAILOVER_ENABLED", "DD_MULTI_REGION_FAILOVER_API_KEY", "DD_FIPS_ENABLED", "DD_FIPS_LOCAL_ADDRESS", "DD_EVP_PROXY_CONFIG_ENABLED", "DD_OL_PROXY_CONFIG_DD_URL"} {
		key := EnvKey(name)
		if !Owns(key) && !Unsupported(key, true) {
			t.Fatalf("accepted env alias %s -> %s", name, key)
		}
	}
}

func TestCapturePlanWarnsAboutDiagnosticExclusion(t *testing.T) {
	p, err := Capture("fakeintake", "http://fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	warnings := strings.Join(p.Warnings, " ")
	if !strings.Contains(warnings, "tracer flare uses the native site") || !strings.Contains(warnings, "not a no-native-egress/isolation mode") {
		t.Fatal("missing diagnostic exclusion in plan output")
	}
}
