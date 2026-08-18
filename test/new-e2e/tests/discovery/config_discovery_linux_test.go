// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"path"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// fakeIntegrationsServerScript is the fake krakend+haproxy metrics HTTP
// server, shared between the compose fixtures below via text/template rather
// than copy-pasted, since it's byte-for-byte identical in all of them.
//
//go:embed testdata/compose/fake-integrations-server.py
var fakeIntegrationsServerScript string

//go:embed testdata/compose/docker-compose.fake-krakend.yaml.tmpl
var fakeKrakendComposeTmpl string

//go:embed testdata/compose/docker-compose.fake-krakend-conflict.yaml.tmpl
var fakeKrakendConflictComposeTmpl string

//go:embed testdata/compose/docker-compose.fake-haproxy.yaml.tmpl
var fakeHaproxyComposeTmpl string

var fakeKrakendComposeStr = mustRenderFakeComposeTemplate(fakeKrakendComposeTmpl)

var fakeKrakendConflictComposeStr = mustRenderFakeComposeTemplate(fakeKrakendConflictComposeTmpl)

var fakeHaproxyComposeStr = mustRenderFakeComposeTemplate(fakeHaproxyComposeTmpl)

// mustRenderFakeComposeTemplate splices fakeIntegrationsServerScript into a
// compose template's `{{.Script}}` placeholder, indented to match the
// surrounding YAML block scalar. YAML literal block scalars (`|`) only
// require non-blank lines to be indented at least as much as the block
// itself, so blank lines are left alone.
func mustRenderFakeComposeTemplate(tmplSrc string) string {
	tmpl := template.Must(template.New("fake-compose").Parse(tmplSrc))
	var buf bytes.Buffer
	data := struct{ Script string }{Script: indentLines(fakeIntegrationsServerScript, 8)}
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}

func indentLines(s string, n int) string {
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

type configDiscoverySuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// haproxyStaticConfigDir is where haproxyStaticOpenmetricsConfig is written
// on the remote host (see createHaproxyStaticOpenmetricsConfig), then
// bind-mounted into the agent container's own conf.d.
const haproxyStaticConfigDir = "/tmp/discovery-haproxy-static-openmetrics"

// haproxyStaticOpenmetricsConfig is a manually-authored, host-level
// (non-template, no ad_identifiers) openmetrics check config, exactly as a
// user would place directly in conf.d/openmetrics.d/conf.yaml. Its namespace
// ("haproxy") is fake-haproxy's own metric namespace
// (__NAMESPACE__ = "haproxy" in datadog_checks/haproxy/check.py), so once
// haproxy's configuration-discovery template matches the fake-haproxy
// container, discovery must be suppressed via the host-wide StaticConfigIndex
// (see comp/core/autodiscovery/impl/configmgr.go's processNewConfig and
// comp/core/autodiscovery/listeners/common_filter.go's
// filterTemplatesDiscovery) -- unlike TestKrakendConfigDiscoverySuppressedByConflictingGenericIntegration's
// fake-krakend-conflict, this config isn't attached to fake-haproxy's
// container at all.
const haproxyStaticOpenmetricsConfig = `instances:
  - openmetrics_endpoint: http://fake-haproxy:8404/metrics
    namespace: haproxy
`

// createHaproxyStaticOpenmetricsConfig writes haproxyStaticOpenmetricsConfig
// to the remote host before the agent's compose stack comes up, so it can be
// bind-mounted into the agent container's conf.d via
// dockeragentparams.WithExtraVolumes (see TestConfigDiscoverySuite).
func createHaproxyStaticOpenmetricsConfig(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	fileManager := host.OS.FileManager()
	createDir, err := fileManager.CreateDirectory(haproxyStaticConfigDir, false)
	if err != nil {
		return nil, err
	}
	return fileManager.CopyInlineFile(
		pulumi.String(haproxyStaticOpenmetricsConfig),
		path.Join(haproxyStaticConfigDir, "conf.yaml"),
		utils.PulumiDependsOn(createDir),
	)
}

func TestConfigDiscoverySuite(t *testing.T) {
	t.Parallel()

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(
			awsdocker.WithRunOptions(
				scendocker.WithPreAgentInstallHook(createHaproxyStaticOpenmetricsConfig),
				scendocker.WithAgentOptions(
					dockeragentparams.WithExtraComposeManifest("fake-krakend", pulumi.String(fakeKrakendComposeStr)),
					dockeragentparams.WithExtraComposeManifest("fake-krakend-conflict", pulumi.String(fakeKrakendConflictComposeStr)),
					dockeragentparams.WithExtraComposeManifest("fake-haproxy", pulumi.String(fakeHaproxyComposeStr)),
					dockeragentparams.WithExtraVolumes(
						haproxyStaticConfigDir+"/conf.yaml:/etc/datadog-agent/conf.d/openmetrics.d/conf.yaml:ro",
					),
				),
			),
		)),
	}
	e2e.Run(t, &configDiscoverySuite{}, options...)
}

// TestKrakendConfigDiscovery verifies that integration config discovery works
// end-to-end: the agent discovers the fake-krakend container via the Docker
// listener (matching ad_identifiers: [krakend] from the shipped auto_conf.yaml),
// calls krakend's discover_config callback with the container's host and
// exposed ports, discover_config probes candidate ports in order (starting
// with krakend's default metrics port 9090) and returns an OpenMetrics check
// config for the first one that yields real krakend metrics. The fake
// container serves a non-krakend decoy on 9090 and the real krakend metrics
// on 9091, so a successful test proves discover_config actually probes and
// validates candidates rather than blindly using the first exposed port.
//
// This also acts as the non-conflict control for the two suppression tests
// below: the suite runs a host-level static openmetrics config claiming
// namespace "haproxy" throughout (see haproxyStaticOpenmetricsConfig), and
// krakend's own discovery here must remain unaffected by it, since the two
// namespaces don't share a root.
func (s *configDiscoverySuite) TestKrakendConfigDiscovery() {
	t := s.T()
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyKrakendConfigDiscovery(c)
	}, 3*time.Minute, 10*time.Second, "krakend check should be scheduled and running via config discovery")
}

func (s *configDiscoverySuite) verifyKrakendConfigDiscovery(c *assert.CollectT) {
	t := s.T()

	configCheckOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "configcheck")
	if !assert.True(c, strings.Contains(configCheckOutput, "=== krakend check ==="), "krakend check should appear in configcheck") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, "openmetrics_endpoint"), "krakend config should have openmetrics_endpoint") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, ":9091/metrics"), "openmetrics_endpoint should point to port 9091 (the real krakend metrics endpoint, not the 9090 decoy)") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, configDiscoveryTag), "krakend config resolved via configuration discovery should carry the %s marker tag", configDiscoveryTag) {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}

	statusOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "status", "collector", "--json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if !assert.NoError(c, err, "failed to parse collector status JSON") {
		t.Logf("status output: %s", statusOutput)
		return
	}
	instances, exists := status.RunnerStats.Checks["krakend"]
	if !assert.True(c, exists, "krakend check should be running; available: %v", getCheckNames(status.RunnerStats.Checks)) {
		return
	}
	ran := false
	for name, stat := range instances {
		if len(stat.ExecutionTimes) > 0 {
			t.Logf("krakend instance %s: runs=%d", name, len(stat.ExecutionTimes))
			ran = true
			break
		}
	}
	if !assert.True(c, ran, "krakend check is configured but has not executed yet") {
		return
	}

	// Verify config.provider in inventory-checks metadata reflects that the
	// config came from the configuration-discovery path (via the Docker
	// listener), not a plain file provider.
	s.verifyKrakendCheckProvider(c)

	// Verify the metric actually submitted by the discovered krakend check
	// carries the configuration-discovery marker tag, not just the resolved
	// config (checked above via configcheck).
	s.verifyKrakendMetricHasConfigDiscoveryTag(c)
}

// verifyKrakendMetricHasConfigDiscoveryTag checks, via fakeintake, that a
// metric submitted by the discovered krakend check (krakend.api.go.goroutines,
// from the fake container's go_goroutines gauge) carries the
// configDiscoveryTag marker tag end to end, not just in the resolved config.
func (s *configDiscoverySuite) verifyKrakendMetricHasConfigDiscoveryTag(c *assert.CollectT) {
	const metricName = "krakend.api.go.goroutines"

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName,
		fakeintakeclient.WithTags[*aggregator.MetricSeries]([]string{configDiscoveryTag}))
	if !assert.NoError(c, err, "failed to query fakeintake for %s", metricName) {
		return
	}
	assert.NotEmpty(c, metrics, "expected at least one %s series tagged with %s", metricName, configDiscoveryTag)
}

// adContainerDiscoveryProvider mirrors names.ADContainerDiscovery in
// comp/core/autodiscovery/providers/names (not importable here: it lives in
// the root module, which test/new-e2e does not depend on). It is the source
// prefix that configmgr_discovery.go's rewriteSource applies to file-based
// configs resolved via configuration discovery against non-process services
// (e.g. containers, discovered via the Docker listener here).
const adContainerDiscoveryProvider = "ad-container-discovery+file"

// configDiscoveryTag mirrors configDiscoveryTag in
// comp/core/autodiscovery/impl/configmgr_discovery.go (not importable here:
// it lives in the root module, which test/new-e2e does not depend on). It is
// the marker tag configuration discovery adds to every instance it
// schedules, so users can identify (and, if needed, exclude) metrics
// submitted by an autodiscovered check that duplicates a manually-configured
// one pointed at the same service from elsewhere.
const configDiscoveryTag = "dd_config_discovery:true"

// verifyKrakendCheckProvider checks that the krakend check has
// config.provider = adContainerDiscoveryProvider in the inventory-checks
// metadata, confirming it was resolved via configuration discovery against
// a container (as opposed to a process).
func (s *configDiscoverySuite) verifyKrakendCheckProvider(c *assert.CollectT) {
	t := s.T()

	metadataOut := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "diagnose", "show-metadata", "inventory-checks")

	var payload struct {
		CheckMetadata map[string][]map[string]interface{} `json:"check_metadata"`
	}
	if !assert.NoError(c, json.Unmarshal([]byte(metadataOut), &payload), "failed to parse inventory-checks metadata") {
		t.Logf("inventory-checks output: %s", metadataOut)
		return
	}

	instances, exists := payload.CheckMetadata["krakend"]
	if !assert.True(c, exists, "krakend should appear in inventory-checks metadata") {
		keys := make([]string, 0, len(payload.CheckMetadata))
		for k := range payload.CheckMetadata {
			keys = append(keys, k)
		}
		t.Logf("available checks in inventory metadata: %v", keys)
		return
	}
	if !assert.NotEmpty(c, instances, "krakend metadata should have at least one instance") {
		return
	}

	// Always logged (not just on failure) so the raw metadata is available for
	// manual debugging, e.g. when config.provider matches but other fields look off.
	t.Logf("krakend inventory-checks metadata: %+v", instances)

	assert.Equal(c, adContainerDiscoveryProvider, instances[0]["config.provider"],
		"krakend resolved via configuration discovery should have config.provider = %s", adContainerDiscoveryProvider)
}

// TestKrakendConfigDiscoverySuppressedByConflictingGenericIntegration verifies
// that configuration discovery must not schedule a second, duplicate check on
// a container that already has any manually-configured generic
// openmetrics/prometheus check matched to it, regardless of that check's own
// namespace. Without this, both krakend (via discovery) and the manual
// openmetrics config would run concurrently against the fake-krakend-conflict
// container, double-collecting (and, for counter metrics, doubling the
// reported value of) the same metrics.
//
// fake-krakend-conflict is a second, independent container from
// fake-krakend (used by TestKrakendConfigDiscovery): it's also image-matched
// to krakend's `ad_identifiers: [krakend]`, but additionally carries a
// com.datadoghq.ad.checks label configuring a manual "openmetrics" check
// against the exact same :9091/metrics endpoint krakend's own discovery would
// resolve to. Its namespace ("krakend.api") happens to be rooted in krakend's
// own __NAMESPACE__, but that's incidental: suppression here is name/namespace
// -agnostic -- once a generic scraper is manually configured against a
// service at all, we assume the user already knows how to collect its
// metrics rather than try to compare namespaces (see
// comp/core/autodiscovery/listeners/common_filter.go's
// filterTemplatesDiscovery).
//
// This covers the "sibling" suppression path: the manual config is matched to
// the exact same container/service as the discovery template (both resolved
// via the Docker listener). See
// TestHaproxyConfigDiscoverySuppressedByHostLevelGenericIntegration below for
// the other, independent suppression path: a manual config that isn't
// attached to the discovered container at all, tracked host-wide via
// StaticConfigIndex.
func (s *configDiscoverySuite) TestKrakendConfigDiscoverySuppressedByConflictingGenericIntegration() {
	t := s.T()
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyKrakendConfigDiscoverySuppressedByConflictingGenericIntegration(c)
	}, 3*time.Minute, 10*time.Second, "manual openmetrics check should be running on the conflicting container, and krakend discovery should be suppressed there")
}

func (s *configDiscoverySuite) verifyKrakendConfigDiscoverySuppressedByConflictingGenericIntegration(c *assert.CollectT) {
	t := s.T()

	configCheckOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "configcheck")
	if !assert.True(c, strings.Contains(configCheckOutput, "=== openmetrics check ==="), "manual openmetrics check should appear in configcheck") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, "namespace: krakend.api"), "manual openmetrics config should carry the conflicting namespace") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}

	statusOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "status", "collector", "--json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if !assert.NoError(c, err, "failed to parse collector status JSON") {
		t.Logf("status output: %s", statusOutput)
		return
	}

	// Exactly one krakend instance should ever be running on this host: the
	// one discovered for the non-conflicting fake-krakend container (verified
	// by TestKrakendConfigDiscovery). Discovery must not have ALSO scheduled a
	// second krakend instance for fake-krakend-conflict, since a manual
	// openmetrics config matched to that same container already covers it.
	krakendInstances := status.RunnerStats.Checks["krakend"]
	if !assert.Len(c, krakendInstances, 1,
		"exactly one krakend instance should be running (discovery must be suppressed on the conflicting container); found: %+v", krakendInstances) {
		return
	}

	openmetricsInstances, exists := status.RunnerStats.Checks["openmetrics"]
	if !assert.True(c, exists, "manual openmetrics check should be running; available: %v", getCheckNames(status.RunnerStats.Checks)) {
		return
	}
	ran := false
	for name, stat := range openmetricsInstances {
		if len(stat.ExecutionTimes) > 0 {
			t.Logf("openmetrics instance %s: runs=%d", name, len(stat.ExecutionTimes))
			ran = true
			break
		}
	}
	assert.True(c, ran, "manual openmetrics check is configured but has not executed yet")
}

// TestHaproxyConfigDiscoverySuppressedByHostLevelGenericIntegration verifies
// the other, independent suppression path: a manually-authored, host-level
// (non-template, no ad_identifiers at all) openmetrics config claiming a
// namespace must suppress a *different* container's configuration-discovery
// template, purely because their namespace roots match — tracked host-wide
// via StaticConfigIndex (comp/core/autodiscovery/impl/configmgr.go's
// processNewConfig), not because the two configs share a container or
// service.
//
// haproxyStaticOpenmetricsConfig is mounted directly into the agent's own
// conf.d/openmetrics.d/conf.yaml (not a container AD label): it targets
// fake-haproxy's endpoint directly (http://fake-haproxy:8404/metrics, reachable
// since all compose fixtures in this suite share one Docker Compose network)
// and claims namespace "haproxy" — fake-haproxy's own metric namespace, via
// its `com.datadoghq.ad.check.id: haproxy` label matching haproxy's shipped
// `ad_identifiers: [haproxy]`. Since the static config isn't attached to
// fake-haproxy's container in any way, this proves suppression works purely
// off the shared, host-wide namespace index — complementing
// TestKrakendConfigDiscoverySuppressedByConflictingGenericIntegration's
// same-container "sibling" case above.
//
// TestKrakendConfigDiscovery (running throughout this same suite, hence
// alongside this host-level static config) is the non-conflict control:
// krakend's own namespace root ("krakend") doesn't match "haproxy", so its
// discovery must remain unaffected — re-asserted explicitly below.
func (s *configDiscoverySuite) TestHaproxyConfigDiscoverySuppressedByHostLevelGenericIntegration() {
	t := s.T()
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyHaproxyConfigDiscoverySuppressedByHostLevelGenericIntegration(c)
	}, 3*time.Minute, 10*time.Second, "host-level static openmetrics check should be running, haproxy discovery should be suppressed, and krakend discovery should be unaffected")
}

func (s *configDiscoverySuite) verifyHaproxyConfigDiscoverySuppressedByHostLevelGenericIntegration(c *assert.CollectT) {
	t := s.T()

	configCheckOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "configcheck")
	if !assert.True(c, strings.Contains(configCheckOutput, "=== openmetrics check ==="), "host-level static openmetrics check should appear in configcheck") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, "namespace: haproxy"), "static openmetrics config should carry the conflicting namespace") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.False(c, isIntegrationScheduled(configCheckOutput, "haproxy"), "haproxy discovery should be suppressed by the host-level static openmetrics config") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}

	statusOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "status", "collector", "--json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if !assert.NoError(c, err, "failed to parse collector status JSON") {
		t.Logf("status output: %s", statusOutput)
		return
	}

	// No haproxy instance should ever be running on this host: discovery must
	// be suppressed by the host-level static openmetrics config claiming its
	// namespace, even though that config isn't attached to fake-haproxy's
	// container at all.
	haproxyInstances := status.RunnerStats.Checks["haproxy"]
	if !assert.Empty(c, haproxyInstances,
		"no haproxy instance should be running (discovery must be suppressed by the host-level static config); found: %+v", haproxyInstances) {
		return
	}

	openmetricsInstances, exists := status.RunnerStats.Checks["openmetrics"]
	if !assert.True(c, exists, "host-level static openmetrics check should be running; available: %v", getCheckNames(status.RunnerStats.Checks)) {
		return
	}
	ran := false
	for name, stat := range openmetricsInstances {
		if len(stat.ExecutionTimes) > 0 {
			t.Logf("openmetrics instance %s: runs=%d", name, len(stat.ExecutionTimes))
			ran = true
			break
		}
	}
	if !assert.True(c, ran, "host-level static openmetrics check is configured but has not executed yet") {
		return
	}

	// krakend's own namespace root ("krakend") doesn't match the static
	// config's namespace ("haproxy"), so its discovery must remain
	// unaffected: exactly one krakend instance, same as TestKrakendConfigDiscovery.
	krakendInstances := status.RunnerStats.Checks["krakend"]
	assert.Len(c, krakendInstances, 1,
		"krakend discovery should be unaffected by the host-level static openmetrics config claiming an unrelated namespace; found: %+v", krakendInstances)
}
