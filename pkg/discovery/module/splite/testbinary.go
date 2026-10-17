// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && linux

package splite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// StartTestBinary runs the system-probe-lite binary built in the source tree
// with cfg until the test ends, and returns once its socket accepts
// connections. It skips the test on platforms where the binary can't run.
func StartTestBinary(t *testing.T, cfg Config) {
	t.Helper()

	// CentOS 7 arm64 is not a supported platform (arm64 support starts at CentOS 8)
	// and the binary requires GLIBC_2.18 which is not available on CentOS 7 (glibc 2.17).
	if runtime.GOARCH == "arm64" {
		platform, err := kernel.Platform()
		require.NoError(t, err)
		platformVersion, err := kernel.PlatformVersion()
		require.NoError(t, err)
		if platform == "centos" && strings.HasPrefix(platformVersion, "7") {
			t.Skip("system-probe-lite requires GLIBC_2.18 on arm64; CentOS 7 (glibc 2.17) is unsupported on arm64")
		}
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	binaryPath := filepath.Join(curDir, "..", "rust", "embedded", "bin", "system-probe-lite")
	require.FileExists(t, binaryPath, "system-probe-lite binary should be built")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath, cfg.Args()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	// system-probe-lite sets the socket mode once the socket listens.
	require.Eventually(t, func() bool {
		info, err := os.Stat(cfg.Socket)
		return err == nil && info.Mode().Perm() == 0720
	}, 10*time.Second, 50*time.Millisecond, "system-probe-lite socket did not become ready")
}
