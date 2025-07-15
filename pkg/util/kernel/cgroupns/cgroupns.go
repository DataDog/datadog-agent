// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package cgroupns provides utilities to work with cgroup namespaces.
package cgroupns

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

// WithRootNS enters the root cgroup namespace and calls the provided function.
func WithRootNS(procRoot string, fn func() error) error {
	rootNS, err := GetRootCgroupNS(procRoot)
	if err != nil {
		return fmt.Errorf("failed to get root cgroup namespace: %w", err)
	}
	defer rootNS.Close()

	return WithNS(procRoot, rootNS, fn)
}

// WithNS enters the provided cgroup namespace and calls the provided function.
func WithNS(procRoot string, ns netns.NsHandle, fn func() error) error {
	if ns == netns.None() {
		return fn()
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	prevNS, err := GetCurrentCgroupNS(procRoot)
	if err != nil {
		return fmt.Errorf("failed to get current cgroup namespace: %w", err)
	}
	defer prevNS.Close()

	if ns.Equal(prevNS) {
		return fn()
	}

	if err := Set(ns); err != nil {
		return fmt.Errorf("failed to set cgroup namespace %d: %w", ns, err)
	}

	fnErr := fn()
	nsErr := Set(prevNS)
	if fnErr != nil {
		return fnErr
	}
	if nsErr != nil {
		return fmt.Errorf("failed to go back to previous cgroup namespace: %w", nsErr)
	}
	return nil
}

// GetCgroupNamespaceFromPid gets the cgroup namespace for a given pid.
func GetCgroupNamespaceFromPid(procRoot string, pid int) (netns.NsHandle, error) {
	return netns.GetFromPath(filepath.Join(procRoot, fmt.Sprintf("%d/ns/cgroup", pid)))
}

// GetCgroupNamespaceFromPidAndThread gets the cgroup namespace for a given pid and thread.
func GetCgroupNamespaceFromPidAndThread(procRoot string, pid int, tid int) (netns.NsHandle, error) {
	return netns.GetFromPath(filepath.Join(procRoot, fmt.Sprintf("%d/task/%d/ns/cgroup", pid, tid)))
}

// GetRootCgroupNS gets the root cgroup namespace.
func GetRootCgroupNS(procRoot string) (netns.NsHandle, error) {
	return GetCgroupNamespaceFromPid(procRoot, 1)
}

// GetCurrentCgroupNS gets the current cgroup namespace.
func GetCurrentCgroupNS(procRoot string) (netns.NsHandle, error) {
	return GetCgroupNamespaceFromPidAndThread(procRoot, os.Getpid(), unix.Gettid())
}

// Set sets the current cgroup namespace.
func Set(ns netns.NsHandle) error {
	return unix.Setns(int(ns), unix.CLONE_NEWCGROUP)
}
