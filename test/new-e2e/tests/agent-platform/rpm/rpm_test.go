// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package rpm_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
)

var (
	osVersion    = flag.String("osversion", "", "os version to test")
	platform     = flag.String("platform", "", "platform to test")
	architecture = flag.String("arch", "", "architecture to test (x86_64, arm64)")
	majorVersion = flag.String("major-version", "7", "major version to test (6, 7)")
)

type rpmTestSuite struct {
	e2e.BaseSuite[environments.Host]

	osVersion float64
}

func TestRpmScript(t *testing.T) {
	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to unmarshal platform file: %v", err)

	osVersions := strings.Split(*osVersion, ",")

	t.Log("Parsed platform json file: ", platformJSON)

	for _, osVers := range osVersions {
		osVers := osVers
		if platformJSON[*platform][*architecture][osVers] == "" {
			// Fail if the image is not defined instead of silently running with default Ubuntu AMI
			t.Fatalf("No image found for %s %s %s", *platform, *architecture, osVers)
		}

		vmOpts := []ec2.VMOption{}
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
			vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
		}

		t.Run(fmt.Sprintf("test RPM package on %s %s", osVers, *architecture), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", osVers)
			slice := strings.Split(osVers, "-")
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

			osDesc := platforms.BuildOSDescriptor(*platform, *architecture, osVers)
			vmOpts = append(vmOpts, ec2.WithAMI(platformJSON[*platform][*architecture][osVers], osDesc, osDesc.Architecture))

			e2e.Run(tt,
				&rpmTestSuite{osVersion: version},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithEC2InstanceOptions(vmOpts...),
				)),
				e2e.WithStackName(fmt.Sprintf("rpm-test-%v-%v-%s-%v", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture, *majorVersion)),
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

	if *platform != "centos" {
		is.T().Skip("Skipping test on non-centos platform")
	}

	var arch string
	if *architecture == "arm64" {
		arch = "aarch64"
	} else {
		arch = *architecture
	}
	yumrepo := fmt.Sprintf("http://yumtesting.datad0g.com/testing/pipeline-%s-a%s/%s/%s/",
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

	is.T().Run("install the RPM package", func(t *testing.T) {
		VMclient.Host.MustExecute("sudo yum makecache -y")
		_, err := VMclient.Host.Execute("sudo yum install -y datadog-agent")

		if is.osVersion < 7 {
			require.Error(is.T(), err, "should fail to install on CentOS 6")
		} else {
			require.NoError(is.T(), err)
		}
	})
}
