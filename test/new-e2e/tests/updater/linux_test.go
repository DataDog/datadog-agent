// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

type vmUpdaterSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestUpdaterSuite(t *testing.T) {
	e2e.Run(t, &vmUpdaterSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithUpdater(),
		awshost.WithEC2InstanceOptions(ec2.WithOSArch(os.UbuntuDefault, os.ARM64Arch)),
	)))
}

func (v *vmUpdaterSuite) TestUpdaterOwnership() {
	// updater user exists and is a system user
	require.Equal(v.T(), "/usr/sbin/nologin\n", v.Env().RemoteHost.MustExecute(`getent passwd dd-updater | cut -d: -f7`), "unexpected: user does not exist or is not a system user")
	// owner
	require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(`stat -c "%U" /opt/datadog/updater`))
	// group
	require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(`stat -c "%G" /opt/datadog/updater`))
	// permissions
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" /opt/datadog/updater`))
}
