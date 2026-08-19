// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2ecomponents "github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// writeProcessesDYamlContent writes a processes.d YAML file as UTF-8 without a BOM.
// Set-Content -Encoding utf8 adds a BOM on Windows PowerShell 5.1, which breaks dd-procmgr parsing.
func writeProcessesDYamlContent(yamlPath, content string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	return psRemote(
		`$ErrorActionPreference='Stop'; $p='%s'; $b = [Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes($p, $b)`,
		yamlPath, b64,
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
