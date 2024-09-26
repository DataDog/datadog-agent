// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type persistentIntegrationsSuite struct {
	e2e.BaseSuite[environments.Host]
	pipelineID string
	apiKey     string
}

func TestPersistentIntegrationsSuite(t *testing.T) {
	pipelineID := os.Getenv("DEST_AGENT_PIPELINE_ID")
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
				// TODO: mark the test as flaky with flake.Mark(tt)?
				tt.Parallel()
				tt.Logf("Testing %s-%s", os, arch)

				e2e.Run(tt,
					&persistentIntegrationsSuite{pipelineID: pipelineID, apiKey: apiKey},
					e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOSArch(os, arch)))),
					e2e.WithStackName(fmt.Sprintf("upgrade-persistent-integrations-test-%s-%s", os.Flavor, arch)),
				)
			})
		}
	}
}

func (v *persistentIntegrationsSuite) TestNVMLIntegrationPersists() {
	var stdout string

	// Install a datadog-agent release:
	installAgentCmd := fmt.Sprintf("DD_API_KEY=%s DD_AGENT_MAJOR_VERSION=7 DD_AGENT_MINOR_VERSION=50 bash -c \"$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script.sh)\"", v.apiKey)
	stdout = v.Env().RemoteHost.MustExecute(installAgentCmd)

	stdout = v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent version")
	v.Require().Contains(stdout, "7.50")

	// Install a marketplace integration (NVML):
	v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration install -t datadog-nvml==1.0.0")
	v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- /opt/datadog-agent/embedded/bin/pip3 install grpcio pynvml")

	// Assert that the integration was installed successfully
	stdout = v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration show datadog-nvml")
	v.Require().Contains(stdout, "Installed version: 1.0.0")

	// Install your package from your pipeline:
	stdout = v.Env().RemoteHost.MustExecute(fmt.Sprintf("TESTING_APT_URL=apttesting.datad0g.com TESTING_APT_REPO_VERSION=\"pipeline-%s-a7-arm64 7\" DD_API_KEY=%s DD_SITE=\"datadoghq.com\" bash -c \"$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script.sh)\"", v.pipelineID, v.apiKey))

	stdout = v.Env().RemoteHost.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration show datadog-nvml")
	v.Assert().Contains(stdout, "Installed version: 1.0.0")
}
