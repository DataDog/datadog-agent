// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
	"helm.sh/helm/v3/pkg/chart/loader"
)

//go:embed testdata/datadog-3.245.2.tgz
var managedChartFixture []byte

func TestManagedChartRenderedRoutes(t *testing.T) {
	if fmt.Sprintf("%x", sha256.Sum256(managedChartFixture)) != ManagedChartSHA256 {
		t.Fatal("chart fixture checksum mismatch")
	}
	for _, endpoint := range []string{"http://fixture:80", "https://fixture:443", ""} {
		var p receivers.Plan
		var err error
		if endpoint == "" {
			p, err = receivers.Datadog("datadoghq.eu", "runner/api_key")
		} else {
			p, err = receivers.Capture("fixture", endpoint, "")
		}
		if err != nil {
			t.Fatal(err)
		}
		c, err := loader.LoadArchive(bytes.NewReader(managedChartFixture))
		if err != nil {
			t.Fatal(err)
		}
		values := buildValues(chartParams{APIKey: receivers.DummyAPIKey, AgentVersion: "7.83.0", ClusterAgentVersion: "7.83.0"}, "receiver-test", nil, "internal-token")
		extra := map[string]interface{}{
			"datadog":             map[string]interface{}{"tags": []interface{}{"keep:me"}, "env": []interface{}{map[string]interface{}{"name": "DD_E2ECTL_TEST", "value": "preserved"}}},
			"clusterChecksRunner": map[string]interface{}{"enabled": true},
		}
		if err := ValidateRoutingValues(extra); err != nil {
			t.Fatal(err)
		}
		mergeMaps(values, extra)
		if err := ApplyRoutingValues(values, p); err != nil {
			t.Fatal(err)
		}
		if err := validateRenderedRouting(c, values, p); err != nil {
			t.Fatalf("endpoint %q: %v", endpoint, err)
		}
	}
}
func TestHelmRawConflictsBeforeApply(t *testing.T) {
	for _, raw := range []string{
		"datadog: {env: [{name: DD_MULTI_REGION_FAILOVER_ENABLED, value: 'true'}]}",
		"datadog: {envDict: {DD_MULTI_REGION_FAILOVER_API_KEY: dummy-key}}",
		"clusterAgent: {env: [{name: DD_FIPS_ENABLED, value: 'true'}]}",
		"agents: {customAgentConfig: {fips: {enabled: true}}}",
		"agents: {customAgentConfig: {multi_region_failover.enabled: true}}",
		"fips: {enabled: true}",
		"datadog: {env: [{name: DD_EVP_PROXY_CONFIG_ENABLED, value: 'true'}]}",
		"clusterChecksRunner: {envDict: {DD_OL_PROXY_CONFIG_DD_URL: 'http://other'}}",

		"datadog: {apiKey: secret}", "datadog: {apiKeyExistingSecret: name}", "datadog: {env: [{name: DD_PROCESS_AGENT_URL, value: 'https://other'}]}",
		"datadog: {envDict: {DD_LOGS_CONFIG_LOGS_DD_URL: 'https://other'}}", "agents: {customAgentConfig: {container_image: {logs_dd_url: 'https://other'}}}",
		"agents: {containers: {agent: {envFrom: [{secretRef: {name: other}}]}}}", "clusterAgent: {metricsProvider: {enabled: true}}",
		"clusterChecksRunner: {env: [{name: DD_API_KEY, value: secret}]}", "datadog: {env: [{name: DD_TAGS, value: a}, {name: DD_TAGS, value: b}]}",
	} {
		var values map[string]interface{}
		if err := yaml.Unmarshal([]byte(raw), &values); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRoutingValues(values); err == nil {
			t.Fatalf("accepted conflict %s", raw)
		}
	}
}
func TestMergeEnvPreservesNonRouteSettings(t *testing.T) {
	entry := func(name, value string) interface{} { return map[string]interface{}{"name": name, "value": value} }
	merged, err := MergeEnv([]interface{}{entry("DD_REMOTE_CONFIGURATION_ENABLED", "false"), entry("DD_TAGS", "keep:me")}, []interface{}{entry("DD_LOG_LEVEL", "debug")})
	if err != nil || len(merged) != 3 {
		t.Fatal(merged, err)
	}
	if _, err := MergeEnv(nil, []interface{}{entry("DD_TAGS", "a"), entry("DD_TAGS", "b")}); err == nil {
		t.Fatal("duplicate names accepted")
	}
}

func TestImageCapabilitiesAndDeliveryIdentityAreSeparate(t *testing.T) {
	p, _ := receivers.Capture("fakeintake", "http://fixture", "")
	profile := &receivers.ProducerProfile{RouteContract: receivers.RouteContract, Roles: []receivers.ProducerRole{receivers.CoreAgent, receivers.TraceAgent, receivers.ProcessAgent, receivers.ClusterChecksRunner}}
	for _, image := range []ImageArtifact{
		{Repository: "registry.example.test/dev/agent", Tag: "7.99.0-e2ectl-unique-build", LocalImageID: "sha256:" + strings.Repeat("a", 64)},
		{Repository: "registry.example.test/dev/agent", RepositoryDigest: "sha256:" + strings.Repeat("b", 64)},
	} {
		params := Params{AgentVersion: "7.99.0-local", ClusterAgentVersion: "7.83.0", Profile: profile, Image: &image}
		if err := validateProducerProfile(params); err != nil {
			t.Fatal(err)
		}
		values := buildValues(chartParams{APIKey: receivers.DummyAPIKey, AgentVersion: params.AgentVersion, ClusterAgentVersion: params.ClusterAgentVersion}, "source-profile", nil, "join-token")
		applyImageArtifact(values, &image)
		for _, role := range []string{"agents", "clusterChecksRunner"} {
			rendered := values[role].(map[string]interface{})["image"].(map[string]interface{})
			if rendered["digest"] != image.RepositoryDigest || rendered["repository"] != image.Repository {
				t.Fatal(rendered)
			}
			if image.LocalImageID != "" && (rendered["digest"] != "" || rendered["tag"] != image.Tag || rendered["pullPolicy"] != "Never") {
				t.Fatal("Docker image ID treated as repository digest")
			}
		}
		if err := ApplyRoutingValues(values, p); err != nil {
			t.Fatal(err)
		}
		c, err := loader.LoadArchive(bytes.NewReader(managedChartFixture))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateRenderedRouting(c, values, p); err != nil {
			t.Fatal(err)
		}
	}
	bad := ImageArtifact{Repository: "registry.example.test/agent", LocalImageID: "sha256:" + strings.Repeat("a", 64), RepositoryDigest: "sha256:" + strings.Repeat("a", 64)}
	if err := bad.Validate(); err == nil {
		t.Fatal("mixed Docker/OCI identities accepted")
	}
	devImage := ImageArtifact{Repository: "registry.example.test/dev/agent", Tag: "7.99.0-e2ectl-unique-build", LocalImageID: "sha256:" + strings.Repeat("a", 64)}
	if err := validateProducerProfile(Params{AgentVersion: "7.99.0-local", ClusterAgentVersion: "7.83.0", Image: &devImage}); err == nil {
		t.Fatal("dev image without capability profile accepted")
	}
	if err := validateProducerProfile(Params{AgentVersion: "7.99.0-local", ClusterAgentVersion: "7.83.0", Profile: profile}); err == nil {
		t.Fatal("capability profile without its verified image accepted")
	}
	// Released installs are version-agnostic: any AgentVersion passes without
	// capability evidence, and the DCA version is a caller default, not a gate.
	if err := validateProducerProfile(Params{AgentVersion: "7.69.0", ClusterAgentVersion: "7.69.0"}); err != nil {
		t.Fatal(err)
	}
}
