// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package rpm_test

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
)

var (
	osDescriptors = flag.String("osdescriptors", "", "platform/arch/os version (debian/x86_64/11)")
	majorVersion  = flag.String("major-version", "7", "major version to test (6, 7)")
)

type rpmTestSuite struct {
	e2e.BaseSuite[environments.Host]

	osDesc    e2eos.Descriptor
	osVersion float64
}

func TestRpmScript(t *testing.T) {
	osDescriptors, err := platforms.ParseOSDescriptors(*osDescriptors)
	if err != nil {
		t.Fatalf("failed to parse os descriptors: %v", err)
	}
	if len(osDescriptors) == 0 {
		t.Fatal("expecting some value to be passed for --osdescriptors on test invocation, got none")
	}

	for _, osDesc := range osDescriptors {
		osDesc := osDesc

		vmOpts := []ec2.VMOption{}
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
			vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
		}

		t.Run("test RPM package on "+platforms.PrettifyOsDescriptor(osDesc), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", platforms.PrettifyOsDescriptor(osDesc))
			slice := strings.Split(osDesc.Version, "-")
			var version float64
			if len(slice) == 2 {
				version, err = strconv.ParseFloat(slice[1], 64)
				if version == 610 {
					version = 6.10
				}
				require.NoError(tt, err)
			} else if len(slice) == 3 {
				version, err = strconv.ParseFloat(slice[1]+"."+slice[2], 64)
				require.NoError(tt, err)
			} else {
				version = 0
			}

			vmOpts = append(vmOpts, ec2.WithOS(osDesc))

			e2e.Run(tt,
				&rpmTestSuite{osVersion: version},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithRunOptions(ec2.WithEC2InstanceOptions(vmOpts...)),
				)),
				e2e.WithStackName(fmt.Sprintf("rpm-test-%s-%v", platforms.PrettifyOsDescriptor(osDesc), *majorVersion)),
			)
		})
	}
}

func (is *rpmTestSuite) TestRpm() {
	filemanager := filemanager.NewUnix(is.Env().RemoteHost)
	unixHelper := helpers.NewUnix()
	agentClient, err := client.NewHostAgentClient(is, is.Env().RemoteHost.HostOutput, false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().RemoteHost, agentClient, filemanager, unixHelper)

	if is.osDesc.Flavor != e2eos.CentOS {
		is.T().Skip("Skipping test on non-centos platform")
	}

	var arch string
	if is.osDesc.Architecture == e2eos.ARM64Arch {
		arch = "aarch64"
	} else {
		arch = "x86_64"
	}
	yumrepo := fmt.Sprintf("http://s3.amazonaws.com/yumtesting.datad0g.com/testing/pipeline-%s-a%s/%s/%s/",
		os.Getenv("E2E_PIPELINE_ID"), *majorVersion, *majorVersion, arch)
	fileManager := VMclient.FileManager

	protocol := "https"
	if is.osVersion < 6 {
		protocol = "http"
	}
	repogpgcheck := "1"
	if is.osVersion < 8.2 {
		repogpgcheck = "0"
	}

	fileContent := fmt.Sprintf("[datadog]\n"+
		"name = Datadog, Inc.\n"+
		"baseurl = %s\n"+
		"enabled=1\n"+
		"gpgcheck=1\n"+
		"repo_gpgcheck=%s\n"+
		"gpgkey=%s://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public\n"+
		"\t%s://keys.datadoghq.com/DATADOG_RPM_KEY_B01082D3.public\n"+
		"\t%s://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"\t%s://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public",
		yumrepo, repogpgcheck, protocol, protocol, protocol, protocol)
	_, err = fileManager.WriteFile("/etc/yum.repos.d/datadog.repo", []byte(fileContent))
	require.NoError(is.T(), err)

	is.T().Run("install the RPM package", func(*testing.T) {
		VMclient.Host.MustExecute("sudo yum makecache -y")
		_, err := VMclient.Host.Execute("sudo yum install -y datadog-agent")

		if is.osVersion < 7 {
			require.Error(is.T(), err, "should fail to install on CentOS 6")
		} else {
			require.NoError(is.T(), err)
		}
	})
}
