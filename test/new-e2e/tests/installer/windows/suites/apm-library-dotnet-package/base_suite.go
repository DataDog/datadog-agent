// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dotnettests contains the E2E tests for the .NET APM Library package.
package dotnettests

import (
	"strings"

	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
)

type baseIISSuite struct {
	installerwindows.BaseSuite
}

func (s *baseIISSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.installIIS()
	s.installAspNet()
}

func (s *baseIISSuite) installIIS() {
	host := s.Env().RemoteHost
	err := windows.InstallIIS(host)
	s.Require().NoError(err)
}

func (s *baseIISSuite) installAspNet() {
	host := s.Env().RemoteHost
	output, err := host.Execute("Install-WindowsFeature Web-Asp-Net45")
	s.Require().NoErrorf(err, "failed to install Asp.Net: %s", output)
}

func (s *baseIISSuite) startIISApp(webConfigFile, aspxFile []byte) {
	host := s.Env().RemoteHost
	err := host.MkdirAll("C:\\inetpub\\wwwroot\\DummyApp")
	s.Require().NoError(err, "failed to create directory for DummyApp")
	_, err = host.WriteFile("C:\\inetpub\\wwwroot\\DummyApp\\web.config", webConfigFile)
	s.Require().NoError(err, "failed to write web.config file")
	_, err = host.WriteFile("C:\\inetpub\\wwwroot\\DummyApp\\index.aspx", aspxFile)
	s.Require().NoError(err, "failed to write index.aspx file")
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
	s.Require().NoErrorf(err, "failed to start site: %s", output)
}

func (s *baseIISSuite) stopIISApp() {
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
	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoErrorf(err, "failed to stop site: %s", output)
}

func (s *baseIISSuite) getLibraryPathFromInstrumentedIIS() string {
	host := s.Env().RemoteHost
	output, err := host.Execute(`(Invoke-WebRequest -Uri "http://localhost:8080/index.aspx" -UseBasicParsing).Content`)
	s.Require().NoErrorf(err, "failed to get content from site: %s", output)
	return strings.TrimSpace(output)
}
