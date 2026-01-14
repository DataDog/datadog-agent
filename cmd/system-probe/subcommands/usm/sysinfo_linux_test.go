// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestSysinfoCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeSysinfoCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "sysinfo", cmd.Use)
	require.Equal(t, "Show system information relevant to USM", cmd.Short)

	// Verify --max-cmdline-length flag exists
	maxCmdlineFlag := cmd.Flags().Lookup("max-cmdline-length")
	require.NotNil(t, maxCmdlineFlag, "--max-cmdline-length flag should exist")
	require.Equal(t, "50", maxCmdlineFlag.DefValue, "--max-cmdline-length should default to 50")

	// Verify --max-name-length flag exists
	maxNameFlag := cmd.Flags().Lookup("max-name-length")
	require.NotNil(t, maxNameFlag, "--max-name-length flag should exist")
	require.Equal(t, "25", maxNameFlag.DefValue, "--max-name-length should default to 25")

	// Verify --max-service-length flag exists
	maxServiceFlag := cmd.Flags().Lookup("max-service-length")
	require.NotNil(t, maxServiceFlag, "--max-service-length flag should exist")
	require.Equal(t, "20", maxServiceFlag.DefValue, "--max-service-length should default to 20")
}

func TestLanguageDetectionGo(t *testing.T) {
	// Create a process wrapper for the current test process (which is a Go binary)
	proc := &procutil.Process{
		Pid:     int32(os.Getpid()),
		Name:    "test",
		Cmdline: []string{"test"},
	}

	// Create language detector and detect language
	detector := privileged.NewLanguageDetector()
	languages := detector.DetectWithPrivileges([]languagemodels.Process{proc})

	// Verify we got one result
	require.Len(t, languages, 1)

	// Verify the language is Go
	assert.Equal(t, languagemodels.Go, languages[0].Name)

	// Verify the version is detected and matches the runtime version
	assert.NotEmpty(t, languages[0].Version)
	assert.Equal(t, runtime.Version(), languages[0].Version)
}
