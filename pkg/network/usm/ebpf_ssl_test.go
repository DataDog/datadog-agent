// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/stretchr/testify/require"
)

func startTest(t *testing.T, arch string) *exec.Cmd {
	cfg := config.New()
	cfg.EnableNativeTLSMonitoring = true

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "testutil", "libmmap")
	bin, err := usmtestutil.BuildUnixTransparentProxyServer(filepath.Join(curDir, "testutil"), "libmmap")
	require.NoError(t, err)
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", arch))

	cmd := exec.Command(bin, lib)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	monitor := setupUSMTLSMonitor(t, cfg)
	require.NotNil(t, monitor)

	return cmd
}

func waitForProgramToBeTraced(t *testing.T, cmd *exec.Cmd) {
	utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid)
}

func waitForProgramNotToBeTraced(t *testing.T, cmd *exec.Cmd) {
	programType := "shared_libraries"
	pid := cmd.Process.Pid

	time.Sleep(3000 * time.Millisecond)

	traced := utils.GetTracedPrograms(programType)
	for _, prog := range traced {
		require.False(t, slices.Contains[[]uint32](prog.PIDs, uint32(pid)))
	}
}

func testArch(t *testing.T, arch string) {
	cmd := startTest(t, arch)

	if arch == runtime.GOARCH {
		waitForProgramToBeTraced(t, cmd)
	} else {
		waitForProgramNotToBeTraced(t, cmd)
	}
}

func TestArchAmd64(t *testing.T) {
	testArch(t, "amd64")
}

func TestArchArm64(t *testing.T) {
	testArch(t, "arm64")
}
