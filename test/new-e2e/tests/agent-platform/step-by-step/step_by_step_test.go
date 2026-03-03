// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package stepbystep

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

var (
	osDescriptors             = flag.String("osdescriptors", "", "platform/arch/os version (debian/x86_64/11)")
	cwsSupportedOsDescriptors = flag.String("cws-supported-osdescriptors", "", "list of os where CWS is supported")
	flavorName                = flag.String("flavor", "datadog-agent", "package flavor to install")
	majorVersion              = flag.String("major-version", "7", "major version to test (6, 7)")
)

type stepByStepSuite struct {
	e2e.BaseSuite[environments.Host]

	osVersion    float64
	osDesc       e2eos.Descriptor
	cwsSupported bool
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

func TestStepByStepScript(t *testing.T) {
	osDescriptors, err := platforms.ParseOSDescriptors(*osDescriptors)
	if err != nil {
		t.Fatalf("failed to parse os descriptors: %v", err)
	}
	if len(osDescriptors) == 0 {
		t.Fatal("expecting some value to be passed for --osdescriptors on test invocation, got none")
	}
	cwsSupportedOsDescriptorsList, err := platforms.ParseOSDescriptors(*cwsSupportedOsDescriptors)
	if err != nil {
		t.Fatalf("failed to parse cws supported os descriptors: %v", err)
	}
	if len(cwsSupportedOsDescriptorsList) == 0 {
		t.Fatal("expecting some value to be passed for --cws-supported-osdescriptors on test invocation, got none")
	}

	for _, osDesc := range osDescriptors {
		osDesc := osDesc

		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsDescriptorsList {
			if cwsSupportedOs == osDesc {
				cwsSupported = true
			}
		}

		vmOpts := []ec2.VMOption{}
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
			vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
		}

		t.Run("test step by step on "+platforms.PrettifyOsDescriptor(osDesc), func(tt *testing.T) {
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
				&stepByStepSuite{cwsSupported: cwsSupported, osVersion: version, osDesc: osDesc},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithRunOptions(ec2.WithEC2InstanceOptions(vmOpts...)),
				)),
				e2e.WithStackName(fmt.Sprintf("step-by-step-test-%s-%s", platforms.PrettifyOsDescriptor(osDesc), *majorVersion)),
			)
		})
	}
}

func (is *stepByStepSuite) TestStepByStep() {
	fileManager := filemanager.NewUnix(is.Env().RemoteHost)
	unixHelper := helpers.NewUnix()
	agentClient, err := client.NewHostAgentClient(is, is.Env().RemoteHost.HostOutput, false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	if is.osDesc.Flavor == e2eos.Debian || is.osDesc.Flavor == e2eos.Ubuntu {
		is.StepByStepDebianTest(VMclient)
	} else if is.osDesc.Flavor == e2eos.CentOS || is.osDesc.Flavor == e2eos.AmazonLinux || is.osDesc.Flavor == e2eos.Fedora || is.osDesc.Flavor == e2eos.RedHat {
		is.StepByStepRhelTest(VMclient)
	} else {
		require.Equal(is.T(), is.osDesc.Flavor, e2eos.Suse, "NonSupportedPlatformError : %s isn't supported !", is.osDesc.Flavor)
		is.StepByStepSuseTest(VMclient)
	}
	is.ConfigureAndRunAgentService(VMclient)
	is.CheckStepByStepAgentInstallation(VMclient)
}

func (is *stepByStepSuite) ConfigureAndRunAgentService(VMclient *common.TestClient) {
	is.T().Run("add config file", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo sh -c \"sed 's/api_key:.*/api_key: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX/' /etc/datadog-agent/datadog.yaml.example > /etc/datadog-agent/datadog.yaml\"")
		ExecuteWithoutError(t, VMclient, "sudo sh -c \"chown dd-agent:dd-agent /etc/datadog-agent/datadog.yaml && chmod 640 /etc/datadog-agent/datadog.yaml\"")
		if (is.osDesc.Flavor == e2eos.Ubuntu && is.osVersion == 14.04) || (is.osDesc.Flavor == e2eos.CentOS && is.osVersion == 6.10) {
			ExecuteWithoutError(t, VMclient, "sudo initctl start datadog-agent")
		} else {
			ExecuteWithoutError(t, VMclient, "sudo systemctl restart datadog-agent.service")
		}
	})
}

func (is *stepByStepSuite) CheckStepByStepAgentInstallation(VMclient *common.TestClient) {
	common.CheckInstallation(is.T(), VMclient)
	common.CheckAgentBehaviour(is.T(), VMclient)
	common.CheckAgentStops(is.T(), VMclient)
	common.CheckAgentRestarts(is.T(), VMclient)
	common.CheckIntegrationInstall(is.T(), VMclient)
	common.SetAgentPythonMajorVersion(is.T(), VMclient, "3")
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckAgentPython(is.T(), VMclient, common.ExpectedPythonVersion3)
	if *majorVersion == "6" {
		common.SetAgentPythonMajorVersion(is.T(), VMclient, "2")
		common.CheckAgentPython(is.T(), VMclient, common.ExpectedPythonVersion2)
	}
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckApmEnabled(is.T(), VMclient)
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckApmDisabled(is.T(), VMclient)
	if *flavorName == "datadog-agent" {
		common.CheckSystemProbeBehavior(is.T(), VMclient)
		if is.cwsSupported {
			common.CheckCWSBehaviour(is.T(), VMclient)
		}
	}
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckADPEnabled(is.T(), VMclient)
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckADPDisabled(is.T(), VMclient)

	is.T().Run("remove the agent", func(tt *testing.T) {
		_, err := VMclient.PkgManager.Remove(*flavorName)
		require.NoError(tt, err, "should uninstall the agent")
	})

	common.CheckUninstallation(is.T(), VMclient)
}

func (is *stepByStepSuite) StepByStepDebianTest(VMclient *common.TestClient) {
	aptTrustedDKeyring := "/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg"
	aptUsrShareKeyring := "/usr/share/keyrings/datadog-archive-keyring.gpg"
	aptrepo := fmt.Sprintf("[signed-by=/usr/share/keyrings/datadog-archive-keyring.gpg] http://s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%s-a%s", os.Getenv("E2E_PIPELINE_ID"), *majorVersion)
	aptrepoDist := fmt.Sprintf("stable-%s %s", is.osDesc.Architecture, *majorVersion)
	fileManager := VMclient.FileManager
	var err error

	is.T().Run("create /usr/share keyring and source list", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg")
		tmpFileContent := fmt.Sprintf("deb %s %s %s", aptrepo, aptrepoDist, *majorVersion)
		_, err = fileManager.WriteFile("/etc/apt/sources.list.d/datadog.list", []byte(tmpFileContent))
		require.NoError(t, err)
		ExecuteWithoutError(t, VMclient, "sudo touch %s && sudo chmod a+r %s", aptUsrShareKeyring, aptUsrShareKeyring)
		keys := []string{"DATADOG_APT_KEY_CURRENT.public", "DATADOG_APT_KEY_C0962C7D.public", "DATADOG_APT_KEY_F14F620E.public", "DATADOG_APT_KEY_382E94DE.public"}
		for _, key := range keys {
			ExecuteWithoutError(t, VMclient, "sudo curl --retry 5 -o \"/tmp/%s\" \"https://apttesting.datad0g.com/test-keys/%s\"", key, key)
			ExecuteWithoutError(t, VMclient, "sudo cat \"/tmp/%s\" | sudo gpg --import --batch --no-default-keyring --keyring \"%s\"", key, aptUsrShareKeyring)
		}
	})
	if (is.osDesc.Flavor == e2eos.Ubuntu && is.osVersion < 15) || (is.osDesc.Flavor == e2eos.Debian && is.osVersion < 9) {
		is.T().Run("create /etc/apt keyring", func(t *testing.T) {
			ExecuteWithoutError(t, VMclient, "sudo cp %s %s", aptUsrShareKeyring, aptTrustedDKeyring)
		})
	}

	is.T().Run("install debian", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo apt-get update")
		ExecuteWithoutError(is.T(), VMclient, "sudo apt-get install %s datadog-signing-keys -y -q", *flavorName)
	})
}

func (is *stepByStepSuite) StepByStepRhelTest(VMclient *common.TestClient) {
	var arch string
	if is.osDesc.Architecture == e2eos.ARM64Arch {
		arch = "aarch64"
	} else {
		arch = "x86_64"
	}
	yumrepo := fmt.Sprintf("http://s3.amazonaws.com/yumtesting.datad0g.com/testing/pipeline-%s-a%s/%s/%s/",
		os.Getenv("E2E_PIPELINE_ID"), *majorVersion, *majorVersion, arch)
	fileManager := VMclient.FileManager
	var err error

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
		"gpgkey=%s://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_CURRENT.public\n"+
		"\t%s://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_B01082D3.public\n"+
		"\t%s://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"\t%s://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_E09422B3.public",
		yumrepo, repogpgcheck, protocol, protocol, protocol, protocol)
	_, err = fileManager.WriteFile("/etc/yum.repos.d/datadog.repo", []byte(fileContent))
	require.NoError(is.T(), err)

	is.T().Run("install rhel", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo yum makecache -y")
		ExecuteWithoutError(t, VMclient, "sudo yum install -y %s", *flavorName)
	})
}

func (is *stepByStepSuite) StepByStepSuseTest(VMclient *common.TestClient) {
	var arch string
	if is.osDesc.Architecture == e2eos.ARM64Arch {
		arch = "aarch64"
	} else {
		arch = "x86_64"
	}

	suseRepo := fmt.Sprintf("http://s3.amazonaws.com/yumtesting.datad0g.com/suse/testing/pipeline-%s-a%s/%s/%s/",
		os.Getenv("E2E_PIPELINE_ID"), *majorVersion, *majorVersion, arch)
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
		"gpgkey=https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_CURRENT.public\n"+
		"	    https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_B01082D3.public\n"+
		"	    https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"	    https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_E09422B3.public\n",
		suseRepo)
	_, err = fileManager.WriteFile("/etc/zypp/repos.d/datadog.repo", []byte(fileContent))
	require.NoError(is.T(), err)

	is.T().Run("install suse", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_CURRENT.public https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_CURRENT.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_CURRENT.public")
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_B01082D3.public https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_B01082D3.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_B01082D3.public")
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_FD4BF915.public https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_FD4BF915.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_FD4BF915.public")
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_E09422B3.public https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_E09422B3.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_E09422B3.public")
		ExecuteWithoutError(t, VMclient, "sudo zypper --non-interactive --no-gpg-checks refresh datadog")
		ExecuteWithoutError(t, VMclient, "sudo zypper --non-interactive --no-refresh install %s", *flavorName)
	})
}
