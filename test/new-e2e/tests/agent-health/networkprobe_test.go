// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type networkProbeSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestNetworkProbeSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &networkProbeSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(healthPlatformAgentConfig),
				),
			),
		)),
	)
}

// TestNetworkProbeInitFailureLifecycle exercises the two independent error paths in
// createNetworkTracerModule and verifies that each produces the correct health issue
// in fakeintake and transitions to RESOLVED when fixed.
//
//  1. Kernel check failure (errNetworkProbeKernelUnsupported): simulated by adding the
//     running kernel to excluded_linux_versions. Asserts network-probe-kernel-unsupported
//     appears as NEW, then RESOLVED after the exclusion is removed.
//
//  2. USM tracer init failure (errNetworkProbeUSMUnsupported): only exercised when the
//     running kernel is < 4.14 (USM minimum). Asserts network-probe-usm-unsupported
//     appears as NEW with USM enabled and no kernel exclusion, then RESOLVED after USM
//     is disabled. Skipped on modern kernels where USM is supported.
func (suite *networkProbeSuite) TestNetworkProbeInitFailureLifecycle() {
	fakeIntake := suite.Env().FakeIntake.Client()

	// Detect the running kernel version once for both sub-tests.
	unameR := strings.TrimSpace(suite.Env().RemoteHost.MustExecute("uname -r"))
	var major, minor, patch int
	_, _ = fmt.Sscanf(unameR, "%d.%d.%d", &major, &minor, &patch)

	// Build a YAML exclusion list covering all possible patch values for major.minor
	// so the kernel exclusion fires even when vDSO encodes the ABI in the patch byte.
	var exclusionYAML strings.Builder
	for p := 0; p <= 255; p++ {
		fmt.Fprintf(&exclusionYAML, "    - %d.%d.%d\n", major, minor, p)
	}
	kernelExclusionConfig := "network_config:\n  enabled: true\nsystem_probe_config:\n  excluded_linux_versions:\n" + exclusionYAML.String()

	// -------------------------------------------------------------------------
	// Step 1 — Kernel check failure: assert network-probe-kernel-unsupported NEW
	// -------------------------------------------------------------------------
	suite.T().Run("KernelIssueDetection", func(t *testing.T) {
		suite.UpdateEnv(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(healthPlatformAgentConfig),
					agentparams.WithSystemProbeConfig(kernelExclusionConfig),
				),
			),
		))
		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

		defer logNetworkProbeLogsOnFailure(t, suite)

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, "network-probe-kernel-unsupported") {
					if iss.PersistedIssue != nil &&
						(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW ||
							iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ONGOING) {
						return
					}
				}
			}
			assert.Fail(ct, "network-probe-kernel-unsupported not found as NEW or ONGOING")
		}, defaultIssueTimeout, defaultIssuePollInterval, "kernel issue not detected in fakeintake")
	})

	// -------------------------------------------------------------------------
	// Step 2 — Kernel fix: remove exclusion, assert network-probe-kernel-unsupported RESOLVED
	// -------------------------------------------------------------------------
	suite.T().Run("KernelIssueResolution", func(t *testing.T) {
		suite.UpdateEnv(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(healthPlatformAgentConfig),
					agentparams.WithSystemProbeConfig("network_config:\n  enabled: true\n"),
				),
			),
		))

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, "network-probe-kernel-unsupported") {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "network-probe-kernel-unsupported not found as RESOLVED")
		}, defaultIssueTimeout, defaultIssuePollInterval, "kernel issue never transitioned to RESOLVED")
	})

	// -------------------------------------------------------------------------
	// Step 3 — USM check failure: only exercised on kernels < 4.14
	// -------------------------------------------------------------------------
	// USM requires kernel >= 4.14. On modern E2E environments (5.x+) this step
	// is skipped because NewTracer would succeed and errNetworkProbeUSMUnsupported
	// would never fire.
	const usmMinMajor, usmMinMinor = 4, 14
	usmUnsupported := major < usmMinMajor || (major == usmMinMajor && minor < usmMinMinor)

	suite.T().Run("USMIssueDetection", func(t *testing.T) {
		if !usmUnsupported {
			t.Skipf("kernel %d.%d.%d >= %d.%d: USM is supported, skipping USM failure scenario", major, minor, patch, usmMinMajor, usmMinMinor)
		}

		suite.UpdateEnv(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(healthPlatformAgentConfig),
					// No kernel exclusion — kernel check passes. USM enabled on an
					// unsupported kernel so NewTracer fails with errNetworkProbeUSMUnsupported.
					agentparams.WithSystemProbeConfig("network_config:\n  enabled: true\nservice_monitoring_config:\n  enabled: true\n"),
				),
			),
		))
		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

		defer logNetworkProbeLogsOnFailure(t, suite)

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, "network-probe-usm-unsupported") {
					if iss.PersistedIssue != nil &&
						(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW ||
							iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ONGOING) {
						return
					}
				}
			}
			assert.Fail(ct, "network-probe-usm-unsupported not found as NEW or ONGOING")
		}, defaultIssueTimeout, defaultIssuePollInterval, "USM issue not detected in fakeintake")
	})

	// -------------------------------------------------------------------------
	// Step 4 — USM fix: disable USM, assert network-probe-usm-unsupported RESOLVED
	// -------------------------------------------------------------------------
	suite.T().Run("USMIssueResolution", func(t *testing.T) {
		if !usmUnsupported {
			t.Skipf("kernel %d.%d.%d >= %d.%d: USM is supported, skipping USM resolution scenario", major, minor, patch, usmMinMajor, usmMinMinor)
		}

		suite.UpdateEnv(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(healthPlatformAgentConfig),
					agentparams.WithSystemProbeConfig("network_config:\n  enabled: true\n"),
				),
			),
		))

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, "network-probe-usm-unsupported") {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "network-probe-usm-unsupported not found as RESOLVED")
		}, defaultIssueTimeout, defaultIssuePollInterval, "USM issue never transitioned to RESOLVED")
	})
}

func logNetworkProbeLogsOnFailure(t *testing.T, suite *networkProbeSuite) {
	t.Helper()
	if !t.Failed() {
		return
	}
	status, _ := suite.Env().RemoteHost.Execute("sudo systemctl status datadog-agent-sysprobe --no-pager -l")
	t.Logf("system-probe status:\n%s", status)
	logs, _ := suite.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent-sysprobe -n 100 --no-pager 2>&1 || sudo cat /var/log/datadog/system-probe.log 2>&1 | tail -100")
	t.Logf("system-probe logs:\n%s", logs)
}
