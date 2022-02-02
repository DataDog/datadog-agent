// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package flare

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZipLinuxKernelSymbols(t *testing.T) {
	srcDir, err := ioutil.TempDir("", "run")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)
	dstDir, err := ioutil.TempDir("", "TestZipLinuxKernelSymbols")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	// create non-empty kallsyms file
	file, err := os.Create(filepath.Join(srcDir, "kallsyms"))
	require.NoError(t, err)
	_, err = file.WriteString("0000000000000000 A irq_stack_union")
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	err = zipLinuxKernelSymbols(dstDir, "test")
	require.NoError(t, err)

	// Check all the log files are in the destination path, at the right subdirectories
	stat, err := os.Stat(filepath.Join(dstDir, "test", "kallsyms"))
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))
}

func TestZipLinuxKrobeEvents(t *testing.T) {
	srcDir, err := ioutil.TempDir("", "run")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)
	dstDir, err := ioutil.TempDir("", "TestZipLinuxKrobeEvents")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	// create non-empty kprobe_events file
	file, err := os.Create(filepath.Join(srcDir, "kprobe_events"))
	require.NoError(t, err)
	_, err = file.WriteString("0000000000000000 A irq_stack_union")
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	err = zipLinuxKrobeEvents(dstDir, "test")
	require.NoError(t, err)

	// Check all the log files are in the destination path, at the right subdirectories
	stat, err := os.Stat(filepath.Join(dstDir, "test", "kprobe_events"))
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))
}

func TestZipLinuxPid1MountInfo(t *testing.T) {
	srcDir, err := ioutil.TempDir("", "run")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)
	dstDir, err := ioutil.TempDir("", "TestZipLinuxPid1MountInfo")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	// create non-empty mountinfo file
	file, err := os.Create(filepath.Join(srcDir, "mountinfo"))
	require.NoError(t, err)
	_, err = file.WriteString("1910 1286 0:322")
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	err = zipLinuxPid1MountInfo(dstDir, "test")
	require.NoError(t, err)

	// Check all the log files are in the destination path, at the right subdirectories
	stat, err := os.Stat(filepath.Join(dstDir, "test", "mountinfo"))
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))
}

func TestZipLinuxTracingAvailableEvents(t *testing.T) {
	srcDir, err := ioutil.TempDir("", "run")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)
	dstDir, err := ioutil.TempDir("", "TestZipLinuxTracingAvailableEvents")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	// create non-empty available_events file
	file, err := os.Create(filepath.Join(srcDir, "available_events"))
	require.NoError(t, err)
	_, err = file.WriteString("0000000000000000 A irq_stack_union")
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	err = zipLinuxTracingAvailableEvents(dstDir, "test")
	require.NoError(t, err)

	// Check all the log files are in the destination path, at the right subdirectories
	stat, err := os.Stat(filepath.Join(dstDir, "test", "available_events"))
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))
}

func TestZipLinuxTracingAvailableFilterFunctions(t *testing.T) {
	srcDir, err := ioutil.TempDir("", "run")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)
	dstDir, err := ioutil.TempDir("", "TestZipLinuxTracingAvailableFilterFunctions")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	// create non-empty available_filter_functions file
	file, err := os.Create(filepath.Join(srcDir, "available_filter_functions"))
	require.NoError(t, err)
	_, err = file.WriteString("0000000000000000 A irq_stack_union")
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	err = zipLinuxTracingAvailableFilterFunctions(dstDir, "test")
	require.NoError(t, err)

	// Check all the log files are in the destination path, at the right subdirectories
	stat, err := os.Stat(filepath.Join(dstDir, "test", "available_filter_functions"))
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))
}

// func TestZipLinuxFiles(t *testing.T) {
// 	assert := assert.New(t)

// 	tests := []struct {
// 		name string
// 		file string
// 		test func
// 	}{
// 		{
// 			name: "TestZipLinuxKernelSymbols",
// 			file: "kallsyms",
// 			test:
// 		},
// 		{
// 			name: "TestZipLinuxKrobeEvents",
// 			file: "kprobe_events",
// 		},
// 		{
// 			name: "TestZipLinuxPid1MountInfo",
// 			file: "mountinfo",
// 		},
// 		{
// 			name: "TestZipLinuxTracingAvailableEvents",
// 			file: "available_events",
// 		},
// 		{
// 			name: "TestZipLinuxTracingAvailableFilterFunctions",
// 			file: "available_filter_functions",
// 		},
// 	}

// 	for _, test := range tests {
// 		t.Run(test.name, func(t *testing.T) {
// 			srcDir, err := ioutil.TempDir("", "run")
// 			require.NoError(t, err)
// 			defer os.RemoveAll(srcDir)
// 			dstDir, err := ioutil.TempDir("", name)
// 			require.NoError(t, err)
// 			defer os.RemoveAll(dstDir)

// 			// create non-empty test file
// 			file, err := os.Create(filepath.Join(srcDir, file))
// 			require.NoError(t, err)
// 			_, err = file.WriteString("0000000000000000 A irq_stack_union")
// 			require.NoError(t, err)
// 			err = file.Close()
// 			require.NoError(t, err)

// 			err = zipLinuxTracingAvailableFilterFunctions(srcDir, dstDir, "test", file)
// 			require.NoError(t, err)

// 			// Check all the log files are in the destination path, at the right subdirectories
// 			stat, err := os.Stat(filepath.Join(dstDir, "test", file))
// 			require.NoError(t, err)
// 			require.Greater(t, stat.Size(), int64(0))
// 		})
// 	}
// }
