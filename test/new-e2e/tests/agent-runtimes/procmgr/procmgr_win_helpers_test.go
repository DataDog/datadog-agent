// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2ecomponents "github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// psRemote builds a PowerShell script for RemoteHost.Execute; string args are escaped for single-quoted literals.
func psRemote(format string, args ...any) string {
	for i, a := range args {
		if s, ok := a.(string); ok {
			args[i] = escapePSSingleQuotedLiteral(s)
		}
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func escapePSSingleQuotedLiteral(s string) string {
	s = strings.ReplaceAll(s, `%`, `%%`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}

// Path helpers for remote PowerShell: the e2e runner is Linux/macOS, so registry paths
// need explicit slash normalization instead of filepath.Join.
func toWindowsSlashPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

func joinWindowsPath(base string, elems ...string) string {
	parts := make([]string, 0, len(elems)+1)
	parts = append(parts, strings.TrimRight(toWindowsSlashPath(base), `/`))
	parts = append(parts, elems...)
	return strings.Join(parts, "/")
}

// normalizeWindowsPathForCompare lowercases and canonicalizes Windows paths/command lines
// read from WMI so comparisons are stable across slash styles and registry trailing slashes.
func normalizeWindowsPathForCompare(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, `"`)
	s = strings.ReplaceAll(s, "/", `\`)
	for strings.Contains(s, `\\`) {
		s = strings.ReplaceAll(s, `\\`, `\`)
	}
	return s
}

func ensureWindowsDirPS(dir string) string {
	return psRemote(`New-Item -ItemType Directory -Force -Path '%s' | Out-Null`, dir)
}

func psLiteralPathExists(path string) string {
	return psRemote(`if (-not (Test-Path -LiteralPath '%s')) { exit 1 }`, path)
}

func psLegacySCMServiceMustNotBeRunning(serviceName string) string {
	return psRemote(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 0 }; if ($s.Status -eq 'Running') { exit 1 }; exit 0`,
		serviceName,
	)
}

func psProcessExistsByName(name string) string {
	return psRemote(
		`$p = Get-CimInstance Win32_Process -Filter "Name='%s'"; if ($null -eq $p) { exit 1 } else { exit 0 }`,
		name,
	)
}

func psProcessAbsentByName(name string) string {
	return psRemote(
		`$p = Get-CimInstance Win32_Process -Filter "Name='%s'"; if ($null -eq $p) { exit 0 } else { exit 1 }`,
		name,
	)
}

// replaceProcessesDYaml applies old→new in processes.d YAML without a UTF-8 BOM.
// Set-Content -Encoding utf8 adds a BOM on Windows PowerShell 5.1, which breaks dd-procmgr parsing.
func replaceProcessesDYaml(yamlPath, old, new string) string {
	return psRemote(
		`$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $o=$c; $c=$c.Replace('%s','%s'); if ($o -eq $c) { exit 1 }; $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`,
		yamlPath, old, new,
	)
}

func windowsProcessOwnerByName(host *e2ecomponents.RemoteHost, name string) (string, error) {
	script := psRemote(
		`$p = Get-CimInstance Win32_Process -Filter "Name='%s'" | Select-Object -First 1; if ($null -eq $p) { exit 1 }; $o = Invoke-CimMethod -InputObject $p -MethodName GetOwner; if ($o.ReturnValue -ne 0) { exit $o.ReturnValue }; "$($o.Domain)/$($o.User)"`,
		name,
	)
	out, err := host.Execute(script)
	return strings.TrimSpace(out), err
}

func windowsProcessOwnerByPID(host *e2ecomponents.RemoteHost, pid string) (string, error) {
	script := psRemote(
		`$p = Get-CimInstance Win32_Process -Filter "ProcessId=%s"; if ($null -eq $p) { exit 1 }; $o = Invoke-CimMethod -InputObject $p -MethodName GetOwner; if ($o.ReturnValue -ne 0) { exit $o.ReturnValue }; "$($o.Domain)/$($o.User)"`,
		pid,
	)
	out, err := host.Execute(script)
	return strings.TrimSpace(out), err
}

func waitProcmgrDescribeRunning(
	t *testing.T,
	host *e2ecomponents.RemoteHost,
	describeCmd string,
	timeout time.Duration,
	commandContains ...string,
) string {
	t.Helper()
	var pid string
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		out, err := host.Execute(describeCmd)
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p) || !assert.NotEqual(ct, "-", p) {
			return
		}
		cmd := fieldValue(out, "Command")
		for _, fragment := range commandContains {
			assert.Contains(ct, strings.ToLower(cmd), strings.ToLower(fragment))
		}
		pid = p
	}, timeout, 2*time.Second)
	return pid
}

func assertPrivilegedCatalogRejection(
	ct *assert.CollectT,
	host *e2ecomponents.RemoteHost,
	cli string,
	processName string,
) {
	ct.Helper()

	_, err := host.Execute(psProcessAbsentByName("process-agent.exe"))
	assert.NoError(ct, err)

	out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processName))
	assert.NoError(ct, err)
	assertField(ct, out, "State", "Failed")
}

func assertReloadAfterDescriptionChange(
	t *testing.T,
	host *e2ecomponents.RemoteHost,
	yamlPath string,
	processName string,
	reloadCmd string,
	describeCmd string,
	originalLine string,
	e2eLine string,
	originalPID string,
) {
	t.Helper()

	t.Cleanup(func() {
		_, _ = host.Execute(replaceProcessesDYaml(yamlPath, e2eLine, originalLine))
		_, _ = host.Execute(reloadCmd)
	})

	host.MustExecute(replaceProcessesDYaml(yamlPath, originalLine, e2eLine))

	reloadOut, err := host.Execute(reloadCmd)
	require.NoError(t, err)
	assert.Contains(t, reloadOut, processName, "reload output: %s", reloadOut)
	assert.Contains(t, reloadOut, "Modified", "reload output: %s", reloadOut)

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		out, err := host.Execute(describeCmd)
		assert.NoError(ct, err)
		assertField(ct, out, "State", "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p) || !assert.NotEqual(ct, "-", p) {
			return
		}
		assert.NotEqual(ct, originalPID, p, "%s should respawn with a new PID after reload", processName)
	}, 90*time.Second, 2*time.Second)

	out, err := host.Execute(describeCmd)
	require.NoError(t, err)
	assertField(t, out, "Description", "E2E-reload-after-yaml")
}
