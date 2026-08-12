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
}

// agentOptions returns the agentparams for the suite.
//
// Deliberately minimal. Preflight mode is eligible only while data_plane.enabled is still at its
// default: isEligible (comp/dataplane/preflightmode/impl/eligible.go) checks the setting's
// *source*, not just its value, so writing data_plane.enabled into datadog.yaml — even as
// false — would make this suite silently assert nothing at all. data_plane.preflight_mode is left
// unset for a similar reason: it defaults to true, and exercising the default is exercising
// what actually ships.
func (s *preflightModeSuite) agentOptions() []agentparams.Option {
	if s.extraAgentConfig == "" {
		return nil
	}
	return []agentparams.Option{agentparams.WithAgentConfig(s.extraAgentConfig)}
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
