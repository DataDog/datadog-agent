// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	installerLSASecretKey = "L$datadog_ddagentuser_password"
	scmAgentPasswordKey   = "_SC_datadogagent"
	datadogAgentService   = "datadogagent"
)

type agentSpawnCredentialDiagnostics struct {
	InstalledUser                     string `json:"InstalledUser"`
	InstalledDomain                   string `json:"InstalledDomain"`
	DatadogAgentServiceAccount        string `json:"DatadogAgentServiceAccount"`
	RegistryUserSid                   string `json:"RegistryUserSid"`
	ServiceAccountSid                 string `json:"ServiceAccountSid"`
	ServiceAccountMatchesRegistryUser bool   `json:"ServiceAccountMatchesRegistryUser"`
	InstallerLSASecretPresent         bool   `json:"InstallerLSASecretPresent"`
	ScmLSASecretPresent               bool   `json:"ScmLSASecretPresent"`
	ScmLSASecretLength                int    `json:"ScmLSASecretLength"`
	ProcmgrLogTail                    string `json:"ProcmgrLogTail"`
}

func joinRemoteWindowsPath(base string, elems ...string) string {
	parts := make([]string, 0, len(elems)+1)
	parts = append(parts, strings.TrimRight(strings.ReplaceAll(base, `\`, `/`), `/`))
	parts = append(parts, elems...)
	return strings.Join(parts, "/")
}

func procmgrCLIBin(host *components.RemoteHost) (string, error) {
	installPath, err := windowsAgent.GetInstallPathFromRegistry(host)
	if err != nil {
		return "", err
	}
	return joinRemoteWindowsPath(installPath, "bin", "agent", "dd-procmgr.exe"), nil
}

func describeField(output, label string) string {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) {
			return strings.TrimSpace(trimmed[len(needle):])
		}
	}
	return ""
}

func assertInstallerLSASecretAbsent(host *components.RemoteHost) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
if (-not ('Datadog.LsaUtil' -as [type])) {
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Datadog {
  public static class LsaUtil {
    [StructLayout(LayoutKind.Sequential)]
    public struct LsaUnicodeString {
      public ushort Length;
      public ushort MaximumLength;
      public IntPtr Buffer;
    }
    [StructLayout(LayoutKind.Sequential)]
    public struct LsaObjectAttributes {
      public int Length;
      public IntPtr RootDirectory;
      public LsaUnicodeString ObjectName;
      public uint Attributes;
      public IntPtr SecurityDescriptor;
      public IntPtr SecurityQualityOfService;
    }
    [DllImport("advapi32.dll", PreserveSig=true)]
    public static extern uint LsaOpenPolicy(ref LsaUnicodeString systemName, ref LsaObjectAttributes objectAttributes, int accessMask, out IntPtr policyHandle);
    [DllImport("advapi32.dll", PreserveSig=true)]
    public static extern uint LsaRetrievePrivateData(IntPtr policyHandle, ref LsaUnicodeString keyName, out IntPtr privateData);
    [DllImport("advapi32.dll")]
    public static extern uint LsaClose(IntPtr objectHandle);
  }
}
'@
}
$systemName = New-Object Datadog.LsaUtil+LsaUnicodeString
$objectAttributes = New-Object Datadog.LsaUtil+LsaObjectAttributes
$policy = [IntPtr]::Zero
$status = [Datadog.LsaUtil]::LsaOpenPolicy([ref]$systemName, [ref]$objectAttributes, 4, [ref]$policy)
if ($status -ne 0) { throw "LsaOpenPolicy failed: 0x$($status.ToString('X8'))" }
$keyBuffer = [IntPtr]::Zero
try {
  $key = '%s'
  $bytes = [System.Text.Encoding]::Unicode.GetBytes($key)
  $keyBuffer = [System.Runtime.InteropServices.Marshal]::AllocHGlobal($bytes.Length)
  [System.Runtime.InteropServices.Marshal]::Copy($bytes, 0, $keyBuffer, $bytes.Length)
  $lsaKey = New-Object Datadog.LsaUtil+LsaUnicodeString
  $lsaKey.Length = [uint16]($bytes.Length - 2)
  $lsaKey.MaximumLength = [uint16]$bytes.Length
  $lsaKey.Buffer = $keyBuffer
  $secret = [IntPtr]::Zero
  $status = [Datadog.LsaUtil]::LsaRetrievePrivateData($policy, [ref]$lsaKey, [ref]$secret)
  if ($status.ToString('X8') -eq 'C0000034') { exit 0 }
  if ($status -ne 0) { throw "LsaRetrievePrivateData failed: 0x$($status.ToString('X8'))" }
  throw "installer LSA secret should be absent on pre-LSA no-password upgrade"
} finally {
  if ($keyBuffer -ne [IntPtr]::Zero) { [System.Runtime.InteropServices.Marshal]::FreeHGlobal($keyBuffer) }
  if ($policy -ne [IntPtr]::Zero) { [void][Datadog.LsaUtil]::LsaClose($policy) }
}
`, installerLSASecretKey)

	_, err := host.Execute(script)
	return err
}

func collectAgentSpawnCredentialDiagnostics(host *components.RemoteHost) (*agentSpawnCredentialDiagnostics, error) {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
if (-not ('Datadog.LsaUtil' -as [type])) {
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Datadog {
  public static class LsaUtil {
    [StructLayout(LayoutKind.Sequential)]
    public struct LsaUnicodeString {
      public ushort Length;
      public ushort MaximumLength;
      public IntPtr Buffer;
    }
    [StructLayout(LayoutKind.Sequential)]
    public struct LsaObjectAttributes {
      public int Length;
      public IntPtr RootDirectory;
      public LsaUnicodeString ObjectName;
      public uint Attributes;
      public IntPtr SecurityDescriptor;
      public IntPtr SecurityQualityOfService;
    }
    [DllImport("advapi32.dll", PreserveSig=true)]
    public static extern uint LsaOpenPolicy(ref LsaUnicodeString systemName, ref LsaObjectAttributes objectAttributes, int accessMask, out IntPtr policyHandle);
    [DllImport("advapi32.dll", PreserveSig=true)]
    public static extern uint LsaRetrievePrivateData(IntPtr policyHandle, ref LsaUnicodeString keyName, out IntPtr privateData);
    [DllImport("advapi32.dll")]
    public static extern uint LsaClose(IntPtr objectHandle);
    [DllImport("advapi32.dll")]
    public static extern uint LsaFreeMemory(IntPtr buffer);
  }
}
'@
}

function Get-LsaSecretLength([string]$key) {
  $systemName = New-Object Datadog.LsaUtil+LsaUnicodeString
  $objectAttributes = New-Object Datadog.LsaUtil+LsaObjectAttributes
  $policy = [IntPtr]::Zero
  $status = [Datadog.LsaUtil]::LsaOpenPolicy([ref]$systemName, [ref]$objectAttributes, 4, [ref]$policy)
  if ($status -ne 0) { throw "LsaOpenPolicy failed: 0x$($status.ToString('X8'))" }
  $keyBuffer = [IntPtr]::Zero
  try {
    $bytes = [System.Text.Encoding]::Unicode.GetBytes($key)
    $keyBuffer = [System.Runtime.InteropServices.Marshal]::AllocHGlobal($bytes.Length)
    [System.Runtime.InteropServices.Marshal]::Copy($bytes, 0, $keyBuffer, $bytes.Length)
    $lsaKey = New-Object Datadog.LsaUtil+LsaUnicodeString
    $lsaKey.Length = [uint16]($bytes.Length - 2)
    $lsaKey.MaximumLength = [uint16]$bytes.Length
    $lsaKey.Buffer = $keyBuffer
    $secret = [IntPtr]::Zero
    $status = [Datadog.LsaUtil]::LsaRetrievePrivateData($policy, [ref]$lsaKey, [ref]$secret)
    if ($status.ToString('X8') -eq 'C0000034') { return 0 }
    if ($status -ne 0) { throw "LsaRetrievePrivateData($key) failed: 0x$($status.ToString('X8'))" }
    if ($secret -eq [IntPtr]::Zero) { return 0 }
    $secretRef = [Runtime.InteropServices.Marshal]::PtrToStructure($secret, [type][Datadog.LsaUtil+LsaUnicodeString])
    return [int]($secretRef.Length / 2)
  } finally {
    if ($keyBuffer -ne [IntPtr]::Zero) { [System.Runtime.InteropServices.Marshal]::FreeHGlobal($keyBuffer) }
    if ($policy -ne [IntPtr]::Zero) { [void][Datadog.LsaUtil]::LsaClose($policy) }
  }
}

function Get-AccountSid([string]$domain, [string]$user) {
  if ([string]::IsNullOrWhiteSpace($domain)) {
    return (New-Object System.Security.Principal.NTAccount($user)).Translate([System.Security.Principal.SecurityIdentifier]).Value
  }
  return (New-Object System.Security.Principal.NTAccount("$domain\$user")).Translate([System.Security.Principal.SecurityIdentifier]).Value
}

function Get-ServiceAccountSid([string]$serviceAccount) {
  if ($serviceAccount -eq 'LocalSystem') {
    return (New-Object System.Security.Principal.SecurityIdentifier('S-1-5-18')).Value
  }
  $parts = $serviceAccount -split '\\', 2
  if ($parts.Count -eq 2 -and $parts[0] -eq '.') {
    return (New-Object System.Security.Principal.NTAccount($parts[1])).Translate([System.Security.Principal.SecurityIdentifier]).Value
  }
  return (New-Object System.Security.Principal.NTAccount($serviceAccount)).Translate([System.Security.Principal.SecurityIdentifier]).Value
}

$registryPath = '%s'
$installedUser = (Get-ItemPropertyValue -Path $registryPath -Name 'installedUser')
$installedDomain = (Get-ItemPropertyValue -Path $registryPath -Name 'installedDomain')
$serviceAccount = (Get-WmiObject Win32_Service -Filter "Name='%s'").StartName
$registrySid = Get-AccountSid $installedDomain $installedUser
$serviceSid = Get-ServiceAccountSid $serviceAccount
$installerSecretLength = Get-LsaSecretLength '%s'
$scmSecretLength = Get-LsaSecretLength '%s'
$procmgrLog = Join-Path $env:ProgramData 'Datadog\logs\dd-procmgr.log'
$procmgrLogTail = ''
if (Test-Path $procmgrLog) {
  $procmgrLogTail = ((Get-Content -LiteralPath $procmgrLog -Tail 80) -join [Environment]::NewLine)
}
@{
  InstalledUser = $installedUser
  InstalledDomain = $installedDomain
  DatadogAgentServiceAccount = $serviceAccount
  RegistryUserSid = $registrySid
  ServiceAccountSid = $serviceSid
  ServiceAccountMatchesRegistryUser = ($registrySid -eq $serviceSid)
  InstallerLSASecretPresent = ($installerSecretLength -gt 0)
  ScmLSASecretPresent = ($scmSecretLength -gt 0)
  ScmLSASecretLength = $scmSecretLength
  ProcmgrLogTail = $procmgrLogTail
} | ConvertTo-Json -Compress
`, windowsAgent.RegistryKeyPath, datadogAgentService, installerLSASecretKey, scmAgentPasswordKey)

	out, err := host.Execute(script)
	if err != nil {
		return nil, err
	}
	var diagnostics agentSpawnCredentialDiagnostics
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &diagnostics); err != nil {
		return nil, fmt.Errorf("parse spawn credential diagnostics: %w; raw output: %s", err, out)
	}
	return &diagnostics, nil
}

func logAgentSpawnCredentialDiagnostics(host *components.RemoteHost, logf func(format string, args ...any)) {
	diagnostics, err := collectAgentSpawnCredentialDiagnostics(host)
	if err != nil {
		logf("agent spawn credential diagnostics failed: %v", err)
		return
	}
	logf("agent spawn credential diagnostics: installed=%s\\%s datadogagent_service_account=%q registry_user_sid=%s service_account_sid=%s service_account_matches_registry_user=%t installer_lsa_secret_present=%t scm_lsa_secret_present=%t scm_lsa_secret_length=%d",
		diagnostics.InstalledDomain,
		diagnostics.InstalledUser,
		diagnostics.DatadogAgentServiceAccount,
		diagnostics.RegistryUserSid,
		diagnostics.ServiceAccountSid,
		diagnostics.ServiceAccountMatchesRegistryUser,
		diagnostics.InstallerLSASecretPresent,
		diagnostics.ScmLSASecretPresent,
		diagnostics.ScmLSASecretLength,
	)
	if tail := strings.TrimSpace(diagnostics.ProcmgrLogTail); tail != "" {
		logf("dd-procmgr.log tail:\n%s", tail)
	} else {
		logf("dd-procmgr.log tail: (missing or empty)")
	}
}

func windowsProcessOwnerByPID(host *components.RemoteHost, pid string) (string, error) {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$p = Get-CimInstance Win32_Process -Filter "ProcessId=%s"
if ($null -eq $p) { exit 1 }
$o = Invoke-CimMethod -InputObject $p -MethodName GetOwner
if ($o.ReturnValue -ne 0) { exit $o.ReturnValue }
"$($o.Domain)/$($o.User)"
`, pid)
	out, err := host.Execute(script)
	return strings.TrimSpace(out), err
}

func (suite *testUpgradeWithoutStoredPasswordSuite) assertAgentProfileSpawnWithoutInstallerLSAPassword(
	host *components.RemoteHost,
	expectedUser string,
) {
	cliBin, err := procmgrCLIBin(host)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(cliBin)

	err = assertInstallerLSASecretAbsent(host)
	suite.Require().NoError(err, "installer LSA secret must be absent after 7.65 -> current upgrade without password")

	logAgentSpawnCredentialDiagnostics(host, suite.T().Logf)

	procName := fmt.Sprintf("e2e-pre-lsa-spawn-%d", time.Now().UnixNano())
	createCmd := fmt.Sprintf(
		`& '%s' create --name=%s --command='C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' `+
			`--args=-NoProfile --args=-NonInteractive --args=-Command --args='Start-Sleep -Seconds 3600' `+
			`--env=SystemRoot=C:\Windows --env='PATH=C:\Windows\System32;C:\Windows' `+
			`--restart-policy=always --description='pre-LSA agent-profile spawn check'`,
		cliBin, procName,
	)
	createOut, err := host.Execute(createCmd)
	suite.Require().NoError(err,
		"agent-profile create should succeed using SCM-stored datadogagent credentials; output: %s", createOut)
	suite.Assert().Contains(createOut, "UUID:")

	var loggedFailedDiagnostics bool
	suite.EventuallyWithT(func(c *assert.CollectT) {
		describeOut, err := host.Execute(fmt.Sprintf(`& '%s' describe %s`, cliBin, procName))
		assert.NoError(c, err, "describe output: %s", describeOut)

		state := describeField(describeOut, "State")
		if state == "Failed" && !loggedFailedDiagnostics {
			loggedFailedDiagnostics = true
			suite.T().Logf("describe output after spawn failure:\n%s", describeOut)
			logAgentSpawnCredentialDiagnostics(host, suite.T().Logf)
		}

		assert.Equal(c, "Running", state,
			"agent-profile child should reach Running without installer LSA password")
		assert.Equal(c, "agent", describeField(describeOut, "Profile"))
		assert.Equal(c, expectedUser, describeField(describeOut, "User"))
		assert.Equal(c, expectedUser, describeField(describeOut, "Runtime User"))

		pid := describeField(describeOut, "PID")
		if !assert.NotEmpty(c, pid) || !assert.NotEqual(c, "-", pid) {
			return
		}
		owner, err := windowsProcessOwnerByPID(host, pid)
		assert.NoError(c, err)
		assert.NotContains(c, owner, "NT AUTHORITY/SYSTEM",
			"agent-profile child must not run as LocalSystem")
		assert.Contains(c, strings.ToUpper(owner), strings.ToUpper(TestUser),
			"process owner should be the domain Agent user")
	}, 2*time.Minute, 2*time.Second)
}
