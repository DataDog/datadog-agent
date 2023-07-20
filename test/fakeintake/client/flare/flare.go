// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"strings"
)

// Flare contains all the information sent by the Datadog Agent when using the Flare command
// zipFiles is a mapping between filenames and *zip.File obtained from zip.Reader struct.
//
// * `email`: email provided when creating the flare.
// * `zipFiles`: map between filenames and their information in the form of a zip.File object
// * `agentVersion`: the version of the Agent which created the flare.
// * `hostname`: hostname of the host on which the flare was created. Also the name of the flare root folder.
type Flare struct {
	email        string
	zipFiles     map[string]*zip.File
	agentVersion string
	hostname     string
}

// GetEmail is a getter for the 'email' field
func (flare *Flare) GetEmail() string {
	return flare.email
}

// GetEmail is a getter for the 'agentVersion' field
func (flare *Flare) GetAgentVersion() string {
	return flare.agentVersion
}

// GetEmail is a getter for the 'hostname' field
func (flare *Flare) GetHostname() string {
	return flare.hostname
}

// FileExists returns true if the filename exists in the flare archive
// If the file is within subfolders, the full path should be provided
func (flare *Flare) FileExists(filename string) bool {
	_, found := flare.zipFiles[trimTrailingSlash(filename)]
	return found
}

// IsFile returns true if filename exists and is a regular file.
func (flare *Flare) IsFile(filename string) bool {
	return flare.FileExists(filename) && flare.getFileInfo(filename).Mode().IsRegular()
}

// IsFile returns true if filename exists and is a directory.
func (flare *Flare) IsDir(dirname string) bool {
	return flare.FileExists(dirname) && flare.getFileInfo(dirname).Mode().IsDir()
}

// IsFile returns true if filename exists and has the right permissions
func (flare *Flare) HasPerm(filename string, perm fs.FileMode) bool {
	return flare.FileExists(filename) && flare.getFileInfo(filename).Mode().Perm() == perm
}

// FileHasContent returns true if filename is a regular file and its content is not empty
func (flare *Flare) FileHasContent(filename string) bool {
	return flare.IsFile(filename) && flare.getFileInfo(filename).Size() > 0
}

// FileContains returns true if filename is a regular file and if its content contains a given pattern
func (flare *Flare) FileContains(filename string, pattern string) bool {
	if !flare.IsFile(filename) {
		return false
	}

	fileContent := flare.getFileContent(filename)
	return strings.Contains(fileContent, pattern)
}

// getFile returns a *zip.File whose name is 'path' or 'path/'. It's expected that the caller has verified that 'path' exists before calling this function.
func (flare *Flare) getFile(path string) *zip.File {
	return flare.zipFiles[trimTrailingSlash(path)]
}

// getFileInfo returns a fs.FileInfo associated to the file whose name is 'path' or 'path/'. It's expected that the caller has verified that 'path' exists before calling this function.
func (flare *Flare) getFileInfo(path string) fs.FileInfo {
	return flare.getFile(path).FileInfo()
}

// getFileContent gets the content from a file and returns it as a string
func (flare *Flare) getFileContent(path string) string {
	file := flare.getFile(path)
	fileReader, err := file.Open()
	if err != nil {
		return ""
	}

	fileContent := make([]byte, file.UncompressedSize64)
	_, err = io.ReadFull(fileReader, fileContent)
	if err != nil {
		return ""
	}

	return string(fileContent)
}

// trimTrailingSlash removes all '/' (or equivalent depending on the OS) at the end of 'path'
func trimTrailingSlash(path string) string {
	return strings.TrimRight(path, string(os.PathSeparator))
}
