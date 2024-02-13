package http

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"
	componentsos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vmSuite struct {
	e2e.BaseSuite[environments.Host]
}

var (
	assetDir   string
	kitchenDir string
	testspath  string
	devMode    = flag.Bool("devmode", false, "run tests in dev mode")
)

func init() {
	// Get the absolute path to the test assets directory
	currDir, _ := os.Getwd()
	//test\new-e2e\tests\network\protocol\http\http_test.go
	//D:\src\agent.e2e\pkg\network\protocols\http
	assetDir = filepath.Join(currDir, "..", "..", "..", "..", "..", "..", "pkg", "network", "protocols", "http", "testsuite.exe")

	kitchenDir = filepath.Join(currDir, "..", "..", "..", "..", "..", "..", "test", "kitchen", "site-cookbooks")
	testspath = filepath.Join(kitchenDir, "dd-system-probe-check", "files", "default", "tests", "pkg", "network")
}

func TestVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(componentsos.WindowsDefault))))}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) TestTestSuite() {
	v.T().Run("Works", v.testExample)
}

/*
func TestLocal(t *testing.T) {

		tests := findTestPrograms(t, testspath, "testsuite.exe")
		for _, test := range tests {
			t.Logf("Found %s\n", test)
		}
	}
*/
func (v *vmSuite) testExample(t *testing.T) {

	// get the remote host
	vm := v.Env().RemoteHost

	out, err := vm.Execute("whoami")
	if err != nil {
		t.Fatalf("Error executing command: %v", err)
	}

	// log who we are
	t.Logf("whoami: %s", out)

	err = windows.InstallIIS(vm)
	require.NoError(t, err)
	// HEADSUP the paths are windows, but this will execute in linux. So fix the paths
	t.Log("IIS Installed, continuing")

	t.Log("Creating sites")
	// figure out where we're being executed from.  These paths should be in
	// native path separators (i.e. not windows paths if executing in ci/on linux)

	_, srcfile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	exPath := filepath.Dir(srcfile)

	sites := []windows.IISSiteDefinition{
		{
			Name:        "TestSite1",
			BindingPort: "*:8081:",
			AssetsDir:   path.Join(exPath, "assets"),
		},
		{
			Name:        "TestSite2",
			BindingPort: "*:8082:",
			AssetsDir:   path.Join(exPath, "assets"),
		},
	}

	t.Logf("AssetsDir: %s", sites[0].AssetsDir)
	err = windows.CreateIISSite(vm, sites)
	require.NoError(t, err)
	t.Log("Sites created, continuing")

	testsuites := findTestPrograms(t, testspath, "testsuite.exe")

	remoteDirBase := "c:\\tmp"
	for _, testsuite := range testsuites {
		remotePath := filepath.Join(remoteDirBase, testsuite)
		remoteDir := filepath.Dir(remotePath)
		t.Logf("Creating remote directory %s", remoteDir)
		remoteDir = strings.Replace(remoteDir, "\\", "/", -1)
		err := vm.MkdirAll(remoteDir)
		if err != nil {
			t.Logf("Error creating remote directory: %s %v", remoteDir, err)
		} else {
			t.Logf("Created remote directory: %s", remoteDir)
		}
		assert.NoError(t, err)
		remoteFile := filepath.Join(remotePath)          //, "testsuite.exe")
		localFile := filepath.Join(testspath, testsuite) //, "testsuite.exe")
		fi, err := os.Stat(localFile)
		t.Logf("Copying %s to %s (size %d)", localFile, remoteFile, fi.Size())
		s := time.Now()
		vm.CopyFile(localFile, remoteFile)
		d := time.Now()
		t.Logf("Copy took %v, rate %.2f Mb/s", d.Sub(s), (float64(fi.Size()*8)/1024/1024)/float64(d.Sub(s).Seconds()))
		assert.NoError(t, err)

		// check to see if there's a testdata directory.  If there is, copy that too
		localDir := filepath.Dir(localFile)
		testdata := filepath.Join(localDir, "testdata")
		td, err := os.Stat(testdata)
		if err == nil && td.IsDir() {
			t.Logf("Copying testdata dir %s to %s", testdata, filepath.Join(remoteDir, "testdata"))
			vm.CopyFolder(testdata, filepath.Join(remoteDir, "testdata"))
		}

	}

	// install the agent (just so we can get the driver(s) installed)
	agentPackage, err := windowsAgent.GetPackageFromEnv()
	require.NoError(t, err)
	remoteMSIPath, err := windows.GetTemporaryFile(vm)
	require.NoError(t, err)
	t.Log("Getting install package...")
	err = windows.PutOrDownloadFile(vm, agentPackage.URL, remoteMSIPath)
	require.NoError(t, err)

	err = windows.InstallMSI(vm, remoteMSIPath, "", "")
	t.Log("Install complete")
	require.NoError(t, err)

	// disable the agent, and enable the drivers for testing
	_, err = vm.Execute("stop-service -force datadogagent")
	require.NoError(t, err)
	_, err = vm.Execute("sc.exe config datadogagent start= disabled")
	require.NoError(t, err)
	_, err = vm.Execute("sc.exe config ddnpm start= demand")
	require.NoError(t, err)
	_, err = vm.Execute("start-service ddnpm")
	require.NoError(t, err)

	t.Log("Testsuites copied, continuing")
	// run the test suites
	for _, testsuite := range testsuites {
		t.Logf("Running testsuite: %s", testsuite)
		remotePath := filepath.Join(remoteDirBase, testsuite) //, "testsuite.exe")
		executeAndLogOutput(t, vm, remotePath, "\"-test.v\"")
	}

}

func executeAndLogOutput(t *testing.T, vm *components.RemoteHost, command string, args ...string) {
	cmdDir := filepath.Dir(command)
	outfilename := command + ".out"
	fullcommand := "cd " + cmdDir + ";"
	fullcommand += command + " " + strings.Join(args, " ") + " | Out-File -Encoding ASCII -FilePath " + outfilename
	_, err := vm.Execute(fullcommand)
	require.NoError(t, err)

	// get the output
	outbytes, err := vm.ReadFile(outfilename)
	require.NoError(t, err)

	// log the output
	for _, line := range strings.Split(string(outbytes[:]), "\n") {
		t.Logf("TestSuite: %s", line)
	}
}

func findTestPrograms(t *testing.T, rootdir, filespec string) []string {
	var testfiles []string
	err := filepath.Walk(rootdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatalf("Error walking path: %v", err)
		}
		if !info.IsDir() && strings.Contains(info.Name(), filespec) {
			testfiles = append(testfiles, strings.Replace(path, rootdir, "", 1)[1:]) // cut off leading slash
		}
		return nil
	})
	require.NoError(t, err)
	return testfiles
}
