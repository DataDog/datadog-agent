// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

import (
	_ "embed"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scenwindows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
)

type apmvmSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

//go:embed fixtures/system-probe.yaml
var systemProbeConfig string

func TestUSMAutoTaggingSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoFakeIntake(
		awsHostWindows.WithRunOptions(
			scenwindows.WithAgentOptions(
				agentparams.WithSystemProbeConfig(systemProbeConfig),
			),
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

	defaultFiles  usmTaggingFiles
	siteFiles     map[string]usmTaggingFiles
	clientEnvVars map[string]string

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

// packagePath resolves rel against this package's source directory.
//
// Asset paths must not be resolved against the process working directory. The
// test binary is only run from the package directory by convention -- `go test`
// chdirs there, and the prebuilt-binary launcher sets Dir explicitly -- but
// neither is guaranteed, and when the process starts elsewhere a relative path
// silently points at nothing. Deriving the directory from this file's own
// compile-time path keeps asset lookups correct either way.
//
// An already-absolute path is returned unchanged, so the helper is safe to apply
// unconditionally at the copy helpers below.
func packagePath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	_, srcfile, _, ok := runtime.Caller(0)
	if !ok {
		panic("sysprobe-functional: could not determine the test package source path")
	}
	return filepath.Join(filepath.Dir(srcfile), rel)
}

func (v *apmvmSuite) SetupSuite() {
	t := v.T()

	// this creates the VM.
	v.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer v.CleanupOnSetupFailure()

	// get the remote host
	vm := v.Env().RemoteHost

	err := windows.InstallIIS(vm)
	require.NoError(t, err)
	// HEADSUP the paths are windows, but this will execute in linux. So fix the paths
	t.Log("IIS Installed, continuing")

	t.Log("Creating sites")
	// These paths are consumed on the remote windows host but built here on
	// linux, so they must be in native separators.
	for idx := range sites {
		sites[idx].AssetsDir = packagePath("assets")
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

	host.CopyFile(packagePath(filename), sitepath)
	return nil
}

func copyFileToAppRoot(host *components.RemoteHost, app windows.IISApplicationDefinition, filename, targetfilename string) error {

	apppath := path.Join(app.PhysicalPath, targetfilename)
	host.CopyFile(packagePath(filename), apppath)
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
		vm.CopyFile(packagePath(test.clientJSONFile), clientJSONFile)
	}

	if test.clientAppConfig != "" {
		vm.CopyFile(packagePath(test.clientAppConfig), clientAppConfig)
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
	vm.CopyFile(packagePath("usmtest/test_tags.ps1"), testScript)

	testExe := path.Join("c:", "users", "administrator", "littleget.exe")
	vm.CopyFile(packagePath("usmtest/littleget.exe"), testExe)

	pipeExe := path.Join("c:", "users", "administrator", "NamedPipeCmd.exe")
	vm.CopyFile(packagePath("usmtest/NamedPipeCmd.exe"), pipeExe)

	pscommand := "%s %s -TargetHost localhost -TargetPort %s -TargetPath %s -ExpectedClientTags %s -ExpectedServerTags %s -ConnExe %s"

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
			var envstringBuilder strings.Builder
			for k, v := range test.clientEnvVars {
				fmt.Fprintf(&envstringBuilder, "$Env:%s=\"%s\" ; ", k, v)
			}
			localcmd := fmt.Sprintf(pscommand, envstringBuilder.String(), testScript, targetport, targetpath, strings.Join(test.expectedClientTags, ","), strings.Join(test.expectedServerTags, ","), testExe)

			if len(test.clientEnvVars) > 0 {
				var envargBuilder strings.Builder
				for k, v := range test.clientEnvVars {
					fmt.Fprintf(&envargBuilder, "%s=%s", k, v)
				}
			}

			out, err := vm.Execute(localcmd)
			if err != nil {
				t.Logf("Error running test: %v", out)
			}
			assert.NoError(t, err)
		})
	}
}
