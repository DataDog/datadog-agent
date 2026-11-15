// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package sysfsroot finds the sysfs root holding the GPU device tree. It only
// depends on pkg/util/kernel, so pkg/gpu/tags can use it without pulling in pkg/gpu/amd.
package sysfsroot

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// DeviceSysFSRoot returns kernel.SysFSRoot(), or /sys when that root has no class/
// directory and /sys has one: device sysfs is not namespaced, so the container's /sys shows the host devices.
var DeviceSysFSRoot = newDeviceSysFSRoot()

func newDeviceSysFSRoot() func() string {
	return funcs.MemoizeNoError(func() string {
		return deviceSysFSRoot(kernel.SysFSRoot(), "/sys")
	})
}

func deviceSysFSRoot(root, fallback string) string {
	if root == fallback || isDir(filepath.Join(root, "class")) || !isDir(filepath.Join(fallback, "class")) {
		return root
	}
	return fallback
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
