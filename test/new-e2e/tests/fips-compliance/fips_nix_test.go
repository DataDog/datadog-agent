// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"
	"fmt"
	"time"

	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/openssl-default.cnf
var defaultOpenSSLConfig []byte

type LinuxFIPSComplianceSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxFIPSComplianceSuite(t *testing.T) {
	e2e.Run(t, &LinuxFIPSComplianceSuite{},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault)),
			awshost.WithAgentOptions(agentparams.WithFlavor("datadog-fips-agent")),
		)),
		e2e.WithSkipDeleteOnFailure(),
	)
}

func (v *LinuxFIPSComplianceSuite) TestFIPSDefaultConfig() {
	status := v.Env().RemoteHost.MustExecute("sudo GOFIPS=0 datadog-agent status")
	assert.NotContains(v.T(), status, "can't enable FIPS mode for OpenSSL")
	assert.Contains(v.T(), status, "Status date")
	assert.Contains(v.T(), status, "FIPS compliant: false")

	v.Env().RemoteHost.MustExecute("sudo systemctl set-environment GOFIPS=1")
	v.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent")

	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		status = v.Env().RemoteHost.MustExecute("sudo GOFIPS=1 datadog-agent status")
		assert.NotContains(t, status, "can't enable FIPS mode for OpenSSL")
		assert.Contains(t, status, "Status date")
		assert.Contains(t, status, "FIPS compliant: true")
	}, 60*time.Second, 5*time.Second)

	v.Env().RemoteHost.MustExecute("sudo systemctl unset-environment GOFIPS")
	v.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent")
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		service := v.Env().RemoteHost.MustExecute("sudo systemctl status datadog-agent")
		assert.Contains(t, service, "Active: active (running)")
	}, 60*time.Second, 5*time.Second)
}

func (v *LinuxFIPSComplianceSuite) TestFIPSNoFIPSProvider() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")
	v.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo sh -c "echo '%s' > /opt/datadog-agent/embedded/ssl/openssl.cnf"`, defaultOpenSSLConfig))

	status, err := v.Env().RemoteHost.Execute("sudo GOFIPS=0 datadog-agent status")
	assert.Nil(v.T(), err)
	assert.Contains(v.T(), status, "Status date")

	status, err = v.Env().RemoteHost.Execute("sudo GOFIPS=1 datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "can't enable FIPS mode for OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSEnabledNoOpenSSLConfig() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")

	status, err := v.Env().RemoteHost.Execute("sudo GOFIPS=0 datadog-agent status")
	assert.Nil(v.T(), err)
	assert.Contains(v.T(), status, "Status date")

	status, err = v.Env().RemoteHost.Execute("sudo GOFIPS=1 datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "can't enable FIPS mode for OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}
