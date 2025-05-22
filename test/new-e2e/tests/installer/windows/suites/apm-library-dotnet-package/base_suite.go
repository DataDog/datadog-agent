// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dotnettests contains the E2E tests for the .NET APM Library package.
package dotnettests

import (
	"fmt"
	"os"
	"path/filepath"
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
# Configure application pool settings
Import-Module WebAdministration
$appPool = Get-Item "IIS:\AppPools\DefaultAppPool"
$appPool.processModel.idleTimeout = [TimeSpan]::FromMinutes(0)
$appPool.processModel.pingingEnabled = $false
$appPool.recycling.periodicRestart.time = [TimeSpan]::FromMinutes(0)
$appPool.recycling.disallowOverlappingRotation = $true
$appPool | Set-Item

# Create and configure the website
$SitePath = "C:\inetpub\wwwroot\DummyApp"
New-WebSite -Name DummyApp -PhysicalPath $SitePath -Port 8080 -ApplicationPool "DefaultAppPool" -Force
Stop-WebSite -Name "DummyApp"
Start-WebSite -Name "DummyApp"

# Ensure application pool is running
$state = (Get-WebAppPoolState -Name "DefaultAppPool").Value
if ($state -eq "Stopped") {
    Start-WebAppPool -Name "DefaultAppPool"
} else {
    Restart-WebAppPool -Name "DefaultAppPool"
}

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
    } while ($status -ne "Stopped" -and $retryCount -lt 30)
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

func (s *baseIISSuite) TestName() string {
	fullName := s.T().Name()
	parts := strings.Split(fullName, "/")
	return parts[len(parts)-1]
}

func (s *baseIISSuite) WriteWASEventLogs(filename string) error {
	output, err := s.Env().RemoteHost.Execute("Get-EventLog -LogName System -Source WAS | Format-List")
	if err != nil {
		return fmt.Errorf("failed to get WAS event logs: %w", err)
	}

	outputPath := filepath.Join(s.GetTestOutputDir(s.TestName()), filename)

	return os.WriteFile(outputPath, []byte(output), 0644)
}

func (s *baseIISSuite) EnableProcessAudit() {
	script := `
AuditPol /Set /Category:"Detailed Tracking" /Subcategory:"Process Creation" /Success:Enable /Failure:Enable
AuditPol /Set /Category:"Detailed Tracking" /Subcategory:"Process Termination" /Success:Enable /Failure:Enable
`
	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoErrorf(err, "failed to enable process audit: %s", output)
}

func (s *baseIISSuite) WriteProcessAuditLogs(filename string) error {
	output, err := s.Env().RemoteHost.Execute(`Get-WinEvent -FilterHashtable @{LogName='Security'; Id=4688, 4689} | ForEach-Object {
    $_ | Select-Object TimeCreated, ProviderName, Id, @{Name="Message"; Expression = {
        $_.Message -replace '(?s)Token Elevation Type:.*', ''
    }}
} | Format-List`)
	if err != nil {
		return fmt.Errorf("failed to get Security event logs: %w", err)
	}

	outputPath := filepath.Join(s.GetTestOutputDir(s.TestName()), filename)

	return os.WriteFile(outputPath, []byte(output), 0644)
}

func (s *baseIISSuite) EnableIISConfigurationLog() {
	script := `wevtutil sl Microsoft-IIS-Configuration/Operational /e:true`
	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoErrorf(err, "failed to enable IIS Configuration log: %s", output)
}

func (s *baseIISSuite) WriteIISConfigurationLogs(filename string) error {
	output, err := s.Env().RemoteHost.Execute("Get-WinEvent -LogName Microsoft-IIS-Configuration/Operational -ErrorAction SilentlyContinue | Format-List")
	if err != nil && !strings.Contains(err.Error(), "NoMatchingEventsFound") {
		return fmt.Errorf("failed to get IIS Configuration logs: %w", err)
	}

	outputPath := filepath.Join(s.GetTestOutputDir(s.TestName()), filename)

	return os.WriteFile(outputPath, []byte(output), 0644)
}

func (s *baseIISSuite) WriteSvcHostTasks(filename string) error {
	output, err := s.Env().RemoteHost.Execute("tasklist /svc /fi \"imagename eq svchost.exe\"")
	if err != nil {
		return fmt.Errorf("failed to get svchost tasks: %w", err)
	}

	outputPath := filepath.Join(s.GetTestOutputDir(s.TestName()), filename)
	return os.WriteFile(outputPath, []byte(output), 0644)
}

// EnableFileAudit enables auditing on the IIS ApplicationHost.config file
func (s *baseIISSuite) EnableAppHostFileAudit() {
	script := `
AuditPol /set /subcategory:"File System" /success:enable /failure:enable
gpupdate /force

# Target file
$file = "$env:windir\System32\inetsrv\config\ApplicationHost.config"

# Get the current ACL
$acl = Get-Acl $file

# Define the user or group for auditing
$user = New-Object System.Security.Principal.NTAccount("Everyone")

# Define audit flags (Success + Failure)
$auditFlags = [System.Security.AccessControl.AuditFlags]::Success ` + "`" + `
             -bor [System.Security.AccessControl.AuditFlags]::Failure

# Define file system rights to audit (not as an array!)
$auditRights = [System.Security.AccessControl.FileSystemRights]::ReadData ` + "`" + `
             -bor [System.Security.AccessControl.FileSystemRights]::WriteData ` + "`" + `
             -bor [System.Security.AccessControl.FileSystemRights]::Delete ` + "`" + `
             -bor [System.Security.AccessControl.FileSystemRights]::ReadPermissions

# Create the audit rule
$auditRule = New-Object System.Security.AccessControl.FileSystemAuditRule (
    $user,
    $auditRights,
    [System.Security.AccessControl.InheritanceFlags]::None,
    [System.Security.AccessControl.PropagationFlags]::None,
    $auditFlags
)

# Apply the audit rule
$acl.AddAuditRule($auditRule)
Set-Acl -Path $file -AclObject $acl
`

	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoErrorf(err, "failed to enable app host file audit: %s", output)
}

func (s *baseIISSuite) WriteAppHostFileAuditLogs(filename string) error {
	script := `Get-WinEvent -FilterHashtable @{
    LogName = 'Security';
    Id = 4663
} -MaxEvents 1000 |
    Where-Object { $_.Message -like "*ApplicationHost.config*" } |
    Format-List TimeCreated, Message`

	output, err := s.Env().RemoteHost.Execute(script)
	if err != nil {
		fmt.Printf("failed to get app host file audit logs: %s\n%s", err, output)
		return fmt.Errorf("failed to get app host file audit logs: %w", err)
	}

	outputPath := filepath.Join(s.GetTestOutputDir(s.TestName()), filename)
	return os.WriteFile(outputPath, []byte(output), 0644)
}

// EnableETWTrace enables ETW tracing for kernel process and file events
func (s *baseIISSuite) EnableETWTrace() {
	script := `@"
Microsoft-Windows-Kernel-Process 0xFF 0x5
Microsoft-Windows-Kernel-File 0xFF 0x5
Microsoft-Windows-IIS-Configuration 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-APPHOSTSVC 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-FTP 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-IisMetabaseAudit 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-IISReset 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-Logging 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-W3SVC 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-W3SVC-PerfCounters 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-W3SVC-WP 0xFFFFFFFFFFFFFFFF 255
Microsoft-Windows-IIS-WMSVC 0xFFFFFFFFFFFFFFFF 255
"@ | Out-File -Encoding ASCII -FilePath .\providers.pf

logman create trace MyKernelTrace -pf .\providers.pf -o $env:TEMP\MyKernelTrace.etl -ets`

	output, err := s.Env().RemoteHost.Execute(script)
	s.Require().NoErrorf(err, "failed to enable ETW tracing: %s", output)
}

// StopETWTrace stops ETW tracing and saves the trace file
func (s *baseIISSuite) StopETWTrace() {
	output, err := s.Env().RemoteHost.Execute("logman stop MyKernelTrace -ets")
	s.Require().NoErrorf(err, "failed to stop ETW tracing: %s", output)

	output, err = s.Env().RemoteHost.Execute(`tracerpt -y "$env:TEMP\MyKernelTrace.etl" -o "$env:TEMP\report.csv" -of CSV`)
	s.Require().NoErrorf(err, "failed to convert ETW trace to CSV: %s", output)

	tempDir, err := s.Env().RemoteHost.Execute("echo $env:TEMP")
	tempDir = strings.TrimSpace(tempDir)
	s.Require().NoErrorf(err, "failed to get temp directory: %s", err)

	reportPath := filepath.Join(s.GetTestOutputDir(s.TestName()), "report.csv")
	tracePath := filepath.Join(s.GetTestOutputDir(s.TestName()), "MyKernelTrace.etl")

	err = s.Env().RemoteHost.GetFile(pathJoin(tempDir, "MyKernelTrace.etl"), tracePath)
	s.Require().NoErrorf(err, "failed to get ETW trace file: %s", err)
	err = s.Env().RemoteHost.GetFile(pathJoin(tempDir, "report.csv"), reportPath)
	s.Require().NoErrorf(err, "failed to get ETW trace report: %s", err)
}

func (s *baseIISSuite) StopTrustedInstaller() {
	output, err := s.Env().RemoteHost.Execute("sc stop TrustedInstaller")
	s.Require().NoErrorf(err, "failed to stop TrustedInstaller: %s", output)
}

// DisableRotationOnConfigChange disables recycling of application pools when configuration changes
func (s *baseIISSuite) DisableRotationOnConfigChange() {
	script := `
Import-Module WebAdministration
Get-ChildItem IIS:\AppPools | ForEach-Object {
    $_.recycling.disallowRotationOnConfigChange = $true
    $_ | Set-Item
}
`
	output, err := s.Env().RemoteHost.Execute(script)
	s.Require().NoErrorf(err, "failed to disable rotation on config change: %s", output)
}
