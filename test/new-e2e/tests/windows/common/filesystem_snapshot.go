// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"strings"
)

// FileSystemSnapshot represents a snapshot of the system files that can be used to compare against later
type FileSystemSnapshot struct {
	host *components.RemoteHost
	path string
}

// Validate ensures the snapshot file exists and is a reasonable size
func (fs *FileSystemSnapshot) Validate() error {
	// ensure file exists
	_, err := fs.host.Lstat(fs.path)
	if err != nil {
		return fmt.Errorf("system file snapshot %s does not exist: %w", fs.path, err)
	}
	// sanity check to ensure file contains a reasonable amount of output
	stat, err := fs.host.Lstat(fs.path)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", fs.path, err)
	}
	if stat.Size() < int64(1024*1024) {
		return fmt.Errorf("system file snapshot %s is too small: %d bytes", fs.path, stat.Size())
	}
	return nil
}

// Cleanup removes the snapshot if it exists
func (fs *FileSystemSnapshot) Cleanup() error {
	exists, err := fs.host.FileExists(fs.path)
	if err != nil {
		return fmt.Errorf("failed to check if snapshot exists %s: %w", fs.path, err)
	}
	if !exists {
		return nil
	}
	err = fs.host.Remove(fs.path)
	if err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", fs.path, err)
	}
	return nil
}

// CompareSnapshots compares two system file snapshots and returns a list of files that are missing in the second snapshot
func (fs *FileSystemSnapshot) CompareSnapshots(other *FileSystemSnapshot) (string, error) {
	// Diff the two files on the remote host, selecting missing items
	// diffing remotely saves bandwidth and is faster than downloading the (relatively large) files
	cmd := fmt.Sprintf(`Compare-Object -ReferenceObject (Get-Content "%s") -DifferenceObject (Get-Content "%s") | Where-Object -Property SideIndicator -EQ '<=' | Select -ExpandProperty InputObject`, fs.path, other.path)
	output, err := fs.host.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("compare system files command failed: %s", err)
	}
	output = strings.TrimSpace(output)
	return output, nil
}

// NewFileSystemSnapshot takes a snapshot of the system files that can be used to compare against later.
// The snapshot is overridden if it already exists.
func NewFileSystemSnapshot(host *components.RemoteHost, pathsToIgnore []string) (*FileSystemSnapshot, error) {
	tempFile, err := GetTemporaryFile(host)
	if err != nil {
		return nil, err
	}

	// quote each path and join with commas
	pattern := ""
	for _, ignorePath := range pathsToIgnore {
		pattern += fmt.Sprintf(`'%s',`, ignorePath)
	}

	// PowerShell list syntax
	pattern = fmt.Sprintf(`@(%s)`, strings.Trim(pattern, ","))
	// Recursively list Windows directory and ignore the paths above
	// Compare-Object is case insensitive by default
	cmd := fmt.Sprintf(`cmd /c dir C:\Windows /b /s | Out-String -Stream | Select-String -NotMatch -SimpleMatch -Pattern %s | Select -ExpandProperty Line > "%s"`, pattern, tempFile)
	if len(cmd) > 8192 {
		return nil, fmt.Errorf("command length %d exceeds max command length: '%s'", len(cmd), cmd)
	}
	_, err = host.Execute(cmd)
	if err != nil {
		return nil, fmt.Errorf("snapshot system files command failed: %s", err)
	}
	f := &FileSystemSnapshot{host: host, path: tempFile}
	err = f.Validate()
	if err != nil {
		cleanupErr := f.Cleanup()
		if cleanupErr != nil {
			return nil, fmt.Errorf("failed to validate and cleanup snapshot: %w, %w", err, cleanupErr)
		}
		return nil, err
	}
	return f, nil
}
