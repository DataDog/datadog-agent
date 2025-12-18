// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	fileProvisionerDefaultID = "file"
	fileExtFilter            = ".json"
)

// FileProvisioner is a provisioner that reads JSON files from a filesystem.
type FileProvisioner struct {
	id   string
	path string
}

var _ Provisioner = &FileProvisioner{}

// NewFileProvisioner returns a new FileProvisioner.
func NewFileProvisioner(id string, path string) *FileProvisioner {
	if id == "" {
		id = fileProvisionerDefaultID
	}

	return &FileProvisioner{
		id:   id,
		path: path,
	}
}

// ID returns the ID of the provisioner.
func (fp *FileProvisioner) ID() string {
	return fp.id
}

// Provision reads JSON files from the filesystem and returns them as raw resources.
func (fp *FileProvisioner) Provision(context.Context, string, io.Writer) (RawResources, error) {
	resources := make(RawResources)

	content, err := os.ReadFile(fp.path)
	if err != nil {
		return nil, err
	}

	// The file is expected to be a "bundle" JSON map:
	//   {"RemoteHost": {...}, "Agent": {...}, ...}
	//
	// We unmarshal into json.RawMessage to keep each value as raw JSON bytes,
	// which is exactly what the e2e importer expects (per-resource payloads).
	var resourcesJSON map[string]json.RawMessage
	if err := json.Unmarshal(content, &resourcesJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file provisioner JSON %q: %w", fp.path, err)
	}
	if len(resourcesJSON) == 0 {
		return nil, fmt.Errorf("file provisioner JSON %q is empty (no resources)", fp.path)
	}
	for key, value := range resourcesJSON {
		resources[key] = value
	}

	return resources, nil
}

// Destroy is a no-op for the FileProvisioner.
func (fp *FileProvisioner) Destroy(context.Context, string, io.Writer) error {
	return nil
}
