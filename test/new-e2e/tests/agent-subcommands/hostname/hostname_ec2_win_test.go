// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type windowsHostnameSuite struct {
	baseHostnameSuite
}

func TestWindowsHostnameEC2Suite(t *testing.T) {
	osOption := awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))
	e2e.Run(t, &windowsHostnameSuite{baseHostnameSuite: baseHostnameSuite{osOption: osOption}}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(osOption)))
}

func (v *windowsHostnameSuite) TestAgentConfigHostnameFileOverride() {
	// Using MustExecute instead of agentparams.WithFile because encoding (utf16 and utf8) contains invisible characters
	// that makes the hostname not RFC1123 compliant and then rejected by the Agent.
	v.Env().RemoteHost.MustExecute(`"hostname.from.file" | Out-File -FilePath "C:/ProgramData/Datadog/hostname.txt" -Encoding ascii`)
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig("hostname_file: C:/ProgramData/Datadog/hostname.txt"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), "hostname.from.file", hostname)
}

func (v *windowsHostnameSuite) TestAgentConfigPreferImdsv2() {
	config := `ec2_prefer_imdsv2: true
ec2_use_windows_prefix_detection: true`

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig(config))))
	// e2e metadata provider already uses IMDSv2
	metadata := client.NewEC2Metadata(v.Env().RemoteHost)

	hostname := v.Env().Agent.Client.Hostname()
	resourceID := metadata.Get("instance-id")
	assert.Equal(v.T(), resourceID, hostname)
}
