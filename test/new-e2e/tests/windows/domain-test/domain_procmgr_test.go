// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
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

func assertInstallerLSASecretAbsent(c *assert.CollectT, host *components.RemoteHost) {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
Add-Type -Namespace Datadog -Name LsaUtil -MemberDefinition @'
using System;
using System.Runtime.InteropServices;
public static class LsaUtil {
  [StructLayout(LayoutKind.Sequential)]
  public struct LsaUnicodeString {
    public ushort Length;
    public ushort MaximumLength;
    public IntPtr Buffer;
  }
  [DllImport("advapi32.dll", SetLastError=true)]
  public static extern uint LsaOpenPolicy(IntPtr systemName, IntPtr objectAttributes, uint accessMask, out IntPtr policyHandle);
  [DllImport("advapi32.dll")]
  public static extern uint LsaRetrievePrivateData(IntPtr policyHandle, ref LsaUnicodeString keyName, out IntPtr privateData);
  [DllImport("advapi32.dll")]
  public static extern uint LsaClose(IntPtr objectHandle);
}
'@
$policy = [IntPtr]::Zero
$status = [Datadog.LsaUtil]::LsaOpenPolicy([IntPtr]::Zero, [IntPtr]::Zero, 4, [ref]$policy)
if ($status -ne 0) { throw "LsaOpenPolicy failed: 0x$($status.ToString('X8'))" }
try {
  $key = '%s'
  $bytes = [System.Text.Encoding]::Unicode.GetBytes($key)
  $buffer = [System.Runtime.InteropServices.Marshal]::AllocHGlobal($bytes.Length)
  [System.Runtime.InteropServices.Marshal]::Copy($bytes, 0, $buffer, $bytes.Length)
  $lsaKey = New-Object Datadog.LsaUtil+LsaUnicodeString
  $lsaKey.Length = [uint16]($bytes.Length - 2)
  $lsaKey.MaximumLength = [uint16]$bytes.Length
  $lsaKey.Buffer = $buffer
  $secret = [IntPtr]::Zero
  $status = [Datadog.LsaUtil]::LsaRetrievePrivateData($policy, [ref]$lsaKey, [ref]$secret)
  if ($status -eq 0xC0000034) { exit 0 }
  if ($status -ne 0) { throw "LsaRetrievePrivateData failed: 0x$($status.ToString('X8'))" }
  throw "installer LSA secret should be absent on pre-LSA no-password upgrade"
} finally {
  if ($policy -ne [IntPtr]::Zero) { [void][Datadog.LsaUtil]::LsaClose($policy) }
}
`, installerLSASecretKey)

	_, err := host.Execute(script)
	assert.NoError(c, err, "installer LSA secret must be absent after 7.65 -> current upgrade without password")
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

	suite.EventuallyWithT(func(c *assert.CollectT) {
		assertInstallerLSASecretAbsent(c, host)
	}, time.Minute, 2*time.Second)

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

	suite.EventuallyWithT(func(c *assert.CollectT) {
		describeOut, err := host.Execute(fmt.Sprintf(`& '%s' describe %s`, cliBin, procName))
		assert.NoError(c, err, "describe output: %s", describeOut)

		assert.Equal(c, "Running", describeField(describeOut, "State"),
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
