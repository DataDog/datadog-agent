// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	perms "github.com/DataDog/test-infra-definitions/components/datadog/agentparams/filepermissions"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type filePermissionsWindowsTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

// updateEnvWithOption updates the environment with a new provisioner option
func (v *filePermissionsWindowsTestSuite) updateEnvWithWindows(opt awshost.ProvisionerOption) {
	var windowsOpt = []awshost.ProvisionerOption{awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))}
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(append(windowsOpt, opt)...))
}

func TestFilePermissionsWindows(t *testing.T) {
	e2e.Run(t, &filePermissionsWindowsTestSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))), e2e.WithDevMode())
}

func (v *filePermissionsWindowsTestSuite) TestDefaultPermissions() {
	// Use different folder because the framework does not handle deletion of multiple files in the same folder
	files := []agentparams.Option{
		agentparams.WithFile(`C:/TestFolder/default`, "default perms", true),
		agentparams.WithFileWithPermissions(`C:/TestFolder2/default`, "default perms as well", true, perms.NewWindowsPermissions()),
	}

	v.updateEnvWithWindows(awshost.WithAgentOptions(files...))

	perm := v.Env().RemoteHost.MustExecute("icacls C:/TestFolder/default")
	assert.Contains(v.T(), perm, `NT AUTHORITY\SYSTEM:(I)(F)`)
	assert.Contains(v.T(), perm, `BUILTIN\Administrators:(I)(F)`)
	assert.Contains(v.T(), perm, `BUILTIN\Users:(I)(RX)`)

	perm = v.Env().RemoteHost.MustExecute("icacls C:/TestFolder2/default")
	assert.Contains(v.T(), perm, `NT AUTHORITY\SYSTEM:(I)(F)`)
	assert.Contains(v.T(), perm, `BUILTIN\Administrators:(I)(F)`)
	assert.Contains(v.T(), perm, `BUILTIN\Users:(I)(RX)`)
}

func (v *filePermissionsWindowsTestSuite) TestIcaclsCommand() {
	cmd := `/grant "ddagentuser:(D,WDAC,RX,RA)" /deny "Administrator:(W)" /deny "Administrators:(R)"`

	files := []agentparams.Option{
		agentparams.WithFileWithPermissions(`C:/TestFolder/icacls_cmd`, "", true, perms.NewWindowsPermissions(perms.WithIcaclsCommand(cmd))),
	}

	v.updateEnvWithWindows(awshost.WithAgentOptions(files...))

	perm := v.Env().RemoteHost.MustExecute("icacls C:/TestFolder/icacls_cmd")
	assert.Contains(v.T(), perm, `BUILTIN\Administrators:(DENY)(R)`)
	assert.Contains(v.T(), perm, `Administrator:(DENY)(W)`)
	assert.Contains(v.T(), perm, "ddagentuser:(RX,D,WDAC)")
}

func (v *filePermissionsWindowsTestSuite) TestRemoveDefaultPermissions() {
	cmd := `/grant "ddagentuser:(RX,W)"`

	files := []agentparams.Option{
		agentparams.WithFileWithPermissions(`C:/TestFolder/remove`, "", true, perms.NewWindowsPermissions(perms.WithDisableInheritance())),
		agentparams.WithFileWithPermissions(`C:/TestFolder2/remove_and_grant`, "", true, perms.NewWindowsPermissions(perms.WithIcaclsCommand(cmd), perms.WithDisableInheritance())),
	}

	v.updateEnvWithWindows(awshost.WithAgentOptions(files...))

	perm := v.Env().RemoteHost.MustExecute("icacls C:/TestFolder/remove")
	assert.Equal(v.T(), "C:/TestFolder/remove \nSuccessfully processed 1 files; Failed processing 0 files\r\n", perm)

	perm = v.Env().RemoteHost.MustExecute("icacls C:/TestFolder2/remove_and_grant")
	assert.Contains(v.T(), perm, "ddagentuser:(RX,W)")
	assert.NotContains(v.T(), perm, `NT AUTHORITY\SYSTEM`)
	assert.NotContains(v.T(), perm, `BUILTIN\Administrators`)
	assert.NotContains(v.T(), perm, `BUILTIN\Users`)
}

func (v *filePermissionsWindowsTestSuite) TestSecretsPermissions() {
	cmd := `/grant "ddagentuser:(RX)" "Administrators:(RX)"`

	files := []agentparams.Option{
		agentparams.WithFileWithPermissions(`C:/TestFolder2/secrets`, "", true, perms.NewWindowsPermissions(perms.WithIcaclsCommand(cmd), perms.WithDisableInheritance())),
	}

	v.updateEnvWithWindows(awshost.WithAgentOptions(files...))

	perm := v.Env().RemoteHost.MustExecute("icacls C:/TestFolder2/secrets")
	assert.Regexp(v.T(), regexp.MustCompile(`C:/TestFolder2/secrets BUILTIN\\Administrators:\(RX\)\n[\s]+[[:alnum:]-]+\\ddagentuser:\(RX\)\n\nSuccessfully`), perm)
}
