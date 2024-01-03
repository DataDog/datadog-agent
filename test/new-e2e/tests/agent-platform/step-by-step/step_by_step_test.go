// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package stepbystep

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"strings"
	"testing"
)

var osVersion = flag.String("osversion", "", "os version to test")
var platform = flag.String("platform", "", "platform to test")
var cwsSupportedOsVersion = flag.String("cws-supported-osversion", "", "list of os where CWS is supported")
var architecture = flag.String("arch", "", "architecture to test (x86_64, arm64))")
var flavorName = flag.String("flavor", "datadog-agent", "package flavor to install")
var majorVersion = flag.String("major-version", "7", "major version to test (6, 7)")

type stepByStepSuite struct {
	e2e.Suite[e2e.VMEnv]
	osVersion    float64
	cwsSupported bool
}

func ExecuteWithoutError(t *testing.T, client *common.TestClient, cmd string, args ...any) {
	var finalCmd string
	if len(args) > 0 {
		finalCmd = fmt.Sprintf(cmd, args...)
	} else {
		finalCmd = cmd
	}
	_, err := client.VMClient.ExecuteWithError(finalCmd)
	require.NoError(t, err)
}

func TestStepByStepScript(t *testing.T) {
	osMapping := map[string]ec2os.Type{
		"debian":      ec2os.DebianOS,
		"ubuntu":      ec2os.UbuntuOS,
		"centos":      ec2os.CentOS,
		"amazonlinux": ec2os.AmazonLinuxOS,
		"redhat":      ec2os.RedHatOS,
		"rhel":        ec2os.RedHatOS,
		"sles":        ec2os.SuseOS,
		"windows":     ec2os.WindowsOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
		"rocky":       ec2os.RockyLinux,
	}

	archMapping := map[string]e2eOs.Architecture{
		"x86_64": e2eOs.AMD64Arch,
		"arm64":  e2eOs.ARM64Arch,
	}

	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	osVersions := strings.Split(*osVersion, ",")
	cwsSupportedOsVersionList := strings.Split(*cwsSupportedOsVersion, ",")
	fmt.Println("Parsed platform json file: ", platformJSON)
	for _, osVers := range osVersions {
		vmOpts := []ec2params.Option{}
		osVers := osVers
		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}

		var testOsType ec2os.Type
		for osName, osType := range osMapping {
			if strings.Contains(osVers, osName) {
				testOsType = osType
			}
		}

		t.Run(fmt.Sprintf("test step by step on %s %s", osVers, *architecture), func(tt *testing.T) {
			tt.Parallel()
			fmt.Printf("Testing %s", osVers)
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
			vmOpts = append(vmOpts, ec2params.WithImageName(platformJSON[*platform][*architecture][osVers], archMapping[*architecture], testOsType))
			if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
				vmOpts = append(vmOpts, ec2params.WithInstanceType(instanceType))
			}
			e2e.Run(tt, &stepByStepSuite{cwsSupported: cwsSupported, osVersion: version}, e2e.EC2VMStackDef(vmOpts...), params.WithStackName(fmt.Sprintf("step-by-step-test-%v-%v-%s-%s", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture, *majorVersion)))
		})
	}
}

func (is *stepByStepSuite) TestStepByStep() {
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)
	unixHelper := helpers.NewUnixHelper()
	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	if *platform == "debian" || *platform == "ubuntu" {
		is.StepByStepDebianTest(VMclient)
	} else if *platform == "centos" || *platform == "amazonlinux" || *platform == "fedora" || *platform == "redhat" {
		is.StepByStepRhelTest(VMclient)
	} else {
		require.Equal(is.T(), *platform, "suse", "NonSupportedPlatformError : %s isn't supported !", *platform)
		is.StepByStepSuseTest(VMclient)
	}
	is.ConfigureAndRunAgentService(VMclient)
	is.CheckStepByStepAgentInstallation(VMclient)

}

func (is *stepByStepSuite) ConfigureAndRunAgentService(VMclient *common.TestClient) {
	is.T().Run("add config file", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo sh -c \"sed 's/api_key:.*/api_key: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX/' /etc/datadog-agent/datadog.yaml.example > /etc/datadog-agent/datadog.yaml\"")
		ExecuteWithoutError(t, VMclient, "sudo sh -c \"chown dd-agent:dd-agent /etc/datadog-agent/datadog.yaml && chmod 640 /etc/datadog-agent/datadog.yaml\"")
		if (*platform == "ubuntu" && is.osVersion == 14.04) || (*platform == "centos" && is.osVersion == 6.10) {
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
	common.CheckAgentPython(is.T(), VMclient, "3")
	if *majorVersion == "6" {
		common.CheckAgentPython(is.T(), VMclient, "2")
	}
	common.CheckApmEnabled(is.T(), VMclient)
	common.CheckApmDisabled(is.T(), VMclient)
	if *flavorName == "datadog-agent" && is.cwsSupported {
		common.CheckCWSBehaviour(is.T(), VMclient)
	}
	common.CheckUninstallation(is.T(), VMclient, *flavorName)
}

func (is *stepByStepSuite) StepByStepDebianTest(VMclient *common.TestClient) {
	var aptTrustedDKeyring = "/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg"
	var aptUsrShareKeyring = "/usr/share/keyrings/datadog-archive-keyring.gpg"
	var aptrepo = "[signed-by=/usr/share/keyrings/datadog-archive-keyring.gpg] http://apttesting.datad0g.com/"
	var aptrepoDist = fmt.Sprintf("pipeline-%s-a%s-%s", os.Getenv("CI_PIPELINE_ID"), *majorVersion, *architecture)
	fileManager := VMclient.FileManager
	var err error

	is.T().Run("create /usr/share keyring and source list", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg")
		tmpFileContent := fmt.Sprintf("deb %s %s %s", aptrepo, aptrepoDist, *majorVersion)
		_, err = fileManager.WriteFile("/etc/apt/sources.list.d/datadog.list", tmpFileContent)
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

	is.T().Run("install debian", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo apt-get update")
		ExecuteWithoutError(is.T(), VMclient, "sudo apt-get install %s datadog-signing-keys -y -q", *flavorName)
	})
}

func (is *stepByStepSuite) StepByStepRhelTest(VMclient *common.TestClient) {
	var arch string
	if *architecture == "arm64" {
		arch = "aarch64"
	} else {
		arch = *architecture
	}
	var yumrepo = fmt.Sprintf("http://yumtesting.datad0g.com/testing/pipeline-%s-a%s/%s/%s/",
		os.Getenv("CI_PIPELINE_ID"), *majorVersion, *majorVersion, arch)
	fileManager := VMclient.FileManager
	var err error

	var protocol = "https"
	if is.osVersion < 6 {
		protocol = "http"
	}
	var repogpgcheck = "1"
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
	_, err = fileManager.WriteFile("/etc/yum.repos.d/datadog.repo", fileContent)
	require.NoError(is.T(), err)

	is.T().Run("install rhel", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo yum makecache -y")
		ExecuteWithoutError(t, VMclient, "sudo yum install -y %s", *flavorName)
	})
}

func (is *stepByStepSuite) StepByStepSuseTest(VMclient *common.TestClient) {
	var arch string
	if *architecture == "arm64" {
		arch = "aarch64"
	} else {
		arch = *architecture
	}

	var suseRepo = fmt.Sprintf("http://yumtesting.datad0g.com/suse/testing/pipeline-%s-a%s/%s/%s/",
		os.Getenv("CI_PIPELINE_ID"), *majorVersion, *majorVersion, arch)
	fileManager := VMclient.FileManager
	var err error

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
	_, err = fileManager.WriteFile("/etc/zypp/repos.d/datadog.repo", fileContent)
	require.NoError(is.T(), err)

	is.T().Run("install suse", func(t *testing.T) {
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_CURRENT.public https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_CURRENT.public")
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_B01082D3.public https://keys.datadoghq.com/DATADOG_RPM_KEY_B01082D3.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_B01082D3.public")
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_FD4BF915.public https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_FD4BF915.public")
		ExecuteWithoutError(t, VMclient, "sudo curl -o /tmp/DATADOG_RPM_KEY_E09422B3.public https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public")
		ExecuteWithoutError(t, VMclient, "sudo rpm --import /tmp/DATADOG_RPM_KEY_E09422B3.public")
		ExecuteWithoutError(t, VMclient, "sudo zypper --non-interactive --no-gpg-checks refresh datadog")
		ExecuteWithoutError(t, VMclient, "sudo zypper --non-interactive install %s", *flavorName)
	})
}
