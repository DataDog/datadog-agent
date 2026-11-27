// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preflightmode contains e2e tests for the Agent Data Plane (ADP) preflight mode
// implemented in comp/dataplane/preflightmode.
//
// Preflight mode runs ADP for a short, isolated window at Agent startup so that
// environment-specific startup problems surface before ADP goes GA. These tests assert that
// the pre-flight really happens on each supported platform.
package preflightmode

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// probeMetricName is the throwaway gauge the Core Agent pushes through the preflight ADP's
// DogStatsD endpoint. It is declared as probeMetricName in
// comp/dataplane/preflightmode/impl/report.go; the two must stay in sync.
//
// This metric is the reason the suite exists. It can only reach the intake if ADP was
// spawned, parsed the generated config, bound the throwaway endpoint, and then aggregated,
// serialized and forwarded the sample. Observing it here therefore proves the pre-flight ran
// end to end, which no amount of log scraping on the Core Agent side can establish.
const probeMetricName = "n_o_i_n_d_e_x.datadog.agent.data_plane.preflight_mode.probe"

// agentVersionTagPrefix is the prefix of the tag the probe stamps with the Core Agent's
// version. Asserting the prefix rather than a value keeps the suite independent of whichever
// build the pipeline produced.
const agentVersionTagPrefix = "agent_version:"

// probeTimeout bounds how long the probe metric may take to reach the intake after an Agent
// restart. The Core Agent gives ADP 20s to bind its endpoint (listenerTimeout in
// comp/dataplane/preflightmode/impl/preflightmode.go) and the sample then has to survive one ADP
// flush, so it normally lands well inside a minute. The rest is margin for slow hosts —
// notably the macOS dedicated hosts, which are the slowest infra this suite runs on.
const probeTimeout = 5 * time.Minute

// preflightModeDuration is how long the suite has the pre-flight leave ADP running, set through
// data_plane.preflight_mode_duration. The default is 90s (minPreflightModeDuration in
// comp/dataplane/preflightmode/impl/preflightmode.go).
//
// It is raised so that the agent telemetry tests are deterministic. The run's outcome counters
// are delta-converted by agent telemetry, and both the scheduled flush and every
// `show-metadata agent-telemetry` call consume the delta, so the outcome is visible to exactly
// one of them. The data-plane-preflight-mode profile first flushes 120s after the Agent starts
// (start_after in comp/core/agenttelemetry/impl/defaultProfiles.yaml), which with the default
// duration lands shortly after the run reports and races the test for it. At 180s the first
// flush lands mid-run and finds nothing, and the next one is 900s later, leaving the test a
// window of many minutes. This is the extended-window case the profile's recurring schedule is
// designed for.
const preflightModeDuration = 180 * time.Second

// telemetryTimeout bounds how long the pre-flight's outcome may take to appear in the agent
// telemetry payload after an Agent restart: the run itself, then ADP's shutdown, with the rest
// as margin for slow hosts.
const telemetryTimeout = preflightModeDuration + 5*time.Minute

// The names the pre-flight's metrics appear under in the agent telemetry payload. These are the
// names in the data-plane-preflight-mode profile, with a dot between subsystem and name, rather
// than the Prometheus names (data_plane__preflight_mode_result and so on) the Core Agent
// registers them under.
const (
	preflightModeResultMetric   = "data_plane.preflight_mode_result"
	preflightModeFindingMetric  = "data_plane.preflight_mode_finding"
	preflightModeDurationMetric = "data_plane.preflight_mode_duration_seconds"
)

// The tag keys each metric carries in the payload: the profile's preserve_tags, plus the emitter
// tag agent telemetry adds to every series. A series carries every one of its keys, including
// those whose value is empty for that finding.
//
// preflightModeFindingTagKeys must match the preserve_tags of data_plane.preflight_mode_finding
// in comp/core/agenttelemetry/impl/defaultProfiles.yaml exactly. datadog-agent#61182 adds
// config_key and worker_name to that list: whichever of #61182 and #61183 merges second must add
// the other's keys here.
var (
	preflightModeResultTagKeys   = []string{"emitter", "result"}
	preflightModeFindingTagKeys  = []string{"emitter", "finding", "source_file", "source_line", "spawn_stage", "spawn_error"}
	preflightModeDurationTagKeys = []string{"emitter"}
)

// preflightModeSuite is the platform-agnostic body of the suite.
//
// Each platform gets its own suite and its own CI job rather than one suite parameterized
// over descriptors. Preflight mode's eligibility rules and DogStatsD transport genuinely differ
// per platform, and a per-platform job means a failure names the platform that regressed
// instead of implicating a shared job that other teams also watch.
type preflightModeSuite struct {
	e2e.BaseSuite[environments.Host]

	// descriptor is the OS to provision.
	descriptor e2eos.Descriptor

	// goos is the runtime.GOOS value the Core Agent stamps into the probe metric's os: tag.
	// It is hardcoded per platform rather than derived from descriptor so that provisioning
	// the wrong OS cannot quietly satisfy the assertion.
	goos string

	// extraAgentConfig is written to datadog.yaml. It must never mention data_plane.enabled;
	// see agentOptions.
	extraAgentConfig string

	// restartAgent restarts the Core Agent. Every Agent start runs exactly one preflight mode
	// window, so this is what triggers the behaviour under test.
	restartAgent func(*components.RemoteHost) error

	// agentLogPath is where the Core Agent writes agent.log, used only for failure diagnostics.
	agentLogPath string

	// grepPreflightMode builds the command that prints the Agent log lines mentioning preflight mode.
	// Platform-specific because the shells differ.
	grepPreflightMode func(logPath string) string

	// appendAgentConfig appends a single line of YAML to the installed datadog.yaml, and returns a
	// function that puts the original file back. It does not restart the Agent. Platform-specific
	// because the config path and the shells differ.
	appendAgentConfig func(host *components.RemoteHost, line string) (restore func() error, err error)
}

// agentOptions returns the agentparams for the suite.
//
// Deliberately minimal. Preflight mode is eligible only while data_plane.enabled is still at its
// default: isEligible (comp/dataplane/preflightmode/impl/eligible.go) checks the setting's
// *source*, not just its value, so writing data_plane.enabled into datadog.yaml — even as
// false — would make this suite silently assert nothing at all. data_plane.preflight_mode is left
// unset for a similar reason: it defaults to true, and exercising the default is exercising
// what actually ships.
//
// The one setting the suite does change is data_plane.preflight_mode_duration; see
// preflightModeDuration. extraAgentConfig must therefore not have a data_plane section of its own.
func (s *preflightModeSuite) agentOptions() []agentparams.Option {
	config := fmt.Sprintf("data_plane:\n  preflight_mode_duration: %s\n", preflightModeDuration)
	if s.extraAgentConfig != "" {
		config += s.extraAgentConfig
	}
	return []agentparams.Option{agentparams.WithAgentConfig(config)}
}

func (s *preflightModeSuite) suiteOptions() []e2e.SuiteOption {
	return []e2e.SuiteOption{
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor)),
					ec2.WithAgentOptions(s.agentOptions()...),
				),
			),
		),
	}
}

// TestProbeMetricReachesIntake asserts that starting the Agent causes ADP to run in preflight mode
// and deliver its probe metric to the intake.
func (s *preflightModeSuite) TestProbeMetricReachesIntake() {
	host := s.Env().RemoteHost
	fakeintake := s.Env().FakeIntake.Client()

	// A preflight mode window runs once per Agent start, so by the time this test body runs the
	// interesting event already happened during provisioning. Reset the intake and restart the
	// Agent so the metric being asserted on is one this test caused. This also makes the test
	// retryable on the same host, which is what our retry logic prefers: a retry re-runs the
	// pre-flight rather than re-reading a payload left over from provisioning.
	require.NoError(s.T(), fakeintake.FlushServerAndResetAggregators(),
		"could not reset the fakeintake aggregators")
	require.NoError(s.T(), s.restartAgent(host), "could not restart the Agent")

	// preflight_mode:true distinguishes the probe from anything else that could carry this metric
	// name, and os: pins the sample to the platform this suite provisioned — without it a
	// misconfigured suite could pass on a payload from the wrong host.
	expectedTags := []string{"preflight_mode:true", "os:" + s.goos}

	found := s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.FilterMetrics(probeMetricName,
			client.WithTags[*aggregator.MetricSeries](expectedTags))
		if !assert.NoError(c, err, "could not query fakeintake for %s", probeMetricName) {
			return
		}
		assert.NotEmpty(c, metrics, "no %s series tagged %v has arrived yet",
			probeMetricName, expectedTags)
	}, probeTimeout, 10*time.Second,
		"the Agent Data Plane preflight mode probe metric never reached the intake")

	if !found {
		s.dumpDiagnostics(host)
		return
	}

	// The series are in the intake now, so re-query synchronously rather than smuggling them
	// out of the retry callback, which runs on another goroutine.
	matched, err := fakeintake.FilterMetrics(probeMetricName,
		client.WithTags[*aggregator.MetricSeries](expectedTags))
	require.NoError(s.T(), err, "could not re-query fakeintake for %s", probeMetricName)
	require.NotEmpty(s.T(), matched, "%s disappeared from the intake between queries", probeMetricName)

	// The probe also carries the Core Agent's version. Checking it is present and non-empty
	// catches a probe that fired with an unpopulated version, which would make the metric
	// useless for correlating pre-flight failures back to a build once this ships.
	for _, series := range matched {
		version, ok := tagValue(series.GetTags(), agentVersionTagPrefix)
		if !assert.Truef(s.T(), ok, "%s series is missing an %s tag; got %v",
			probeMetricName, agentVersionTagPrefix, series.GetTags()) {
			continue
		}
		assert.NotEmptyf(s.T(), version, "%s series has an empty %s tag",
			probeMetricName, agentVersionTagPrefix)
	}
}

// acceptableHealthyResults are the results TestOutcomeInAgentTelemetry accepts from a run on an
// unmodified host. It is an allowlist so that any other result, including one added later, fails
// the test.
//
// TODO: require only "clean" once datadog-agent#61179 (the ADP 1.8.1 standalone-mode warning
// target in expectedWarnings) has merged and the bundled ADP includes saluki#3391
// (dogstatsd_metrics_stats_enable treated as fully supported, 1.8.x) or later. Until then, ADP
// 1.8.1 logs two warnings on every pre-flight run that the pre-flight does not yet recognize as
// its own, so a healthy run reports warnings_in_log.
var acceptableHealthyResults = []string{"clean", "warnings_in_log"}

// TestOutcomeInAgentTelemetry asserts that the outcome of a pre-flight run on an unmodified host
// reaches the agent telemetry payload with the tags the data-plane-preflight-mode profile
// preserves, and that the run did not fail.
//
// The reporter's unit tests use mock telemetry and the profile's unit test only checks the
// allowlist. This instead reads the payload the running Agent builds, through
// `show-metadata agent-telemetry`, so it covers the profile's filtering, tag aggregation and
// serialization on a real host. That command builds the payload on demand for every profile,
// regardless of the profile's schedule.
//
// A healthy run is not guaranteed to emit any finding series, so the finding metric's tag set is
// covered by TestFindingTagsInAgentTelemetry instead.
func (s *preflightModeSuite) TestOutcomeInAgentTelemetry() {
	metrics, ok := s.restartAndAwaitOutcome()
	if !ok {
		return
	}

	findings := metrics[preflightModeFindingMetric]
	result := metrics[preflightModeResultMetric][0].Tags["result"]
	assert.Containsf(s.T(), acceptableHealthyResults, result,
		"the pre-flight failed on an unmodified host; findings: %v", findings)

	switch result {
	case "clean":
		assert.Empty(s.T(), findings, "a clean run should report no findings")
	case "warnings_in_log":
		// Show which warnings fired, so a run can tell the known false positives from a new one.
		for _, f := range findings {
			s.T().Logf("%s finding from %s:%s", f.Tags["finding"], f.Tags["source_file"], f.Tags["source_line"])
		}
	}
}

// TestFindingTagsInAgentTelemetry provokes a warnings_in_log finding and asserts that findings
// reach the agent telemetry payload with every label the profile preserves, including those that
// are empty for that finding.
//
// min_tls_version is a partially supported key in ADP's schema overlay, so ADP's configuration
// compatibility check logs a warning for it when it is set. tlsv1.0 is not the default, so it
// warns even in ADP versions that skip keys set to their default value, and the pre-flight does
// not expect it (expectedWarnings in comp/dataplane/preflightmode/impl/capture.go). This
// guarantees at least one warnings_in_log finding even once the pre-flight's own false positives
// are fixed. With the bundled ADP 1.8.1, healthy runs already report such findings (see
// acceptableHealthyResults), so this test checks the finding tag set end to end, not that this
// particular warning arrived. The Core Agent itself only lowers its TLS floor, which does not
// affect its connection to the intake.
func (s *preflightModeSuite) TestFindingTagsInAgentTelemetry() {
	host := s.Env().RemoteHost

	restore, err := s.appendAgentConfig(host, "min_tls_version: tlsv1.0")
	require.NoError(s.T(), err, "could not add min_tls_version to datadog.yaml")
	s.T().Cleanup(func() {
		// Leave the host as the other tests expect it, whatever order they run in.
		assert.NoError(s.T(), restore(), "could not restore datadog.yaml")
		assert.NoError(s.T(), s.restartAgent(host), "could not restart the Agent after restoring datadog.yaml")
	})

	metrics, ok := s.restartAndAwaitOutcome()
	if !ok {
		return
	}

	assert.Equal(s.T(), "warnings_in_log", metrics[preflightModeResultMetric][0].Tags["result"],
		"unexpected pre-flight result")

	// The run logs other warnings too (with ADP 1.8.1, the pre-flight's known false positives),
	// each its own series, so check every warnings_in_log series rather than requiring exactly one.
	// Their locations are ADP's file!() and line!(), which move between ADP versions, so only check
	// that they are present.
	var warnings []atelMetric
	for _, f := range metrics[preflightModeFindingMetric] {
		if f.Tags["finding"] == "warnings_in_log" {
			warnings = append(warnings, f)
		}
	}
	require.NotEmpty(s.T(), warnings, "no warnings_in_log finding was reported: %v", metrics[preflightModeFindingMetric])
	for _, w := range warnings {
		assert.NotEmpty(s.T(), w.Tags["source_file"], "warnings_in_log finding has no source_file: %v", w.Tags)
		assert.NotEmpty(s.T(), w.Tags["source_line"], "warnings_in_log finding has no source_line: %v", w.Tags)

		// The spawn labels only apply to spawn_failed, but every series still carries them.
		for _, key := range []string{"spawn_stage", "spawn_error"} {
			value, present := w.Tags[key]
			if assert.True(s.T(), present, "warnings_in_log finding is missing %s: %v", key, w.Tags) {
				assert.Empty(s.T(), value, "warnings_in_log finding has a non-empty %s: %v", key, w.Tags)
			}
		}
	}
}

// restartAndAwaitOutcome restarts the Agent, waits for the pre-flight run that restart causes to
// appear in the agent telemetry payload, and returns that payload's metrics. It asserts what holds
// for any run: a single result counting one run, the run's duration, and the exact tag set of
// every series. It returns false if the outcome never appeared or has no single result to assert
// on further.
func (s *preflightModeSuite) restartAndAwaitOutcome() (map[string][]atelMetric, bool) {
	host := s.Env().RemoteHost

	// As in TestProbeMetricReachesIntake, restart so the run being asserted on is one this test
	// caused. A fresh Agent process also starts with empty delta state, so the run's counters show
	// up as exactly 1.
	require.NoError(s.T(), s.restartAgent(host), "could not restart the Agent")

	// Diagnose fails the test outright, rather than returning an error, if the Agent's API is not
	// up yet, so wait for the restarted Agent before polling it.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second, "the Agent did not come back after the restart")

	// Building the payload consumes the counters' deltas, so the outcome is in exactly one
	// snapshot. Keep that snapshot rather than re-querying, and assert on it outside the retry
	// loop, so that a wrong tag set fails with that message instead of as a missing metric.
	var output string
	var metrics map[string][]atelMetric
	found := s.EventuallyWithT(func(c *assert.CollectT) {
		out := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-telemetry"}))
		parsed, err := parseAgentTelemetryMetrics(out)
		if !assert.NoError(c, err) {
			return
		}
		if assert.NotEmpty(c, parsed[preflightModeResultMetric], "%s has not been reported yet", preflightModeResultMetric) {
			output, metrics = out, parsed
		}
	}, telemetryTimeout, 10*time.Second,
		"the Agent Data Plane preflight mode outcome never appeared in the agent telemetry payload")

	if !found {
		s.dumpDiagnostics(host)
		return nil, false
	}
	// Registered after the caller's own cleanups, so it runs before them: the diagnostics come
	// from the host as the test left it.
	s.T().Cleanup(func() {
		if s.T().Failed() {
			s.T().Logf("agent telemetry payload:\n%s", output)
			s.dumpDiagnostics(host)
		}
	})

	// Exactly one run happened since the restart, and it reports exactly one result.
	results := metrics[preflightModeResultMetric]
	if !assert.Lenf(s.T(), results, 1, "expected a single %s series: %v", preflightModeResultMetric, results) {
		return nil, false
	}
	assert.ElementsMatch(s.T(), preflightModeResultTagKeys, tagKeys(results[0].Tags),
		"unexpected tag set on %s", preflightModeResultMetric)
	assert.Equal(s.T(), 1.0, results[0].Value, "%s should count the one run since the restart", preflightModeResultMetric)

	// The run's duration is a gauge rather than a counter, so it is not consumed, and it is set by
	// every run, clean or not. A run left alone for the whole window lasts at least that long.
	durations := metrics[preflightModeDurationMetric]
	if assert.Lenf(s.T(), durations, 1, "expected a single %s series: %v", preflightModeDurationMetric, durations) {
		assert.ElementsMatch(s.T(), preflightModeDurationTagKeys, tagKeys(durations[0].Tags),
			"unexpected tag set on %s", preflightModeDurationMetric)
		assert.GreaterOrEqual(s.T(), durations[0].Value, preflightModeDuration.Seconds(),
			"%s is shorter than the configured window", preflightModeDurationMetric)
	}

	// Every finding series carries every label, including those that do not apply to it, which
	// arrive with an empty value.
	for _, f := range metrics[preflightModeFindingMetric] {
		assert.ElementsMatch(s.T(), preflightModeFindingTagKeys, tagKeys(f.Tags),
			"unexpected tag set on %s", preflightModeFindingMetric)
	}

	return metrics, true
}

// dumpDiagnostics prints the Agent's preflight mode log lines.
//
// The host is destroyed as soon as the suite finishes, so a CI failure that says only "the
// metric never arrived" cannot be investigated afterwards. These lines separate the two
// failures that look identical from the intake's point of view: the pre-flight never started
// (no lines at all, so check ADP is installed and that data_plane.enabled is still at its
// default source), or it ran and reported findings.
//
// Best-effort: a host broken enough to fail the assertion may fail this too.
func (s *preflightModeSuite) dumpDiagnostics(host *components.RemoteHost) {
	out, err := host.Execute(s.grepPreflightMode(s.agentLogPath))
	if err != nil {
		s.T().Logf("could not read preflight mode lines from %s: %v", s.agentLogPath, err)
		return
	}
	if strings.TrimSpace(out) == "" {
		s.T().Logf("no preflight mode lines in %s: the pre-flight never started", s.agentLogPath)
		return
	}
	s.T().Logf("preflight mode lines from %s:\n%s", s.agentLogPath, out)
}

// tagValue returns the value of the first tag carrying prefix.
func tagValue(tags []string, prefix string) (string, bool) {
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return strings.TrimPrefix(tag, prefix), true
		}
	}
	return "", false
}

// nixGrepPreflightMode is shared by the Linux and macOS suites, which read the same kind of text
// log through the same shell. The `|| true` keeps a no-match grep — which exits 1 — from being
// reported as a command failure, because "no lines" is itself the diagnostic we want.
func nixGrepPreflightMode(logPath string) string {
	return fmt.Sprintf("sudo grep -i 'preflight mode' %s | tail -n 50 || true", logPath)
}

// nixAppendAgentConfig is shared by the Linux and macOS suites. It backs datadog.yaml up next to
// itself, keeping its owner and mode, so that restoring it is a move. line is single-quoted for
// the shell, so it must not contain a single quote. The leading newline guards against a file
// that does not end in one.
func nixAppendAgentConfig(configPath string) func(*components.RemoteHost, string) (func() error, error) {
	return func(host *components.RemoteHost, line string) (func() error, error) {
		backup := configPath + ".preflight-e2e.bak"
		if _, err := host.Execute(fmt.Sprintf("sudo cp -p %s %s", configPath, backup)); err != nil {
			return nil, err
		}
		restore := func() error {
			_, err := host.Execute(fmt.Sprintf("sudo mv %s %s", backup, configPath))
			return err
		}
		if _, err := host.Execute(fmt.Sprintf(`printf '\n%%s\n' '%s' | sudo tee -a %s > /dev/null`, line, configPath)); err != nil {
			return nil, errors.Join(err, restore())
		}
		return restore, nil
	}
}
