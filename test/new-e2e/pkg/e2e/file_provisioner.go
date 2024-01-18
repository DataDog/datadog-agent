// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

const (
	fileProvisionerDefaultID = "file"
	fileExtFilter            = ".json"
)

// FileProvisioner is a provisioner that reads JSON files from a filesystem.
type FileProvisioner struct {
	id string
	fs fs.FS
}

var _ Provisioner = &FileProvisioner{}

// NewFileProvisioner returns a new FileProvisioner.
func NewFileProvisioner(id string, fs fs.FS) *FileProvisioner {
	if id == "" {
		id = fileProvisionerDefaultID
	}

	return &FileProvisioner{
		id: id,
		fs: fs,
	}
}

// ID returns the ID of the provisioner.
func (fp *FileProvisioner) ID() string {
	return fp.id
}

// Provision reads JSON files from the filesystem and returns them as raw resources.
func (fp *FileProvisioner) Provision(context.Context, string, io.Writer) (RawResources, error) {
	resources := make(RawResources)

	return resources, fs.WalkDir(fp.fs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}

		if !d.Type().IsRegular() {
			return nil
		}

		if filepath.Ext(path) != fileExtFilter {
			return nil
		}

		data, err := fs.ReadFile(fp.fs, path)
		if err != nil {
			return err
		}

		// We may need to put the relative path instead of just filename
		resources[strings.TrimSuffix(d.Name(), fileExtFilter)] = data
		return nil
	})
}

// Destroy is a no-op for the FileProvisioner.
func (fp *FileProvisioner) Destroy(context.Context, string, io.Writer) error {
	return nil
}
