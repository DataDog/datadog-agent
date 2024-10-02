// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	_ "embed"
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

// TODO: This is a temporary workaround to test the upgrade of the persistent integrations
// Since we need the previous agent version to have produced these files
// We will mock these files for now since this is the first release that will generate them
// Once we have a release that generates these files we can remove this workaround and stop mocking these files

//go:embed fixtures/diff_python_installed_packages.txt
var diffPythonInstalledPackages string

//go:embed fixtures/postinst_python_installed_packages.txt
var postinstPythonInstalledPackages string

//go:embed fixtures/requirements-agent-release.txt
var requirementsAgentRelease string

type persistentIntegrationsSuite struct {
	e2e.BaseSuite[environments.Host]
	pipelineID string
	apiKey     string
	flavor     componentos.Flavor
	arch       componentos.Architecture
}

var filesToMock = map[string]string{
	"/opt/datadog-agent/.diff_python_installed_packages.txt":     diffPythonInstalledPackages,
	"/opt/datadog-agent/.postinst_python_installed_packages.txt": postinstPythonInstalledPackages,
	"/opt/datadog-agent/requirements-agent-release.txt":          requirementsAgentRelease,
}

func TestPersistentIntegrationsSuite(t *testing.T) {
	pipelineID := os.Getenv("SRC_AGENT_PIPELINE_ID")
	apiKey := os.Getenv("DD_API_KEY")

	oses := []componentos.Descriptor{
		componentos.Ubuntu2204,
		componentos.Debian12,
		componentos.RedHat9,
	}

	archs := []componentos.Architecture{
		componentos.AMD64Arch,
		componentos.ARM64Arch,
	}

	for _, os := range oses {
		for _, arch := range archs {
			t.Logf("Running tests for OS: %s Arch: %s", os.Flavor, arch)

			t.Run(fmt.Sprintf("test upgrade persistent integrations on %s-%s", os.Flavor, arch), func(tt *testing.T) {
				flake.Mark(tt)
				tt.Parallel()
				tt.Logf("Testing %s-%s", os, arch)

				e2e.Run(tt,
					&persistentIntegrationsSuite{pipelineID: pipelineID, apiKey: apiKey, flavor: os.Flavor, arch: arch},
					e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOSArch(os, arch)))),
					e2e.WithStackName(fmt.Sprintf("upgrade-persistent-integrations-test-%s-%s", os.Flavor, arch)),
				)
			})
		}
	}
}

func (v *persistentIntegrationsSuite) TestIntegrationPersistsWithFileFlag() {
	host := v.Env().RemoteHost
	fileManager := filemanager.NewUnix(host)
	agentClient, err := client.NewHostAgentClient(v, host.HostOutput, false)
	require.NoError(v.T(), err)

	unixHelper := helpers.NewUnix()
	client := common.NewTestClient(v.Env().RemoteHost, agentClient, fileManager, unixHelper)

	var stdout string

	// Install the agent
	install.Unix(v.T(), client, installparams.WithMajorVersion("7"), installparams.WithFlavor("datadog-agent"), installparams.WithAPIKey(v.apiKey), installparams.WithArch(string(v.arch)), installparams.WithInstallPythonThirdPartyDeps(true))

	// Check Agent version
	agentVersion := v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent version")
	v.Env().RemoteHost.Execute("sudo runuser -u dd-agent -- datadog-agent version > /tmp/agent_version_initial")

	// Make sure that the integration is not installed
	stdout = v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	v.Assert().NotContains(stdout, "datadog-nvml")

	// Install a marketplace integration (NVML):
	v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration install -t datadog-nvml==1.0.0")
	v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- /opt/datadog-agent/embedded/bin/pip3 install grpcio pynvml")

	// Assert that the integration was installed successfully
	stdout = v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	v.Require().Contains(stdout, "datadog-nvml==1.0.0")

	// Prepare the mocked files
	for dest, data := range filesToMock {
		v.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo bash -c \"echo '%s' > %s\"", data, dest))
	}

	// Add flag file to persist the integrations
	v.Env().RemoteHost.MustExecute("sudo touch	/opt/datadog-agent/.install_python_third_party_deps")

	// Unset/Reset sticky bit on /tmp to allow the agent to write the error log
	defer v.Env().RemoteHost.MustExecute("sudo chmod +t /tmp")
	v.Env().RemoteHost.MustExecute("sudo chmod -t /tmp")

	// Upgrade the agent with the package from the pipeline:
	install.Unix(v.T(), client, installparams.WithPipelineID(v.pipelineID), installparams.WithAPIKey(v.apiKey), installparams.WithArch(string(v.arch)), installparams.WithFlavor("datadog-agent"), installparams.WithInstallPythonThirdPartyDeps(true))

	// Check New Agent version is different from the old one
	newAgentVersion := v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent version")
	v.Env().RemoteHost.Execute("sudo runuser -u dd-agent -- datadog-agent version > /tmp/agent_version_after_upgrade")
	v.Assert().NotEqual(agentVersion, newAgentVersion)

	// Assert that the integration is still installed
	stdout = v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	v.Assert().Contains(stdout, "datadog-nvml==1.0.0")
}
