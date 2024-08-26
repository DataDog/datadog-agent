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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	//awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	//windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	//windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	//componentsos "github.com/DataDog/test-infra-definitions/components/os"
	//"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
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
	jsonFile 		string
	appConfigFile 	string
}
type usmTaggingTest struct {
	name string
	description string

	clientJsonFile	string
	clientAppConfig string

	defaultFiles usmTaggingFiles
	siteFiles map[string]usmTaggingFiles  

	appFiles map[string]usmTaggingFiles

	serverSiteName string
	serverSitePort string
	targetPath string

	expectedClientTags []string
	expectedServerTags []string
}

var usmTaggingTests = []usmTaggingTest{
	{
		name: "all values json test 1",
		description: "Basic test with all values from json",
		clientJsonFile: "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile: "usmtest/defaultsite_all.json",
			appConfigFile: "",
		},
		serverSiteName: "", // empty is default site
		serverSitePort: "80",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:defaultsite_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:1",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:Default Web Site'",
		},
	},
	{
		name: "all values xml test 1",
		description: "Test with both json and app config provided, xml supercedes json",
		clientJsonFile: "usmtest/client_all.json",
		clientAppConfig: "usmtest/client_all.xml",
		defaultFiles: usmTaggingFiles{
			jsonFile: "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		serverSiteName: "", // empty is default site
		serverSitePort: "80",
		expectedClientTags: []string{
			"service:webclient_xml",
			"env:testing_env_xml",
			"version:1_xml",
		},
		expectedServerTags: []string{
			"service:defaultsite_xml",
			"env:testing_env_xml",
			"version:1_xml",
			"http.iis.site:1",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:Default Web Site'",
		},
	},
	{
		name: "test different site",
		description: "Test different site in same IIS server, tests correct path discovery",
		clientJsonFile: "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile: "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": usmTaggingFiles{
				jsonFile: "usmtest/site1.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:site1_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	{
		name: "test site with application",
		description: "Test different site in same IIS server, tests correct path discovery with an application",
		clientJsonFile: "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile: "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": usmTaggingFiles{
				jsonFile: "usmtest/site1.json",
			},
		},
		appFiles: map[string]usmTaggingFiles{
			"/site1/app1": usmTaggingFiles{
				jsonFile: "usmtest/site1_app1.json",
			},
			"/site1/app2": usmTaggingFiles{
				jsonFile: "usmtest/site1_app2.json",
			},
			"/site1/app2/nested": usmTaggingFiles{
				jsonFile: "usmtest/app2_nested.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		targetPath: "/site1/app1",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:app1_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	{
		name: "test site with second application",
		description: "Test different site in same IIS server, tests correct path discovery with an application",
		clientJsonFile: "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile: "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": usmTaggingFiles{
				jsonFile: "usmtest/site1.json",
			},
		},
		appFiles: map[string]usmTaggingFiles{
			"/site1/app1": usmTaggingFiles{
				jsonFile: "usmtest/site1_app1.json",
			},
			"/site1/app2": usmTaggingFiles{
				jsonFile: "usmtest/site1_app2.json",
			},
			"/site1/app2/nested": usmTaggingFiles{
				jsonFile: "usmtest/app2_nested.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		targetPath: "/site1/app2",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:app2_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	{
		name: "test site with nested application",
		description: "Test different site in same IIS server, tests correct path discovery with an application",
		clientJsonFile: "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile: "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": usmTaggingFiles{
				jsonFile: "usmtest/site1.json",
			},
		},
		appFiles: map[string]usmTaggingFiles{
			"/site1/app1": usmTaggingFiles{
				jsonFile: "usmtest/site1_app1.json",
			},
			"/site1/app2": usmTaggingFiles{
				jsonFile: "usmtest/site1_app2.json",
			},
			"/site1/app2/nested": usmTaggingFiles{
				jsonFile: "usmtest/app2_nested.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		targetPath: "/site1/app2/nested",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:app2_nested_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	
}

var sites = []windows.IISSiteDefinition{
	{
		Name:        "TestSite1",
		BindingPort: "*:8081:",
		SiteDir:  path.Join("c:", "site1"),
		Applications: []windows.IISApplicationDefinition{
			{
				Name: "/site1/app1",
				PhysicalPath: path.Join("c:", "app1"),
			},
			{
				Name: "/site1/app2",
				PhysicalPath: path.Join("c:", "app2"),
			},
			{
				Name: "/site1/app2/nested",
				PhysicalPath: path.Join("c:", "app2", "nested"),
			},
		},
	},
	{
		Name:        "TestSite2",
		BindingPort: "*:8082:",
		SiteDir:  path.Join("c:", "site2"),
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

	for idx, _ := range sites {
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

	clientJsonFile := path.Join(testRoot, "datadog.json")
	clientAppConfig := path.Join(testRoot, "app.config")

	removeIfExists(vm, clientJsonFile)
	removeIfExists(vm, clientAppConfig)

	if test.clientJsonFile != "" {
		vm.CopyFile(test.clientJsonFile, clientJsonFile)
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
				}
				break
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
	

	pscommand := "%s -TargetHost localhost -TargetPort %s -TargetPath %s -ExpectedClientTags %s -ExpectedServerTags %s -ConnExe %s"

	for _, test := range usmTaggingTests {
		v.Run(test.name, func() {
			t := v.T()

			t.Logf(test.description)

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