// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
)

// WindowsSuite is an interface that provides access to the test suite's Require and Env methods.
// It must be implemented by any suite that wants to use IISHelper.
type WindowsSuite interface {
	Env() *environments.WindowsHost
	Require() *suiteasserts.SuiteAssertions
}

// IISHelper provides reusable IIS testing functionality that can be composed with any test suite.
// It requires a suite that implements the WindowsSuite interface to access the remote host and assertions.
type IISHelper struct {
	suite WindowsSuite
}

// NewIISHelper creates a new IISHelper with the given suite.
func NewIISHelper(s WindowsSuite) *IISHelper {
	return &IISHelper{suite: s}
}

// SetupIIS installs IIS and ASP.NET on the remote host.
// This should be called in the suite's SetupSuite method.
func (h *IISHelper) SetupIIS() {
	h.installIIS()
	h.installAspNet()
}

func (h *IISHelper) installIIS() {
	host := h.suite.Env().RemoteHost
	err := windows.InstallIIS(host)
	h.suite.Require().NoError(err)
}

func (h *IISHelper) installAspNet() {
	host := h.suite.Env().RemoteHost
	output, err := host.Execute("Install-WindowsFeature Web-Asp-Net45")
	h.suite.Require().NoErrorf(err, "failed to install Asp.Net: %s", output)
}

// StartIISApp creates and starts an IIS application with the provided web.config and index.aspx files.
func (h *IISHelper) StartIISApp(webConfigFile, aspxFile []byte) {
	host := h.suite.Env().RemoteHost
	err := host.MkdirAll("C:\\inetpub\\wwwroot\\DummyApp")
	h.suite.Require().NoError(err, "failed to create directory for DummyApp")
	_, err = host.WriteFile("C:\\inetpub\\wwwroot\\DummyApp\\web.config", webConfigFile)
	h.suite.Require().NoError(err, "failed to write web.config file")
	_, err = host.WriteFile("C:\\inetpub\\wwwroot\\DummyApp\\index.aspx", aspxFile)
	h.suite.Require().NoError(err, "failed to write index.aspx file")
	script := `
$SitePath = "C:\inetpub\wwwroot\DummyApp"
New-WebSite -Name DummyApp -PhysicalPath $SitePath -Port 8080 -ApplicationPool "DefaultAppPool" -Force
Stop-WebSite -Name "DummyApp"
Start-WebSite -Name "DummyApp"
$state = (Get-WebAppPoolState -Name "DefaultAppPool").Value
if ($state -eq "Stopped") {
    Start-WebAppPool -Name "DefaultAppPool"
}
Restart-WebAppPool -Name "DefaultAppPool"
Invoke-WebRequest -Uri "http://localhost:8080/index.aspx" -UseBasicParsing
	`
	output, err := host.Execute(script)
	h.suite.Require().NoErrorf(err, "failed to start site: %s", output)
}

// StopIISApp stops the IIS application and waits for the app pool to stop.
func (h *IISHelper) StopIISApp() {
	script := `
Stop-WebSite -Name "DummyApp"
$state = (Get-WebAppPoolState -Name "DefaultAppPool").Value
if ($state -ne "Stopped") {
	Stop-WebAppPool -Name "DefaultAppPool"
	$retryCount = 0
	do {
		Start-Sleep -Seconds 1
		$status = (Get-WebAppPoolState -Name DefaultAppPool).Value
		$retryCount++
	} while ($status -ne "Stopped" -and $retryCount -lt 60)
	if ($status -ne "Stopped") {
		exit -1
	}
}
	`
	host := h.suite.Env().RemoteHost
	output, err := host.Execute(script)
	h.suite.Require().NoErrorf(err, "failed to stop site: %s", output)
}

// GetLibraryPathFromInstrumentedIIS makes a request to the IIS app and returns the content.
// This is typically used to get the path to the instrumented library from the ASP.NET page.
func (h *IISHelper) GetLibraryPathFromInstrumentedIIS() string {
	host := h.suite.Env().RemoteHost
	output, err := host.Execute(`(Invoke-WebRequest -Uri "http://localhost:8080/index.aspx" -UseBasicParsing).Content`)
	h.suite.Require().NoErrorf(err, "failed to get content from site: %s", output)
	return strings.TrimSpace(output)
}
