// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package workloadselectionimpl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// getCompilePolicyBinaryPath returns the full path to the compile policy binary on Unix
func getCompilePolicyBinaryPath() string {
	return filepath.Join(getInstallPath(), "embedded", "bin", "dd-compile-policy")
}

// isCompilePolicyBinaryAvailable checks if the compile policy binary is available
// and executable on Unix systems
func (c *workloadselectionComponent) isCompilePolicyBinaryAvailable() bool {
	compilePath := getCompilePolicyBinaryPath()
	info, err := os.Stat(compilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			c.log.Warnf("failed to stat APM workload selection compile policy binary: %v", err)
		}
		return false
	}
	// On Unix, check for executable bits
	return info.Mode().IsRegular() && info.Mode()&0111 != 0
}

// compileAndWriteConfig compiles the policy binary into a binary file readable by the injector
// On Unix systems, uses standard 0755 permissions for the directory
func (c *workloadselectionComponent) compileAndWriteConfig(rawConfig []byte) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
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
