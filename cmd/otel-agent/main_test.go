// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package main

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExitCode_Disabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test takes too long and is flaky on Windows")
	}
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "false")
	cmd := exec.Command("go", "run", "-tags", "otlp", "main.go", "--config", "test_config.yaml")
	err := cmd.Run()
	require.NoError(t, err)
}
