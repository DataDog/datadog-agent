// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
	"github.com/stretchr/testify/require"
)

func TestModuleSymlinkRepair(t *testing.T) {
	for _, target := range []string{"/usr/src/headers", "../../../usr/src/headers", "cached absolute"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lib/modules/test/build")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "usr/src/headers"), 0755))
			if target == "cached absolute" {
				target = filepath.Join(dir, "usr/src/headers")
			}
			require.NoError(t, os.Symlink(target, path))
			destination, err := openExtractionDirectory(dir)
			require.NoError(t, err)
			defer destination.root.Close()
			require.NoError(t, fixKernelModulesSymlinks(destination, "test", nil))
			require.NoError(t, fixKernelModulesSymlinks(destination, "test", nil))
			resolved, err := filepath.EvalSymlinks(path)
			require.NoError(t, err)
			require.Equal(t, filepath.Join(dir, "usr/src/headers"), resolved)
			link, err := os.Readlink(path)
			require.NoError(t, err)
			require.False(t, filepath.IsAbs(link))
		})
	}
}

func TestModuleSymlinkRepairRejectsAncestorEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "test/build")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.Symlink("/sentinel", path))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "lib"), 0755))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "lib/modules")))
	destination, err := openExtractionDirectory(dir)
	require.NoError(t, err)
	defer destination.root.Close()
	require.Error(t, fixKernelModulesSymlinks(destination, "test", nil))
	target, err := os.Readlink(path)
	require.NoError(t, err)
	require.Equal(t, "/sentinel", target)
	require.Error(t, fixKernelModulesSymlinks(destination, "../../escape", nil))
}

func TestModuleSymlinkRepairRejectsRelativeEscape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib/modules/test/build")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.Symlink("../../../../outside", path))
	destination, err := openExtractionDirectory(dir)
	require.NoError(t, err)
	defer destination.root.Close()
	require.Error(t, fixKernelModulesSymlinks(destination, "test", nil))
}

func TestModuleSymlinkRepairRestrictsLayout(t *testing.T) {
	for _, target := range []string{"../test/build", "missing/../headers", "/usr/../headers"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "lib/modules/test/build")
			require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
			require.NoError(t, os.Symlink(target, name))
			destination, err := openExtractionDirectory(dir)
			require.NoError(t, err)
			defer destination.root.Close()
			require.Error(t, fixKernelModulesSymlinks(destination, "test", nil))
		})
	}
}

func TestModuleSymlinkRepairAllowsMissingTarget(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "lib/modules/test/source")
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
	require.NoError(t, os.Symlink("/usr/src/headers", name))
	destination, err := openExtractionDirectory(dir)
	require.NoError(t, err)
	defer destination.root.Close()
	require.NoError(t, fixKernelModulesSymlinks(destination, "test", nil))
	target, err := os.Readlink(name)
	require.NoError(t, err)
	require.Equal(t, "../../../usr/src/headers", target)
}

type moduleRepairLogger struct {
	types.Logger
	debug, warnings []string
}

func (logger *moduleRepairLogger) Debugf(format string, args ...interface{}) {
	logger.debug = append(logger.debug, fmt.Sprintf(format, args...))
}

func (logger *moduleRepairLogger) Warnf(format string, args ...interface{}) {
	logger.warnings = append(logger.warnings, fmt.Sprintf(format, args...))
}

func TestModuleSymlinkRepairLogs(t *testing.T) {
	destination, err := openExtractionDirectory(t.TempDir())
	require.NoError(t, err)
	defer destination.root.Close()
	require.NoError(t, destination.mkdir("lib/modules/test"))
	require.NoError(t, destination.root.Symlink("/usr/src/headers", "lib/modules/test/build"))
	logger := &moduleRepairLogger{}
	require.NoError(t, fixKernelModulesSymlinks(destination, "test", logger))
	require.Len(t, logger.debug, 1)
	require.Contains(t, logger.debug[0], "lib/modules/test/build")

	require.NoError(t, destination.root.Symlink("../../../../outside", "lib/modules/test/source"))
	require.Error(t, fixKernelModulesSymlinks(destination, "test", logger))
	require.Len(t, logger.warnings, 1)
	require.Contains(t, logger.warnings[0], "lib/modules/test/source")
}
