// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestInjectedEnvBasic(t *testing.T) {
	curPid := os.Getpid()
	proc, err := process.NewProcess(int32(curPid))
	require.NoError(t, err)
	injectionMeta, ok := getInjectionMeta(proc)
	require.Nil(t, injectionMeta)
	require.False(t, ok)

	// Provide an injected replacement for some already-present env variable
	first := os.Environ()[0]
	parts := strings.Split(first, "=")
	key := parts[0]

	expected := []string{"key1=val1", "key2=val2", "key3=val3", fmt.Sprint(key, "=", "new")}
	createEnvsMemfd(t, expected)

	envMap, err := getEnvs(proc)
	require.NoError(t, err)
	require.Subset(t, envMap, map[string]string{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
		key:    "new",
	})
}

func TestInjectedEnvLimit(t *testing.T) {
	env := "A=" + strings.Repeat("A", memFdMaxSize*2)
	full := []string{env}
	createEnvsMemfd(t, full)

	proc, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)
	_, ok := getInjectionMeta(proc)
	require.False(t, ok)
}

// createEnvsMemfd creates an memfd in the current process with the specified
// environment variables in the same way as Datadog/auto_inject.
func createEnvsMemfd(t *testing.T, envs []string) {
	t.Helper()

	var injectionMeta InjectedProcess
	for _, env := range envs {
		injectionMeta.InjectedEnv = append(injectionMeta.InjectedEnv, []byte(env))
	}
	encodedInjectionMeta, err := injectionMeta.MarshalMsg(nil)
	require.NoError(t, err)

	memfd, err := memfile(injectorMemFdName, encodedInjectionMeta)
	require.NoError(t, err)
	t.Cleanup(func() { unix.Close(memfd) })
}

// memfile takes a file name used, and the byte slice containing data the file
// should contain.
//
// name does not need to be unique, as it's used only for debugging purposes.
//
// It is up to the caller to close the returned descriptor.
func memfile(name string, b []byte) (int, error) {
	fd, err := unix.MemfdCreate(name, 0)
	if err != nil {
		return 0, fmt.Errorf("MemfdCreate: %v", err)
	}

	err = unix.Ftruncate(fd, int64(len(b)))
	if err != nil {
		return 0, fmt.Errorf("Ftruncate: %v", err)
	}

	data, err := unix.Mmap(fd, 0, len(b), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return 0, fmt.Errorf("Mmap: %v", err)
	}

	copy(data, b)

	err = unix.Munmap(data)
	if err != nil {
		return 0, fmt.Errorf("Munmap: %v", err)
	}

	return fd, nil
}
