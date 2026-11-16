// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetry

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-telemetry/newerrors"
)

// agentVersionRe extracts the version from the output of "agent version"
// ("Agent 7.85.0-rc.2 - Meta: ..."), without build metadata, matching the
// agent.version tag.
var agentVersionRe = regexp.MustCompile(`Agent (\d+\.\d+\.\d+(?:-[^\s+]+)?)`)

// stackOffsetRe matches the "+0x..." offset of a stack frame.
var stackOffsetRe = regexp.MustCompile(`\+0x[0-9a-f]+`)

type errorTrackingIdentitySuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAgentTelemetryErrorTrackingIdentitySuite provisions one host on the build
// under test, with errortracking enabled through local config, to check the
// release identity tags and the new-error comparison on real payloads.
func TestAgentTelemetryErrorTrackingIdentitySuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &errorTrackingIdentitySuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(errorTrackingAgentOptions(errorTrackingEnabledConfig)...),
				),
			),
		),
	)
}

// tagValue returns the value of key in a comma-separated tags string.
func tagValue(tags, key string) (string, bool) {
	for _, tag := range strings.Split(tags, ",") {
		if v, ok := strings.CutPrefix(tag, key+":"); ok {
			return v, true
		}
	}
	return "", false
}

// TestIdentityTags asserts every record carries agent.version equal to the
// running agent version and an agent.rc value consistent with it.
func (s *errorTrackingIdentitySuite) TestIdentityTags() {
	dumpDiagnosticsOnFailure(s.T(), s.Env())

	match := agentVersionRe.FindStringSubmatch(s.Env().Agent.Client.Version())
	require.Len(s.T(), match, 2, "could not parse the running agent version")
	runningVersion := match[1]
	wantRC := "false"
	if strings.Contains(runningVersion, "-rc.") {
		wantRC = "true"
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)
		require.NotEmpty(c, logs, "no error logs received yet")
		for _, l := range logs {
			version, ok := tagValue(l.Tags, "agent.version")
			assert.True(c, ok, "tags must carry agent.version; got: %q", l.Tags)
			assert.Equal(c, runningVersion, version)
			rc, ok := tagValue(l.Tags, "agent.rc")
			assert.True(c, ok, "tags must carry agent.rc; got: %q", l.Tags)
			assert.Equal(c, wantRC, rc)
		}
	}, 2*time.Minute, 5*time.Second, "timed out waiting for error logs carrying identity tags")
}

// TestNewErrorDetection runs the two-cohort comparison on payloads collected
// from FakeIntake. The candidate cohort is what the build under test emitted.
// The baseline is synthetic: the same events minus one seeded fingerprint, with
// offsets rewritten. Fingerprint stability across builds (offsets, line
// numbers, paths) is proven by the newerrors unit tests, not here. No real previous release
// emits errortracking with identity tags yet, and versions are never faked
// inside a real agent.
func (s *errorTrackingIdentitySuite) TestNewErrorDetection() {
	dumpDiagnosticsOnFailure(s.T(), s.Env())

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		var err error
		logs, err = s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)
		distinct := map[string]struct{}{}
		for _, l := range logs {
			if fp := newerrors.Fingerprint(l.StackTrace); fp != "" {
				distinct[fp] = struct{}{}
			}
		}
		assert.GreaterOrEqual(c, len(distinct), 2, "need at least two distinct error fingerprints")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for two distinct error fingerprints")

	var seeded string
	for _, l := range logs {
		if seeded = newerrors.Fingerprint(l.StackTrace); seeded != "" {
			break
		}
	}
	require.NotEmpty(s.T(), seeded, "no log with a usable stack")

	var candidate, baseline newerrors.Cohort
	for _, l := range logs {
		candidate.Events = append(candidate.Events, newerrors.Event{Host: "candidate", StackTrace: l.StackTrace, Count: l.Count})
		if newerrors.Fingerprint(l.StackTrace) == seeded {
			continue
		}
		baseline.Events = append(baseline.Events, newerrors.Event{
			Host:       "baseline",
			StackTrace: stackOffsetRe.ReplaceAllString(l.StackTrace, "+0x1"),
			Count:      l.Count,
		})
	}

	found := newerrors.FindNew(baseline, candidate)
	require.Len(s.T(), found, 1, "only the seeded error may be reported as new")
	assert.Equal(s.T(), seeded, found[0].Fingerprint)
	assert.Positive(s.T(), found[0].RatePerHost)

	// The reverse comparison finds nothing: every baseline error is in the candidate.
	assert.Empty(s.T(), newerrors.FindNew(candidate, baseline))
}
