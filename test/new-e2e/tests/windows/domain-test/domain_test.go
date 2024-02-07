package domain

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	ad "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/active_directory"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"
	"github.com/stretchr/testify/assert"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testDomainSuite struct {
	e2e.BaseSuite[ad.ActiveDirectoryEnv]

	agentPackage *windowsAgent.Package
	majorVersion string
}

func TestOnDomainController(t *testing.T) {
	agentPackage, err := windowsAgent.GetPackageFromEnv()
	if err != nil {
		t.Fatalf("failed to get MSI URL from env: %v", err)
	}
	t.Logf("Using Agent: %#v", agentPackage)
	majorVersion := strings.Split(agentPackage.Version, ".")[0]

	e2e.Run(t, &testDomainSuite{
		agentPackage: agentPackage,
		majorVersion: majorVersion,
	}, e2e.WithProvisioner(ad.Provisioner(
		ad.WithActiveDirectoryOptions(
			ad.WithDomainName("datadogqalab.com"),
			ad.WithDomainPassword("TestPassword1234#"),
			ad.WithDomainUser("TestUser", "TestPassword1234#"),
		))))
}

func installAgentPackage(host *components.RemoteHost, agentPackage *windowsAgent.Package, args string, logfile string) (string, error) {
	remoteMSIPath, err := windows.GetTemporaryFile(host)
	if err != nil {
		return "", err
	}
	err = windows.PutOrDownloadFile(host, agentPackage.URL, remoteMSIPath)
	if err != nil {
		return "", err
	}
	return remoteMSIPath, windows.InstallMSI(host, remoteMSIPath, args, logfile)
}

// hostIPFromURL extracts the host from the given URL and returns any IP associated to that host
// or an error
func hostIPFromURL(fakeintakeURL string) (string, error) {
	parsed, err := url.Parse(fakeintakeURL)
	if err != nil {
		return "", err
	}

	host := parsed.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no ip for host %s", host)
	}

	// return any valid ip
	return ips[0].String(), nil
}

func (suite *testDomainSuite) TestGivenDomainUserCanInstallAgent() {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), suite.T())
	suite.Require().NoError(err, "should get output dir")
	suite.T().Logf("Output dir: %s", outputDir)

	host := suite.Env().DomainControllerHost

	fakeIntakeIP, err := hostIPFromURL(suite.Env().FakeIntake.URL)
	cmdline := fmt.Sprintf("SITE=%s DDAGENTUSER_NAME=datadogqalab.com\\DatadogTestUser DDAGENTUSER_PASSWORD=\"TestPassword1234#\"", fakeIntakeIP)
	suite.T().Logf("Using command line %s", cmdline)
	_, err = installAgentPackage(host, suite.agentPackage, cmdline, filepath.Join(outputDir, "tc_ins_dc_006_install.log"))
	suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")

	suite.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := suite.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}
