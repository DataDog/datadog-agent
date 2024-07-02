// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/stretchr/testify/require"
)

func testArch(t *testing.T, arch string) {
	cfg := config.New()
	cfg.EnableNativeTLSMonitoring = true

	if !http.TLSSupported(cfg) {
		t.Skip("shared library tracing not supported for this platform")
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "testdata", "libmmap")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", arch))

	monitor := setupUSMTLSMonitor(t, cfg)
	require.NotNil(t, monitor)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	if arch == runtime.GOARCH {
		utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid)
	} else {
		utils.WaitForPathToBeBlocked(t, "shared_libraries", lib)
	}
}

func TestArchAmd64(t *testing.T) {
	testArch(t, "amd64")
}

func TestArchArm64(t *testing.T) {
	testArch(t, "arm64")
}
