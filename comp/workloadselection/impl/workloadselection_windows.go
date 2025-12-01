// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package workloadselectionimpl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// getCompilePolicyBinaryPath returns the full path to the compile policy binary on Windows
func getCompilePolicyBinaryPath() string {
	return filepath.Join(getInstallPath(), "bin", "dd-compile-policy.exe")
}

// isCompilePolicyBinaryAvailable checks if the compile policy binary is available
// on Windows systems
func (c *workloadselectionComponent) isCompilePolicyBinaryAvailable() bool {
	compilePath := getCompilePolicyBinaryPath()
	info, err := os.Stat(compilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			c.log.Warnf("failed to stat APM workload selection compile policy binary: %v", err)
		}
		return false
	}
	// On Windows, check that it's a regular file (not a directory)
	return info.Mode().IsRegular()
}

// compileAndWriteConfig compiles the policy binary into a binary file readable by the injector
// On Windows, uses SetRepositoryPermissions to set appropriate ACLs
func (c *workloadselectionComponent) compileAndWriteConfig(rawConfig []byte) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := paths.SetRepositoryPermissions(dir); err != nil {
		return fmt.Errorf("failed to set permissions on directory %s: %w", dir, err)
	}
	cmd := exec.Command(getCompilePolicyBinaryPath(), "--input-string", string(rawConfig), "--output-file", configPath)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error executing dd-policy-compile (%w); out: '%s'; err: '%s'", err, stdoutBuf.String(), stderrBuf.String())
	}
	return nil
}
