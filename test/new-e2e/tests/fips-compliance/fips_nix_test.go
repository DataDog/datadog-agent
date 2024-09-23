// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"
	"fmt"

	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/openssl-default.cnf
var defaultOpenSSLConfig string

type LinuxFIPSComplianceSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxFIPSComplianceSuite(t *testing.T) {
	e2e.Run(t, &LinuxFIPSComplianceSuite{},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault)),
		)))
}

func (v *LinuxFIPSComplianceSuite) TestFIPSDefaultConfig() {
	status := v.Env().RemoteHost.MustExecute("sudo GOFIPS=0 datadog-agent status")
	assert.NotContains(v.T(), status, "can't enable FIPS mode for OpenSSL")
	assert.Contains(v.T(), status, "Status date")

	status = v.Env().RemoteHost.MustExecute("sudo GOFIPS=1 datadog-agent status")
	assert.NotContains(v.T(), status, "can't enable FIPS mode for OpenSSL")
	assert.Contains(v.T(), status, "Status date")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSNoFIPSProvider() {
	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp")
	output := v.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo sh -c "echo '%s' > /opt/datadog-agent/embedded/ssl/openssl.cnf"`, defaultOpenSSLConfig))
	assert.Empty(v.T(), output)

	status, err := v.Env().RemoteHost.Execute("sudo GOFIPS=0 datadog-agent status")
	assert.Nil(v.T(), err)
	assert.Contains(v.T(), status, "Status date")

	status, err = v.Env().RemoteHost.Execute("sudo GOFIPS=1 datadog-agent status")
	require.NotNil(v.T(), err)
	assert.Contains(v.T(), err.Error(), "can't enable FIPS mode for OpenSSL")
	assert.NotContains(v.T(), status, "Status date")

	v.Env().RemoteHost.MustExecute("sudo mv /opt/datadog-agent/embedded/ssl/openssl.cnf.tmp /opt/datadog-agent/embedded/ssl/openssl.cnf")
}

func (v *LinuxFIPSComplianceSuite) TestFIPSEnabledNoConfig() {
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
