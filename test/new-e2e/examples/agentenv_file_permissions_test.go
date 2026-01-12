// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

type filePermissionsTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestFilePermissions(t *testing.T) {
	e2e.Run(t, &filePermissionsTestSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *filePermissionsTestSuite) TestFilePermissions() {
	files := []agentparams.Option{
		agentparams.WithFile("/tmp/default_perms", "default perms", true),
		agentparams.WithFileWithPermissions(`/tmp/default_perms2`, "default perms as well", true, perms.NewUnixPermissions()),
		agentparams.WithFileWithPermissions(`/tmp/six_four_zero`, "Perms are 640", true, perms.NewUnixPermissions(perms.WithPermissions("0640"))),
		agentparams.WithFileWithPermissions(`/tmp/seven_seven_seven`, "Perms are 777", true, perms.NewUnixPermissions(perms.WithPermissions("0777"))),
	}

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(ec2.WithAgentOptions(files...))))

	perm := v.Env().RemoteHost.MustExecute("ls -la /tmp/default_perms")
	assert.Contains(v.T(), perm, "-rw-r--r--")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/default_perms2")
	assert.Contains(v.T(), perm, "-rw-r--r--")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/six_four_zero")
	assert.Contains(v.T(), perm, "-rw-r-----")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/seven_seven_seven")
	assert.Contains(v.T(), perm, "-rwxrwxrwx")
}

func (v *filePermissionsTestSuite) TestUserGroupPermissions() {
	files := []agentparams.Option{
		agentparams.WithFile("/tmp/default_usergroup_root", "default user/group using root", true),
		agentparams.WithFileWithPermissions(`/tmp/default_usergroup_root2`, "default user/group using root 2", true, perms.NewUnixPermissions()),
		agentparams.WithFile("/tmp/default_usergroup_no_root", "default user/group not using root ", false),
		agentparams.WithFileWithPermissions(`/tmp/default_usergroup_no_root2`, "default usear/group not using root 2", false, perms.NewUnixPermissions()),
		agentparams.WithFileWithPermissions(`/tmp/dd-agent_user`, "dd-agent + ubuntu", false, perms.NewUnixPermissions(perms.WithOwner("dd-agent"))),
		agentparams.WithFileWithPermissions(`/tmp/dd-agent_group`, "ubuntu + dd-agent", false, perms.NewUnixPermissions(perms.WithGroup("dd-agent"))),
		agentparams.WithFileWithPermissions(`/tmp/dd-agent_user_and_groups`, "root + dd-agent", false, perms.NewUnixPermissions(perms.WithGroup("dd-agent"), perms.WithOwner("root"))),
		agentparams.WithFileWithPermissions(`/tmp/own_by_root_plus_permissions`, "root:root 750", false, perms.NewUnixPermissions(perms.WithPermissions("0750"), perms.WithGroup("root"), perms.WithOwner("root"))),
	}

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(ec2.WithAgentOptions(files...))))

	perm := v.Env().RemoteHost.MustExecute("ls -la /tmp/default_usergroup_root")
	assert.Contains(v.T(), perm, "root root")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/default_usergroup_root2")
	assert.Contains(v.T(), perm, "root root")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/default_usergroup_no_root")
	assert.Contains(v.T(), perm, "ubuntu ubuntu")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/default_usergroup_no_root2")
	assert.Contains(v.T(), perm, "ubuntu ubuntu")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/dd-agent_user")
	assert.Contains(v.T(), perm, "dd-agent ubuntu")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/dd-agent_group")
	assert.Contains(v.T(), perm, "ubuntu dd-agent")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/dd-agent_user_and_groups")
	assert.Contains(v.T(), perm, "root dd-agent")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/own_by_root_plus_permissions")
	assert.Contains(v.T(), perm, "-rwxr-x--- 1 root root")
}

func (v *filePermissionsTestSuite) TestSecretsPermissions() {
	files := []agentparams.Option{
		secretsutils.WithUnixSetupScript("/tmp/secrets", false),
		secretsutils.WithUnixSetupScript("/tmp/secrets_root_group", true),
	}

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(ec2.WithAgentOptions(files...))))

	perm := v.Env().RemoteHost.MustExecute("ls -la /tmp/secrets")
	assert.Contains(v.T(), perm, "-rwx------ 1 dd-agent dd-agent")

	perm = v.Env().RemoteHost.MustExecute("ls -la /tmp/secrets_root_group")
	assert.Contains(v.T(), perm, "-rwxr-x--- 1 dd-agent root")
}
