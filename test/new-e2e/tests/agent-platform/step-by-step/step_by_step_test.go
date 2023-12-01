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
var packageName = flag.String("packagename", "datadog-agent", "name of the package")
var majorVersion = flag.String("major-version", "7", "major version to test (6, 7)")

type stepByStepSuite struct {
	e2e.Suite[e2e.VMEnv]
	agentMajorVersion int
	osVersion         float64
	cwsSupported      bool
}

func ExecuteWithoutError(cmd string, t *testing.T, client *common.TestClient) {
	_, err := client.VMClient.ExecuteWithError(cmd)
	require.NoError(t, err)
}

func TestStepByStepScript(t *testing.T) {
	osMapping := map[string]ec2os.Type{
		"debian":      ec2os.DebianOS,
		"ubuntu":      ec2os.UbuntuOS,
		"centos":      ec2os.CentOS,
		"amazonlinux": ec2os.AmazonLinuxOS,
		"redhat":      ec2os.RedHatOS,
		"windows":     ec2os.WindowsOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
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
		osVers := osVers
		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}

		t.Run(fmt.Sprintf("test step by step on %s %s", osVers, *architecture), func(tt *testing.T) {
			tt.Parallel()
			fmt.Printf("Testing %s", osVers)
			slice := strings.Split(osVers, "-")
			var version float64
			if len(slice) == 2 {
				version, err = strconv.ParseFloat(slice[1], 64)
				require.NoError(tt, err)
			} else {
				version = 0
			}
			majVersion, _ := strconv.Atoi(*majorVersion)
			e2e.Run(tt, &stepByStepSuite{cwsSupported: cwsSupported, osVersion: version, agentMajorVersion: majVersion}, e2e.EC2VMStackDef(ec2params.WithImageName(platformJSON[*platform][*architecture][osVers], archMapping[*architecture], osMapping[*platform])), params.WithStackName(fmt.Sprintf("step-by-step-test-%v-%v-%s", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture)))
		})
	}
}

func (is *stepByStepSuite) TestStepByStep() {
	if *platform == "debian" || *platform == "ubuntu" {
		StepByStepDebianTest(is)
	} else if *platform == "centos" || *platform == "amazonlinux" || *platform == "fedora" || *platform == "redhat" {
		StepByStepRhelTest(is)
	} else if *platform == "suse" {
		StepByStepSuseTest(is)
	}
}

func StepByStepDebianTest(is *stepByStepSuite) {
	var aptTrustedDKeyring = "/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg"
	var aptUsrShareKeyring = "/usr/share/keyrings/datadog-archive-keyring.gpg"
	var aptrepo = "[signed-by=/usr/share/keyrings/datadog-archive-keyring.gpg] http://apttesting.datad0g.com/"
	var aptrepoDist = fmt.Sprintf("pipeline-%s-a%d-%s", os.Getenv("CI_PIPELINE_ID"), is.agentMajorVersion, *architecture)
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)
	unixHelper := helpers.NewUnixHelper()
	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	is.T().Run("create /usr/share keyring and source list", func(t *testing.T) {
		ExecuteWithoutError("sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg", t, VMclient)
		tmpCmd := fmt.Sprintf("deb %s %s %v", aptrepo, aptrepoDist, is.agentMajorVersion)
		_, err = fileManager.WriteFile("/etc/apt/sources.list.d/datadog.list", tmpCmd)
		require.NoError(t, err)
		tmpCmd = fmt.Sprintf("sudo touch %s && sudo chmod a+r %s", aptUsrShareKeyring, aptUsrShareKeyring)
		ExecuteWithoutError(tmpCmd, t, VMclient)
		keys := []string{"DATADOG_APT_KEY_CURRENT.public", "DATADOG_APT_KEY_F14F620E.public", "DATADOG_APT_KEY_382E94DE.public"}
		for _, key := range keys {
			tmpCmd = fmt.Sprintf("sudo curl --retry 5 -o \"/tmp/%s\" \"https://keys.datadoghq.com/%s\"", key, key)
			ExecuteWithoutError(tmpCmd, t, VMclient)
			tmpCmd = fmt.Sprintf("sudo cat \"/tmp/%s\" | sudo gpg --import --batch --no-default-keyring --keyring \"%s\"", key, aptUsrShareKeyring)
			ExecuteWithoutError(tmpCmd, t, VMclient)
		}
	})
	if (*platform == "ubuntu" && is.osVersion < 16) || (*platform == "debian" && is.osVersion < 9) {
		is.T().Run("create /etc/apt keyring", func(t *testing.T) {
			tmpCmd := fmt.Sprintf("sudo cp %s %s", aptUsrShareKeyring, aptTrustedDKeyring)
			ExecuteWithoutError(tmpCmd, t, VMclient)
		})
	}

	is.T().Run("install debian", func(t *testing.T) {
		ExecuteWithoutError("sudo apt-get update", t, VMclient)
		tmpCmd := fmt.Sprintf("sudo apt-get install %s -y -q", *packageName)
		ExecuteWithoutError(tmpCmd, is.T(), VMclient)
	})
}

func StepByStepRhelTest(is *stepByStepSuite) {
	var arch string
	if *architecture == "arm64" {
		arch = "aarch64"
	} else {
		arch = *architecture
	}
	var yumrepo = fmt.Sprintf("http://yumtesting.datad0g.com/testing/pipeline-%s-a%d/%d/%s/",
		os.Getenv("CI_PIPELINE_ID"), is.agentMajorVersion, is.agentMajorVersion, arch)

	fileManager := filemanager.NewUnixFileManager(is.Env().VM)
	unixHelper := helpers.NewUnixHelper()
	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	if *platform == "centos" && is.cwsSupported {
		is.T().Run("set SElinux to permissive mode to be able to start system-probe", func(t *testing.T) {
			ExecuteWithoutError("setenforce 0", t, VMclient)
		})
	}

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
		"\t%s://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"\t%s://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public",
		yumrepo, repogpgcheck, protocol, protocol, protocol)
	is.T().Log(fileContent)
	_, err = fileManager.WriteFile("/etc/yum.repos.d/datadog.repo", fileContent)
	require.NoError(is.T(), err)

	is.T().Run("install rhel", func(t *testing.T) {
		ExecuteWithoutError("sudo yum makecache -y", t, VMclient)
		tmpCmd := fmt.Sprintf("sudo yum install -y %s", *packageName)
		ExecuteWithoutError(tmpCmd, t, VMclient)
	})
}

func StepByStepSuseTest(is *stepByStepSuite) {
	var suserepo = ""
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)
	unixHelper := helpers.NewUnixHelper()
	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)
	VMclient := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	fileContent := fmt.Sprintf("[datadog]\n"+
		"name = Datadog, Inc.\n"+
		"baseurl = %s\n"+
		"enabled=1\n"+
		"gpgcheck=1\n"+
		"repo_gpgcheck=1\n"+
		"gpgkey=https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public\n"+
		"	    https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public\n"+
		"	    https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public\n",
		suserepo)
	_, err = fileManager.WriteFile("/etc/zypp/repos.d/datadog.repo", fileContent)
	require.NoError(is.T(), err)

	is.T().Run("install suse", func(t *testing.T) {
		ExecuteWithoutError("sudo curl -o /tmp/DATADOG_RPM_KEY_CURRENT.public https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public", t, VMclient)
		ExecuteWithoutError("sudo rpm --import /tmp/DATADOG_RPM_KEY_CURRENT.public", t, VMclient)
		ExecuteWithoutError("sudo curl -o /tmp/DATADOG_RPM_KEY_FD4BF915.public https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public", t, VMclient)
		ExecuteWithoutError("sudo rpm --import /tmp/DATADOG_RPM_KEY_FD4BF915.public", t, VMclient)
		ExecuteWithoutError("sudo curl -o /tmp/DATADOG_RPM_KEY_E09422B3.public https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public", t, VMclient)
		ExecuteWithoutError("sudo rpm --import /tmp/DATADOG_RPM_KEY_E09422B3.public", t, VMclient)
		ExecuteWithoutError("sudo zypper --non-interactive --no-gpg-checks refresh datadog", t, VMclient)
		tmpCmd := fmt.Sprintf("sudo zypper --non-interactive install %s", *packageName)
		ExecuteWithoutError(tmpCmd, t, VMclient)
	})
}
