// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements helpers to parse a Datadog Agent Flare and fetch its content
package flare

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
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

// GetAgentVersion is a getter for the 'agentVersion' field
func (flare *Flare) GetAgentVersion() string {
	return flare.agentVersion
}

// GetHostname is a getter for the 'hostname' field
func (flare *Flare) GetHostname() string {
	return flare.hostname
}

// GetFilenames returns all the filenames in the flare archive
func (flare *Flare) GetFilenames() []string {
	filenames := make([]string, 0, len(flare.zipFiles))
	for name := range flare.zipFiles {
		filenames = append(filenames, name)
	}
	return filenames
}

// GetFile returns a *zip.File whose name is 'path' or 'path/'. Returns an error if the file does not exist
func (flare *Flare) GetFile(path string) (*zip.File, error) {
	cleanPath := filepath.Clean(path)
	file, found := flare.zipFiles[cleanPath]

	if !found {
		return nil, fmt.Errorf("Could not find %v file in flare archive", cleanPath)
	}

	return file, nil
}

// GetFileInfo returns a fs.FileInfo associated to the file whose name is 'path' or 'path/'. Returns an error if the file does not exist
func (flare *Flare) GetFileInfo(path string) (fs.FileInfo, error) {
	file, err := flare.GetFile(path)
	if err != nil {
		return nil, err
	}

	return file.FileInfo(), nil
}

// GetPermission returns a fs.FileMode associated to the file whose name is 'path' or 'path/'. Returns an error if the file does not exist
func (flare *Flare) GetPermission(path string) (os.FileMode, error) {
	fileInfo, err := flare.GetFileInfo(path)
	if err != nil {
		return 0, err
	}

	return fileInfo.Mode(), nil
}

// GetFileContent gets the content from a file and returns it as a string. Returns an error if the file does not exist
func (flare *Flare) GetFileContent(path string) (string, error) {
	file, err := flare.GetFile(path)
	if err != nil {
		return "", err
	}

	fileReader, err := file.Open()
	if err != nil {
		return "", nil
	}
	defer fileReader.Close()

	fileContent := make([]byte, file.UncompressedSize64)
	_, err = io.ReadFull(fileReader, fileContent)
	if err != nil {
		return "", nil
	}

	return string(fileContent), nil
}
