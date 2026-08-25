// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	fleethost "github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/host"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const installerLSASecretKey = "L$datadog_ddagentuser_password"

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

	fleetHost := fleethost.New(&environments.Host{RemoteHost: host})
	fleetHost.AssertProcessRunning(suite.T(), procName, "")

	info, ok, err := fleetHost.DescribeProcess(procName, "")
	suite.Require().NoError(err)
	suite.Require().True(ok)
	suite.Assert().Equal("agent", info.Profile)
	suite.Assert().Equal(expectedUser, info.User)
	suite.Assert().Equal(expectedUser, info.RuntimeUser)
	suite.Require().NotZero(info.PID)

	owner, err := windowsProcessOwnerByPID(host, strconv.Itoa(info.PID))
	suite.Require().NoError(err)
	suite.Assert().NotContains(owner, "NT AUTHORITY/SYSTEM",
		"agent-profile child must not run as LocalSystem")
	suite.Assert().Contains(strings.ToUpper(owner), strings.ToUpper(TestUser),
		"process owner should be the domain Agent user")
}
