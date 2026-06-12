// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddot

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// AssertDDOTManagedByProcmgrWindows verifies the OCI DDOT extension process is supervised
// by dd-procmgrd on a Windows host (processes.d + dd-procmgr describe), not only that
// dd-procmgr-service is running.
func AssertDDOTManagedByProcmgrWindows(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	cfg := filepath.Join(installRoot, "processes.d", procmgrConfigName)

	requireRemoteLiteralPath(t, host, cli, "dd-procmgr CLI")
	requireRemoteLiteralPath(t, host, cfg, "DDOT procmgr config")

	waitForProcmgrCLIWindows(t, host, cli)
	waitProcmgrDDOTDescribeRunningStable(t, host, psProcmgr(cli, "describe "+procmgrProcessName))
}

func requireRemoteLiteralPath(t *testing.T, host *components.RemoteHost, path, description string) {
	t.Helper()
	_, err := host.Execute(psLiteralPathExists(path))
	require.NoError(t, err, "%s should exist at %s", description, path)
}

func psLiteralPathExists(path string) string {
	return fmt.Sprintf(
		`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`,
		psEscapeSingleQuoted(path),
	)
}

// psProcmgr runs a dd-procmgr subcommand (e.g. "status", "describe datadog-agent-ddot").
func psProcmgr(cliExe, invocation string) string {
	return fmt.Sprintf(
		`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; & '%s' %s"`,
		psEscapeSingleQuoted(cliExe),
		invocation,
	)
}

func psEscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

func waitForProcmgrCLIWindows(t *testing.T, host *components.RemoteHost, cli string) {
	t.Helper()
	cmd := psProcmgr(cli, "status")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Execute(cmd)
		assert.NoError(c, err, "dd-procmgr CLI not reachable")
	}, 2*time.Minute, 2*time.Second)
}
