// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed fixtures/openssl-default.cnf
var defaultOpenSSLConfig []byte

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

//go:embed fixtures/system-probe.yaml
var systemProbeConfig string

type LinuxFIPSComplianceSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxFIPSComplianceSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault)),
			ec2.WithAgentOptions(
				agentparams.WithFlavor("datadog-fips-agent"),
				// Install custom check that reports the FIPS mode of Python
				agentparams.WithFile(
					`/etc/datadog-agent/checks.d/e2e_fips_test.py`,
					fipsTestCheck,
					true,
				),
				agentparams.WithFile(
					`/etc/datadog-agent/conf.d/e2e_fips_test.yaml`,
					`
init_config:
instances: [{}]
`,
					false,
				),
			),
		),
	)),
	}

	e2e.Run(t, &LinuxFIPSComplianceSuite{}, suiteParams...)
}

func (v *LinuxFIPSComplianceSuite) TestFIPSDefaultConfig() {
	status := v.Env().RemoteHost.MustExecute("sudo datadog-agent status")
	assert.NotContains(v.T(), status, "FIPS mode requested (requirefips tag set) but not available: OpenSSL")
	assert.Contains(v.T(), status, "Status date")
	assert.Contains(v.T(), status, "FIPS Mode: enabled")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSNoFIPSProvider() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")
	v.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo sh -c "echo '%s' > /opt/datadog-agent/embedded/ssl/openssl.cnf"`, defaultOpenSSLConfig))

	status, err := v.Env().RemoteHost.Execute("sudo datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "FIPS mode requested (requirefips tag set) but not available: OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSEnabledNoOpenSSLConfig() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")

	status, err := v.Env().RemoteHost.Execute("sudo datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "FIPS mode requested (requirefips tag set) but not available: OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}

func (v *LinuxFIPSComplianceSuite) TestReportsFIPSStatusMetrics() {
	// Test that the custom check from our fixtures is able to report metrics while
	// in FIPS mode. These metric values are based on the status of Python's FIPS mode.
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("e2e.fips_mode", fakeintakeclient.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'e2e.fips_mode' with value higher than 0 yet")

		metrics, err = v.Env().FakeIntake.Client().FilterMetrics("e2e.fips_cryptography", fakeintakeclient.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'e2e.fips_cryptography' with value higher than 0 yet")

		metrics, err = v.Env().FakeIntake.Client().FilterMetrics("e2e.fips_ssl", fakeintakeclient.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'e2e.fips_ssl' with value higher than 0 yet")
	}, 5*time.Minute, 10*time.Second)
}

// this test check that the FIPS Agent processes are loaded with the FIPS OpenSSL libraries
func (v *LinuxFIPSComplianceSuite) TestFIPSEnabledLoadedOPENSSLLibs() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault)),
			ec2.WithAgentOptions(
				agentparams.WithFlavor("datadog-fips-agent"),
				agentparams.WithSecurityAgentConfig(securityAgentConfig),
				agentparams.WithSystemProbeConfig(systemProbeConfig),
			),
		),
	))

	paths := []string{
		"/opt/datadog-agent/bin/agent/agent",
		"/opt/datadog-agent/embedded/bin/trace-agent",
		"/opt/datadog-agent/embedded/bin/process-agent",
		"/opt/datadog-agent/embedded/bin/security-agent",
		"/opt/datadog-agent/embedded/bin/system-probe",
	}
	var pid string
	var err error

	// wake up the trace-agent
	// NB: the endpoint is no-op
	v.Env().RemoteHost.NewHTTPClient().Get("http://localhost:8126/services")

	for _, agentPath := range paths {
		v.T().Run(agentPath, func(t *testing.T) {
			t.Logf("Checking loaded libraries for %v", agentPath)
			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				pid, err = v.Env().RemoteHost.Host.Execute(fmt.Sprintf("pidof %v", agentPath))
				require.NoError(collect, err)
				pid = strings.TrimSpace(pid)
				require.NotEmpty(collect, pid)
			}, time.Second*10, time.Second)
			loadedLibs, err := v.Env().RemoteHost.Host.Execute(fmt.Sprintf("sudo cat /proc/%s/maps", pid))
			require.NoError(t, err)
			assert.Contains(t, loadedLibs, "/opt/datadog-agent/embedded/lib/ossl-modules/fips.so")
		})
	}
}
