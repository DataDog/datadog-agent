// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteagentimpl implements the remoteagent component interface
package remoteagentimpl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	core "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// remoteFlareFiller implements the FlareBuilder interface for remote agents.
// It collects flare data and builds a GetFlareFilesResponse that can be sent
// to another process.
type remoteFlareFiller struct {
	files        map[string][]byte
	creationLogs bytes.Buffer
}

// newRemoteFlareFiller creates a new remoteFlareFiller instance
func newRemoteFlareFiller() *remoteFlareFiller {
	return &remoteFlareFiller{
		files: make(map[string][]byte),
	}
}

// IsLocal returns true when the flare is created by the CLI instead of the running Agent process
func (r *remoteFlareFiller) IsLocal() bool {
	return false // remoteFlareFiller is always used in a remote context, so it is never local
}

// Logf adds a formatted log entry to the flare file
func (r *remoteFlareFiller) Logf(format string, params ...interface{}) error {
	// Add timestamp to log
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, fmt.Sprintf(format, params...))

	_, err := r.creationLogs.WriteString(logEntry)
	if err != nil {
		return fmt.Errorf("error writing to flare log: %v", err)
	}

	// Also add the logs to the flare files map
	r.files["flare-creation.log"] = r.creationLogs.Bytes()

	return nil
}

// AddFile creates a new file in the flare with the content
func (r *remoteFlareFiller) AddFile(destFile string, content []byte) error {
	// Content should be scrubbed here in a real implementation
	// For this implementation, we'll just store the content
	r.files[destFile] = content
	return nil
}

// AddFileWithoutScrubbing creates a new file in the flare with the content without scrubbing
func (r *remoteFlareFiller) AddFileWithoutScrubbing(destFile string, content []byte) error {
	r.files[destFile] = content
	return nil
}

// AddFileFromFunc creates a new file in the flare with the content returned by the callback
func (r *remoteFlareFiller) AddFileFromFunc(destFile string, cb func() ([]byte, error)) error {
	content, err := cb()
	if err != nil {
		_ = r.Logf("Error getting content for %s: %v", destFile, err)
		return err
	}

	// Content should be scrubbed here in a real implementation
	return r.AddFile(destFile, content)
}

// CopyFile copies the content of srcFile to the root of the flare
func (r *remoteFlareFiller) CopyFile(srcFile string) error {
	filename := filepath.Base(srcFile)
	return r.CopyFileTo(srcFile, filename)
}

// CopyFileTo copies the content of srcFile to destFile in the flare
func (r *remoteFlareFiller) CopyFileTo(srcFile string, destFile string) error {
	content, err := os.ReadFile(srcFile)
	if err != nil {
		_ = r.Logf("Error reading file %s: %v", srcFile, err)
		return err
	}

	// Content should be scrubbed here in a real implementation
	return r.AddFile(destFile, content)
}

// CopyDirTo copies files from the srcDir to a specific directory in the flare
func (r *remoteFlareFiller) CopyDirTo(srcDir string, destDir string, shouldInclude func(string) bool) error {
	return r.copyDirToInternal(srcDir, destDir, shouldInclude, true)
}

// CopyDirToWithoutScrubbing copies files from the srcDir to a specific directory in the flare without scrubbing
func (r *remoteFlareFiller) CopyDirToWithoutScrubbing(srcDir string, destDir string, shouldInclude func(string) bool) error {
	return r.copyDirToInternal(srcDir, destDir, shouldInclude, false)
}

// copyDirToInternal is a helper function for directory copying
func (r *remoteFlareFiller) copyDirToInternal(srcDir string, destDir string, shouldInclude func(string) bool, scrub bool) error {
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if shouldInclude != nil && !shouldInclude(relPath) {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		content, err := os.ReadFile(path)
		if err != nil {
			_ = r.Logf("Error reading file %s: %v", path, err)
			return err
		}

		if scrub {
			return r.AddFile(destPath, content)
		}
		return r.AddFileWithoutScrubbing(destPath, content)
	})

	if err != nil {
		_ = r.Logf("Error walking directory %s: %v", srcDir, err)
	}

	return err
}

// PrepareFilePath returns the full path of a file in the flare
func (r *remoteFlareFiller) PrepareFilePath(path string) (string, error) {
	// For remote flare filler, we don't actually create files on disk
	// We just return a placeholder path to satisfy the interface
	_ = r.Logf("PrepareFilePath called for %s", path)
	return filepath.Join("/tmp", "remote-flare", path), nil
}

// RegisterFilePerm adds the current permissions for a file to the flare's permissions.log
func (r *remoteFlareFiller) RegisterFilePerm(path string) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		_ = r.Logf("Error getting file permissions for %s: %v", path, err)
		return
	}

	permInfo := fmt.Sprintf("%s: %s\n", path, fileInfo.Mode().String())

	// Append to permissions log
	currentPerms, exists := r.files["permissions.log"]
	if !exists {
		currentPerms = []byte{}
	}

	r.files["permissions.log"] = append(currentPerms, []byte(permInfo)...)
}

// RegisterDirPerm adds the current permissions for all the files in a directory to the flare's permissions.log
func (r *remoteFlareFiller) RegisterDirPerm(path string) {
	_ = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			_ = r.Logf("Error walking directory for permissions %s: %v", filePath, err)
			return nil
		}

		r.RegisterFilePerm(filePath)
		return nil
	})
}

// GetFlareArgs returns the struct of caller-provided arguments that can be referenced by various flare providers
func (r *remoteFlareFiller) GetFlareArgs() flarebuilder.FlareArgs {
	return flarebuilder.FlareArgs{}
}

// Save archives all the data added to the flare, cleanup all the temporary directories and return the path to
// the archive file. For remoteFlareFiller, this just finalizes the in-memory representation.
func (r *remoteFlareFiller) Save() (string, error) {
	// For a remote flare filler, we don't actually create an archive file
	// We just log that Save was called and return a placeholder
	_ = r.Logf("Save called - %d files collected", len(r.files))
	return "remote-flare-placeholder", nil
}

// GetFlareResponse converts the collected files into a GetFlareFilesResponse
func (r *remoteFlareFiller) GetFlareResponse() *core.GetFlareFilesResponse {
	// Make sure the creation log is included
	r.files["flare-creation.log"] = r.creationLogs.Bytes()

	response := &core.GetFlareFilesResponse{
		Files: r.files,
	}

	return response
}
