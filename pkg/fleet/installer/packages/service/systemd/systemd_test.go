// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package systemd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSystemctlWithStaleSysVScript mimics systemctl on a RHEL host where a stale
// /etc/init.d script makes systemd-sysv-install (chkconfig) fail enable/disable
// unless SYSTEMCTL_SKIP_SYSV is set.
const fakeSystemctlWithStaleSysVScript = `#!/bin/sh
echo "$* SYSTEMCTL_SKIP_SYSV=$SYSTEMCTL_SKIP_SYSV" >> "$FAKE_SYSTEMCTL_LOG"
case "$1" in
is-enabled) exit 0 ;;
enable|disable)
	if [ "$SYSTEMCTL_SKIP_SYSV" != "1" ]; then
		echo "service datadog-agent does not support chkconfig" >&2
		exit 1
	fi
	;;
esac
exit 0
`

func installFakeSystemctl(t *testing.T) (logPath string) {
	t.Helper()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(fakeSystemctlWithStaleSysVScript), 0755))
	logPath = filepath.Join(t.TempDir(), "systemctl.log")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_SYSTEMCTL_LOG", logPath)
	t.Setenv("SYSTEMCTL_SKIP_SYSV", "")
	return logPath
}

func TestDisableUnitSkipsSysVSync(t *testing.T) {
	logPath := installFakeSystemctl(t)

	require.NoError(t, DisableUnit(context.Background(), "datadog-agent.service"))

	calls, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(calls), "disable --force datadog-agent.service SYSTEMCTL_SKIP_SYSV=1\n")
}
