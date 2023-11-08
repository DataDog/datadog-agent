// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
)

const (
	StaticProvisionerDefaultID = "static"

	fileExtFilter = ".json"
)

type FileProvisioner struct {
	id string
	fs fs.FS
}

var _ Provisioner = &FileProvisioner{}

func NewFileProvisioner(id string, fs fs.FS) *FileProvisioner {
	if id == "" {
		id = StaticProvisionerDefaultID
	}

	return &FileProvisioner{
		id: id,
		fs: fs,
	}
}

func (fp *FileProvisioner) ID() string {
	return fp.id
}

func (fp *FileProvisioner) Provision(string, context.Context) (RawResources, error) {
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

func (fp *FileProvisioner) Delete(string, context.Context) error {
	return nil
}
