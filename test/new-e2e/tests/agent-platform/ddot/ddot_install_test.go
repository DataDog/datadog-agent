// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ddot

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

var (
	osVersion    = flag.String("osversion", "", "os version to test")
	platform     = flag.String("platform", "", "platform to test")
	architecture = flag.String("arch", "", "architecture to test (x86_64, arm64))")
)

type ddotInstallSuite struct {
	e2e.BaseSuite[environments.Host]
	osVersion float64
}

func ExecuteWithoutError(_ *testing.T, client *common.TestClient, cmd string, args ...any) {
	var finalCmd string
	if len(args) > 0 {
		finalCmd = fmt.Sprintf(cmd, args...)
	} else {
		finalCmd = cmd
	}
	client.Host.MustExecute(finalCmd)
}

func TestDDOTInstallScript(t *testing.T) {
	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

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

		t.Run(fmt.Sprintf("test ddot install on %s %s", osVers, *architecture), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", osVers)
			slice := strings.Split(osVers, "-")
			var version float64
			if len(slice) == 2 {
				version, err = strconv.ParseFloat(slice[1], 64)
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
				&ddotInstallSuite{osVersion: version},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithEC2InstanceOptions(vmOpts...),
				)),
				e2e.WithStackName(fmt.Sprintf("ddot-install-test-%v-%s", osVers, *architecture)),
			)
		})
	}
}

func (is *ddotInstallSuite) TestDDOTInstall() {
	fileManager := filemanager.NewUnix(is.Env().RemoteHost)
	unixHelper := helpers.NewUnix()
	agentClient, err := client.NewHostAgentClient(is, is.Env().RemoteHost.HostOutput, false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	if *platform == "debian" || *platform == "ubuntu" {
		is.ddotDebianTest(VMclient)
	} else if *platform == "centos" || *platform == "amazonlinux" || *platform == "fedora" || *platform == "redhat" {
		is.ddotRhelTest(VMclient)
	} else {
		require.Equal(is.T(), *platform, "suse", "NonSupportedPlatformError : %s isn't supported !", *platform)
		is.ddotSuseTest(VMclient)
	}
	is.CheckDDOTInstallation(VMclient)
}

func (is *ddotInstallSuite) CheckDDOTInstallation(VMclient *common.TestClient) {
	is.T().Run("check otel-agent binary exists", func(tt *testing.T) {
		_, err := VMclient.FileManager.FileExists("/opt/datadog-agent/embedded/bin/otel-agent")
		require.NoError(tt, err, "otel-agent binary should be present")
	})

	is.T().Run("check otel-agent example config exists", func(tt *testing.T) {
		_, err := VMclient.FileManager.FileExists("/etc/datadog-agent/otel-config.yaml.example")
		require.NoError(tt, err, "otel-agent example config file should be present")
	})
}

func (is *ddotInstallSuite) ddotDebianTest(VMclient *common.TestClient) {
	aptTrustedDKeyring := "/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg"
	aptUsrShareKeyring := "/usr/share/keyrings/datadog-archive-keyring.gpg"
	aptrepo := fmt.Sprintf("[signed-by=/usr/share/keyrings/datadog-archive-keyring.gpg] http://s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%s-a7", os.Getenv("E2E_PIPELINE_ID"))
	aptrepoDist := fmt.Sprintf("stable-%s", *architecture)
	fileManager := VMclient.FileManager
	var err error

	is.T().Run("create /usr/share keyring and source list", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg")
		tmpFileContent := fmt.Sprintf("deb %s %s 7", aptrepo, aptrepoDist)
		_, err = fileManager.WriteFile("/etc/apt/sources.list.d/datadog.list", []byte(tmpFileContent))
		require.NoError(t, err)
		ExecuteWithoutError(t, VMclient, "sudo touch %s && sudo chmod a+r %s", aptUsrShareKeyring, aptUsrShareKeyring)
		keys := []string{"DATADOG_APT_KEY_CURRENT.public", "DATADOG_APT_KEY_C0962C7D.public", "DATADOG_APT_KEY_F14F620E.public", "DATADOG_APT_KEY_382E94DE.public"}
		for _, key := range keys {
			ExecuteWithoutError(t, VMclient, "sudo curl --retry 5 -o \"/tmp/%s\" \"https://keys.datadoghq.com/%s\"", key, key)
			ExecuteWithoutError(t, VMclient, "sudo cat \"/tmp/%s\" | sudo gpg --import --batch --no-default-keyring --keyring \"%s\"", key, aptUsrShareKeyring)
		}
	})
	if (*platform == "ubuntu" && is.osVersion < 15) || (*platform == "debian" && is.osVersion < 9) {
		is.T().Run("create /etc/apt keyring", func(t *testing.T) {
			ExecuteWithoutError(t, VMclient, "sudo cp %s %s", aptUsrShareKeyring, aptTrustedDKeyring)
		})
	}

	is.T().Run("install ddot", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo apt-get update")
		ExecuteWithoutError(t, VMclient, "sudo apt-get install datadog-agent-ddot -y -q")
	})

	is.T().Run("check datadog-agent is also installed and has the same version as ddot", func(t *testing.T) {
		// Check both packages are installed
		ExecuteWithoutError(t, VMclient, "dpkg-query -l datadog-agent")
		ExecuteWithoutError(t, VMclient, "dpkg-query -l datadog-agent-ddot")

		// Get versions of both packages and compare them
		agentVersion, err := VMclient.Host.Execute("dpkg-query --showformat='${Version}' --show datadog-agent")
		require.NoError(t, err, "should be able to get datadog-agent version")

		ddotVersion, err := VMclient.Host.Execute("dpkg-query --showformat='${Version}' --show datadog-agent-ddot")
		require.NoError(t, err, "should be able to get datadog-agent-ddot version")

		require.Equal(t, strings.TrimSpace(agentVersion), strings.TrimSpace(ddotVersion),
			"datadog-agent and datadog-agent-ddot should have the same version")
	})
}

func (is *ddotInstallSuite) ddotRhelTest(VMclient *common.TestClient) {
	var arch string
	if *architecture == "arm64" {
		arch = "aarch64"
	} else {
		arch = *architecture
	}
	yumrepo := fmt.Sprintf("http://s3.amazonaws.com/yumtesting.datad0g.com/testing/pipeline-%s-a7/%s/%s/",
		os.Getenv("E2E_PIPELINE_ID"), "7", arch)
	fileManager := VMclient.FileManager
	var err error

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
		"gpgkey=https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public\n"+
		"\thttps://keys.datadoghq.com/DATADOG_RPM_KEY_B01082D3.public\n"+
		"\thttps://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"\thttps://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public",
		yumrepo, repogpgcheck)
	_, err = fileManager.WriteFile("/etc/yum.repos.d/datadog.repo", []byte(fileContent))
	require.NoError(is.T(), err)

	is.T().Run("install ddot", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo yum makecache -y")
		ExecuteWithoutError(t, VMclient, "sudo yum install -y datadog-agent-ddot")
	})

	is.T().Run("check datadog-agent is also installed and has the same version as ddot", func(t *testing.T) {
		// Check both packages are installed
		ExecuteWithoutError(t, VMclient, "rpm -q datadog-agent")
		ExecuteWithoutError(t, VMclient, "rpm -q datadog-agent-ddot")

		// Get versions of both packages and compare them
		agentVersion, err := VMclient.Host.Execute("rpm -q --queryformat='%{VERSION}-%{RELEASE}' datadog-agent")
		require.NoError(t, err, "should be able to get datadog-agent version")

		ddotVersion, err := VMclient.Host.Execute("rpm -q --queryformat='%{VERSION}-%{RELEASE}' datadog-agent-ddot")
		require.NoError(t, err, "should be able to get datadog-agent-ddot version")

		require.Equal(t, strings.TrimSpace(agentVersion), strings.TrimSpace(ddotVersion),
			"datadog-agent and datadog-agent-ddot should have the same version")
	})
}

func (is *ddotInstallSuite) ddotSuseTest(VMclient *common.TestClient) {
	var arch string
	if *architecture == "arm64" {
		arch = "aarch64"
	} else {
		arch = *architecture
	}

	suseRepo := fmt.Sprintf("http://s3.amazonaws.com/yumtesting.datad0g.com/suse/testing/pipeline-%s-a7/%s/%s/",
		os.Getenv("E2E_PIPELINE_ID"), "7", arch)
	fileManager := VMclient.FileManager
	var err error

	// Disable all existing non-datadog repos to avoid issues during refresh (which is hard to prevent zypper from doing spontaneously);
	// we don't need them to install the Agent anyway.
	ExecuteWithoutError(nil, VMclient, "sudo rm -f /etc/zypp/repos.d/*.repo")

	fileContent := fmt.Sprintf("[datadog]\n"+
		"name = Datadog, Inc.\n"+
		"baseurl = %s\n"+
		"enabled=1\n"+
		"gpgcheck=1\n"+
		"repo_gpgcheck=1\n"+
		"gpgkey=https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public\n"+
		"	    https://keys.datadoghq.com/DATADOG_RPM_KEY_B01082D3.public\n"+
		"	    https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"	    https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public\n",
		suseRepo)
	_, err = fileManager.WriteFile("/etc/zypp/repos.d/datadog.repo", []byte(fileContent))
	require.NoError(is.T(), err)

	ExecuteWithoutError(nil, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_CURRENT.public https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public")
	ExecuteWithoutError(nil, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_CURRENT.public")
	ExecuteWithoutError(nil, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_B01082D3.public https://keys.datadoghq.com/DATADOG_RPM_KEY_B01082D3.public")
	ExecuteWithoutError(nil, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_B01082D3.public")
	ExecuteWithoutError(nil, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_FD4BF915.public https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public")
	ExecuteWithoutError(nil, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_FD4BF915.public")
	ExecuteWithoutError(nil, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_E09422B3.public https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public")
	ExecuteWithoutError(nil, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_E09422B3.public")

	is.T().Run("install ddot", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo zypper --non-interactive --no-gpg-checks refresh datadog")
		ExecuteWithoutError(t, VMclient, "sudo zypper --non-interactive --no-refresh install datadog-agent-ddot")
	})

	is.T().Run("check datadog-agent is also installed and has the same version as ddot", func(t *testing.T) {
		// Check both packages are installed
		ExecuteWithoutError(t, VMclient, "rpm -q datadog-agent")
		ExecuteWithoutError(t, VMclient, "rpm -q datadog-agent-ddot")

		// Get versions of both packages and compare them
		agentVersion, err := VMclient.Host.Execute("rpm -q --queryformat='%{VERSION}-%{RELEASE}' datadog-agent")
		require.NoError(t, err, "should be able to get datadog-agent version")

		ddotVersion, err := VMclient.Host.Execute("rpm -q --queryformat='%{VERSION}-%{RELEASE}' datadog-agent-ddot")
		require.NoError(t, err, "should be able to get datadog-agent-ddot version")

		require.Equal(t, strings.TrimSpace(agentVersion), strings.TrimSpace(ddotVersion),
			"datadog-agent and datadog-agent-ddot should have the same version")
	})
}
