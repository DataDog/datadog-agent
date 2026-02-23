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

	"golang.org/x/sys/windows"
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

// setFileReadableByEveryone sets the DACL on a file to allow:
// - Current owner (ddagentuser): Full Control
// - SYSTEM: Full Control
// - Administrators: Full Control
// - Everyone: Read and Execute
// The owner is NOT changed - it remains as ddagentuser
func setFileReadableByEveryone(path string) error {
	// Create an SDDL with only the DACL part (no Owner/Group)
	// D:AI - DACL, Auto-Inherit.
	//        "Inherit permissions from the parent folder (System/Admins usually)."
	// (A;;GRGX;;;WD) - Allow Generic Read + Generic Execute to Everyone (World Domain).
	sddl := "D:AI(A;;GRGX;;;WD)"

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("failed to create security descriptor: %w", err)
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("failed to get DACL: %w", err)
	}

	// Only set the DACL, don't touch owner or group
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,  // owner - leave unchanged
		nil,  // group - leave unchanged
		dacl, // DACL - set this
		nil,  // SACL - leave unchanged
	)
}

// compileAndWriteConfig compiles the policy binary into a binary file readable by the injector
// On Windows, sets ACLs to allow Everyone read+execute access while keeping ddagentuser as owner
func (c *workloadselectionComponent) compileAndWriteConfig(rawConfig []byte) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(configPath), "workload-policy-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command(getCompilePolicyBinaryPath(), "--input-string", string(rawConfig), "--output-file", tmpPath)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error executing dd-policy-compile (%w); out: '%s'; err: '%s'", err, stdoutBuf.String(), stderrBuf.String())
	}
	// Set permissions on the file to allow Everyone read+execute access
	// We keep the current owner (ddagentuser) and only modify the DACL
	if err := setFileReadableByEveryone(tmpPath); err != nil {
		return fmt.Errorf("failed to set permissions on file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to atomically replace policy file: %w", err)
	}
	return nil
}
