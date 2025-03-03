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

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
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
	e2e.Run(t, &LinuxFIPSComplianceSuite{},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault)),
			awshost.WithAgentOptions(agentparams.WithFlavor("datadog-fips-agent")),
		)),
	)
}

func (v *LinuxFIPSComplianceSuite) TestFIPSDefaultConfig() {
	_, err := v.Env().RemoteHost.Execute("sudo GOFIPS=0 datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "the 'requirefips' build tag is enabled, but it conflicts with the detected env variable GOFIPS=0 which would disable FIPS mode")

	status := v.Env().RemoteHost.MustExecute("sudo datadog-agent status")
	assert.NotContains(v.T(), status, "can't enable FIPS mode for OpenSSL")
	assert.Contains(v.T(), status, "Status date")
	assert.Contains(v.T(), status, "FIPS Mode: enabled")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSNoFIPSProvider() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")
	v.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo sh -c "echo '%s' > /opt/datadog-agent/embedded/ssl/openssl.cnf"`, defaultOpenSSLConfig))

	status, err := v.Env().RemoteHost.Execute("sudo datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "can't enable FIPS mode for OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSEnabledNoOpenSSLConfig() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")

	status, err := v.Env().RemoteHost.Execute("sudo datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "can't enable FIPS mode for OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}

// this test check that the FIPS Agent processes are loaded with the FIPS OpenSSL libraries
func (v *LinuxFIPSComplianceSuite) TestFIPSEnabledLoadedOPENSSLLibs() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault)),
		awshost.WithAgentOptions(
			agentparams.WithFlavor("datadog-fips-agent"),
			agentparams.WithSecurityAgentConfig(securityAgentConfig),
			agentparams.WithSystemProbeConfig(systemProbeConfig),
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

	for _, agentPath := range paths {
		v.T().Logf("Checking loaded libraries for %v", agentPath)
		v.EventuallyWithT(func(collect *assert.CollectT) {
			pid, err = v.Env().RemoteHost.Host.Execute(fmt.Sprintf("pidof %v", agentPath))
			require.NoError(collect, err)
			pid = strings.TrimSpace(pid)
			require.NotEmpty(collect, pid)
		}, time.Second*10, time.Second)
		loadedLibs := v.Env().RemoteHost.Host.MustExecute(fmt.Sprintf("sudo cat /proc/%s/maps", pid))
		assert.Contains(v.T(), loadedLibs, "/opt/datadog-agent/embedded/lib/ossl-modules/fips.so")
	}
}
