// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

import (
	_ "embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type apmvmSuite struct {
	e2e.BaseSuite[environments.WindowsHost]

	testspath string
}

//go:embed fixtures/system-probe.yaml
var systemProbeConfig string

func TestUSMAutoTaggingSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoFakeIntake(
		awsHostWindows.WithAgentOptions(
			agentparams.WithSystemProbeConfig(systemProbeConfig),
		),
	))}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &apmvmSuite{}, suiteParams...)
}

type usmTaggingFiles struct {
	jsonFile      string
	appConfigFile string
}
type usmTaggingTest struct {
	name        string
	description string

	clientJSONFile  string
	clientAppConfig string

	defaultFiles usmTaggingFiles
	siteFiles    map[string]usmTaggingFiles

	appFiles map[string]usmTaggingFiles

	serverSiteName string
	serverSitePort string
	targetPath     string

	expectedClientTags []string
	expectedServerTags []string
}

var sites = []windows.IISSiteDefinition{
	{
		Name:        "TestSite1",
		BindingPort: "*:8081:",
		SiteDir:     path.Join("c:", "site1"),
		Applications: []windows.IISApplicationDefinition{
			{
				Name:         "/site1/app1",
				PhysicalPath: path.Join("c:", "app1"),
			},
			{
				Name:         "/site1/app2",
				PhysicalPath: path.Join("c:", "app2"),
			},
			{
				Name:         "/site1/app2/nested",
				PhysicalPath: path.Join("c:", "app2", "nested"),
			},
		},
	},
	{
		Name:        "TestSite2",
		BindingPort: "*:8082:",
		SiteDir:     path.Join("c:", "site2"),
	},
}

func (v *apmvmSuite) SetupSuite() {
	t := v.T()

	// Get the absolute path to the test assets directory
	currDir, err := os.Getwd()
	require.NoError(t, err)

	reporoot, _ := filepath.Abs(filepath.Join(currDir, "..", "..", "..", ".."))
	kitchenDir := filepath.Join(reporoot, "test", "kitchen", "site-cookbooks")
	v.testspath = filepath.Join(kitchenDir, "dd-system-probe-check", "files", "default", "tests")

	// this creates the VM.
	v.BaseSuite.SetupSuite()

	// get the remote host
	vm := v.Env().RemoteHost

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

	for idx := range sites {
		sites[idx].AssetsDir = path.Join(exPath, "assets")
	}

	err = windows.CreateIISSite(vm, sites)
	require.NoError(t, err)
	t.Log("Sites created, continuing")
}

// site directories are
// c:\inetpub\wwwroot for the default site and
// c:\tmp\inetpub\{siteName} for the other sites

// pass sitename as empty string for default site
func copyFileToSiteRoot(host *components.RemoteHost, sitename, filename, targetfilename string) error {

	sitepath := path.Join("c:", "inetpub", "wwwroot", targetfilename)
	if sitename != "" {
		for _, site := range sites {
			if site.Name == sitename {
				if site.SiteDir == "" {
					sitepath = path.Join("c:", "tmp", "inetpub")
				} else {
					sitepath = path.Join(site.SiteDir, targetfilename)
				}
				break
			}
		}
	}

	// test this out.  Should copy path-relative to assets
	host.CopyFile(filename, sitepath)
	return nil
}

func copyFileToAppRoot(host *components.RemoteHost, app windows.IISApplicationDefinition, filename, targetfilename string) error {

	apppath := path.Join(app.PhysicalPath, targetfilename)
	host.CopyFile(filename, apppath)
	return nil
}

func cleanSite(host *components.RemoteHost, pathroot string) error {

	targetjson := path.Join(pathroot, "datadog.json")
	targetconfig := path.Join(pathroot, "web.config")
	removeIfExists(host, targetjson)
	removeIfExists(host, targetconfig)
	return nil
}

func cleanSites(host *components.RemoteHost) error {

	// first clean the default site
	siteroot := path.Join("c:", "inetpub", "wwwroot")
	cleanSite(host, siteroot)

	// now clean the other sites
	for _, site := range sites {
		if site.SiteDir == "" {
			cleanSite(host, path.Join("c:", "tmp", "inetpub", site.Name))
		} else {
			cleanSite(host, site.SiteDir)
		}
		// if the site has applications, clean that too
		for _, app := range site.Applications {
			cleanSite(host, app.PhysicalPath)
		}
	}
	return nil
}

func removeIfExists(host *components.RemoteHost, path string) error {
	exists, err := host.FileExists(path)
	if err != nil {
		return err
	}
	if exists {
		host.Remove(path)
	}
	return nil
}

func setupTest(vm *components.RemoteHost, test usmTaggingTest) error {

	testRoot := path.Join("c:", "users", "administrator")

	clientJSONFile := path.Join(testRoot, "datadog.json")
	clientAppConfig := path.Join(testRoot, "littleget.exe.config")

	removeIfExists(vm, clientJSONFile)
	removeIfExists(vm, clientAppConfig)

	if test.clientJSONFile != "" {
		vm.CopyFile(test.clientJSONFile, clientJSONFile)
	}

	if test.clientAppConfig != "" {
		vm.CopyFile(test.clientAppConfig, clientAppConfig)
	}

	cleanSites(vm)
	if test.defaultFiles.jsonFile != "" {
		err := copyFileToSiteRoot(vm, "", test.defaultFiles.jsonFile, "datadog.json")
		if err != nil {
			return err
		}
	}

	if test.defaultFiles.appConfigFile != "" {
		err := copyFileToSiteRoot(vm, "", test.defaultFiles.appConfigFile, "web.config")
		if err != nil {
			return err
		}
	}
	for site, files := range test.siteFiles {
		if files.jsonFile != "" {
			err := copyFileToSiteRoot(vm, site, files.jsonFile, "datadog.json")
			if err != nil {
				return err
			}
		}
		if files.appConfigFile != "" {
			err := copyFileToSiteRoot(vm, site, files.appConfigFile, "web.config")
			if err != nil {
				return err
			}
		}
	}

	for path, files := range test.appFiles {
		// path is the site path.  See if we can find it
		for _, site := range sites {
			for _, app := range site.Applications {
				if app.Name == path {
					if files.jsonFile != "" {
						err := copyFileToAppRoot(vm, app, files.jsonFile, "datadog.json")
						if err != nil {
							return err
						}
					}
					if files.appConfigFile != "" {
						err := copyFileToAppRoot(vm, app, files.appConfigFile, "web.config")
						if err != nil {
							return err
						}
					}
					break
				}
			}
		}
	}
	return nil
}
func (v *apmvmSuite) TestUSMAutoTaggingSuite() {

	// get the remote host
	vm := v.Env().RemoteHost

	// copy test script
	testScript := path.Join("c:", "users", "administrator", "test_tags.ps1")
	vm.CopyFile("usmtest/test_tags.ps1", testScript)

	testExe := path.Join("c:", "users", "administrator", "littleget.exe")
	vm.CopyFile("usmtest/littleget.exe", testExe)

	pipeExe := path.Join("c:", "users", "administrator", "NamedPipeCmd.exe")
	vm.CopyFile("usmtest/NamedPipeCmd.exe", pipeExe)

	pscommand := "%s -TargetHost localhost -TargetPort %s -TargetPath %s -ExpectedClientTags %s -ExpectedServerTags %s -ConnExe %s"

	for _, test := range usmTaggingTests {
		v.Run(test.name, func() {
			t := v.T()

			t.Logf("%s", test.description)

			err := setupTest(vm, test)
			require.NoError(t, err)

			targetport := "80"
			if test.serverSitePort != "" {
				targetport = test.serverSitePort
			}

			targetpath := "/"
			if test.targetPath != "" {
				targetpath = test.targetPath
			}
			localcmd := fmt.Sprintf(pscommand, testScript, targetport, targetpath, strings.Join(test.expectedClientTags, ","), strings.Join(test.expectedServerTags, ","), testExe)
			out, err := vm.Execute(localcmd)
			if err != nil {
				t.Logf("Error running test: %v", out)
			}
			assert.NoError(t, err)
		})
	}
}
