// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfig

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
	"strings"
	"testing"
)

func TestGenerateWithRoutingTLSAndSignalFamilies(t *testing.T) {
	for _, endpoint := range []string{"http://sink:8080", "https://sink:443"} {
		p, err := receivers.Capture("blackhole", endpoint, "")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := GenerateWithRouting(p, "must-not-reach-capture", "tags: [custom:tag]\nlogs_enabled: true\nhostname: sender\n")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, "must-not-reach-capture") {
			t.Fatal("real credential leaked to capture")
		}
		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"dd_url", "apm_config.apm_dd_url", "apm_config.telemetry.dd_url", "process_config.process_dd_url", "orchestrator_explorer.orchestrator_dd_url", "container_image.logs_dd_url", "container_lifecycle.logs_dd_url", "sbom.logs_dd_url", "agent_telemetry.logs_dd_url"} {
			if m[key] != endpoint {
				t.Errorf("%s = %#v", key, m[key])
			}
		}
		for _, prefix := range []string{"logs_config", "container_image", "container_lifecycle", "sbom", "agent_telemetry"} {
			if m[prefix+".logs_no_ssl"] != strings.HasPrefix(endpoint, "http:") {
				t.Errorf("TLS incorrect for %s", prefix)
			}
		}
		if m["remote_configuration.enabled"] != false || m["api_key"] != receivers.DummyAPIKey || m["logs_enabled"] != true || m["hostname"] != "sender" {
			t.Fatalf("wrong configuration: %#v", m)
		}
		if _, ok := m["health_platform.dd_url"]; ok {
			t.Fatal("invented health setting")
		}
		if m["tags"].([]interface{})[0] != "custom:tag" {
			t.Fatal("tags lost")
		}
	}
}
func TestGenerateWithRoutingRejectsConflicts(t *testing.T) {
	p, _ := receivers.Capture("fakeintake", "http://fixture", "")
	for _, raw := range []string{"api_key: secret", "logs_config.logs_dd_url: https://other", "remote_configuration.enabled: true", "sbom.additional_endpoints: []"} {
		if _, err := GenerateWithRouting(p, "", raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	p.RemoteConfig = "receiver"
	if _, err := GenerateWithRouting(p, "", ""); err == nil {
		t.Fatal("accepted unsupported receiver RC")
	}
}
func TestGenerateNativeDoesNotKeepCaptureTLS(t *testing.T) {
	p, _ := receivers.Datadog("datadoghq.eu", "runner/api_key")
	raw, err := GenerateWithRouting(p, "test-native-key", "tags: [keep:me]")
	if err != nil {
		t.Fatal(err)
	}
	for _, capture := range []string{"dd_url", "logs_no_ssl", receivers.DummyAPIKey, "config_root"} {
		if strings.Contains(raw, capture) {
			t.Fatalf("native config retains %s", capture)
		}
	}
}
