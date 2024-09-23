// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"

	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type WindowsFIPSComplianceSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestWindowsFIPSComplianceSuite(t *testing.T) {
	e2e.Run(t, &WindowsFIPSComplianceSuite{},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		)))
}

func (v *WindowsFIPSComplianceSuite) TestFIPSHostEnabledAgentEnabled() {

	_, err := v.Env().RemoteHost.Execute(`Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy" -Name "Enabled" -Value 1 -Type DWord`)
	assert.Nil(v.T(), err)

	status, err := v.Env().RemoteHost.Execute(`$env:GOFIPS=1; & "$env:ProgramFiles\Datadog\Datadog Agent\bin\agent.exe" status; $env:GOFIPS=$null`)
	assert.Nil(v.T(), err)
	assert.Contains(v.T(), status, "Status date")

	_, err = v.Env().RemoteHost.Execute(`Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy" -Name "Enabled" -Value 0 -Type DWord`)
	assert.Nil(v.T(), err)
}

func (v *WindowsFIPSComplianceSuite) TestFIPSHostDisabledAgentEnabled() {
	_, err := v.Env().RemoteHost.Execute(`Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy" -Name "Enabled" -Value 0 -Type DWord`)
	assert.Nil(v.T(), err)

	status, _ := v.Env().RemoteHost.Execute(`$env:GOFIPS=1; & "$env:ProgramFiles\Datadog\Datadog Agent\bin\agent.exe" status; $env:GOFIPS=$null`)
	assert.Contains(v.T(), status, "cngcrypto: not in FIPS mode")
	assert.NotContains(v.T(), status, "Status date")
}
