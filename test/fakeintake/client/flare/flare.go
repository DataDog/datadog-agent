// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"io"
	"io/fs"
	"path/filepath"
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

// getFile returns a *zip.File whose name is 'path' or 'path/'. It's expected that the caller has verified that 'path' exists before calling this function.
func (flare *Flare) getFile(path string) *zip.File {
	return flare.zipFiles[filepath.Clean(path)]
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
