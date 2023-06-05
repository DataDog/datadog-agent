// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sysctl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	// bogus sysctl
	_, err := NewString("/tmp", "foo", 0).Get()
	require.Error(t, err)

	s, err := NewString("/proc", "net/ipv4/tcp_keepalive_intvl", 0).Get()
	require.NoError(t, err)
	require.NotEmpty(t, s)

	procRoot := createTmpProcSys(t)

	createTmpSysctl(t, procRoot, "foo", "bar\n")
	sctl := NewString(procRoot, "foo", 10*time.Second)
	v, err := sctl.Get()
	require.NoError(t, err)
	require.Equal(t, "bar", v)
	// update the value on disk
	createTmpSysctl(t, procRoot, "foo", "baz\n")
	// check if value is still cached
	v, err = sctl.Get()
	require.NoError(t, err)
	require.Equal(t, "bar", v)

	createTmpSysctl(t, procRoot, "foo", "bar\n")
	// sysctl with a shorter ttl
	sctl = NewString(procRoot, "foo", 2*time.Second)
	// ttl should still not have expired
	v, err = sctl.Get()
	require.NoError(t, err)
	require.Equal(t, "bar", v)

	// check for ttl expiry
	createTmpSysctl(t, procRoot, "foo", "baz")
	v, err = sctl.get(time.Now().Add(2 * time.Second))
	require.NoError(t, err)
	require.Equal(t, "baz", v)
}

func TestInt(t *testing.T) {
	_, err := NewInt("/tmp", "foo", 0).Get()
	require.Error(t, err)

	i, err := NewInt("/proc", "net/ipv4/tcp_keepalive_intvl", 0).Get()
	require.NoError(t, err)
	require.NotZero(t, i)

	procRoot := createTmpProcSys(t)

	createTmpSysctl(t, procRoot, "foo", "12\n")
	sctl := NewInt(procRoot, "foo", 10*time.Second)
	v, err := sctl.Get()
	require.NoError(t, err)
	require.Equal(t, 12, v)
	// update the value on disk
	createTmpSysctl(t, procRoot, "foo", "22\n")
	// check if value is still cached
	v, err = sctl.Get()
	require.NoError(t, err)
	require.Equal(t, 12, v)

	createTmpSysctl(t, procRoot, "foo", "12\n")
	// sysctl with a shorter ttl
	sctl = NewInt(procRoot, "foo", 2*time.Second)
	// ttl should still not have expired
	v, err = sctl.Get()
	require.NoError(t, err)
	require.Equal(t, 12, v)

	// check for ttl expiry
	createTmpSysctl(t, procRoot, "foo", "22\n")
	v, err = sctl.get(time.Now().Add(2 * time.Second))
	require.NoError(t, err)
	require.Equal(t, 22, v)
}

func createTmpProcSys(t *testing.T) (procRoot string) {
	procRoot = t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(procRoot, "sys"), 0777))
	return procRoot
}

func createTmpSysctl(t *testing.T, procRoot, sysctl string, v string) {
	require.NoError(t, os.WriteFile(filepath.Join(procRoot, "sys", sysctl), []byte(v), 0777))
}
