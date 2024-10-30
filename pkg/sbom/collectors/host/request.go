// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/sbom/types"
)

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	Path string
	FS   fs.FS
}

type relFS struct {
	root string
	fs   fs.FS
}

func newFS(root string) fs.FS {
	fs := os.DirFS(root)
	return &relFS{root: "/", fs: fs}
}

func (f *relFS) Open(name string) (fs.File, error) {
	if filepath.IsAbs(name) {
		var err error
		name, err = filepath.Rel(f.root, name)
		if err != nil {
			return nil, err
		}
	}

	return f.fs.Open(name)
}

// NewScanRequest creates a new scan request
func NewScanRequest(path string, fs fs.FS) types.ScanRequest {
	return scanRequest{Path: path, FS: fs}
}

// NewHostScanRequest creates a new scan request for the root filesystem
func NewHostScanRequest() types.ScanRequest {
	scanPath := "/"
	if hostRoot := os.Getenv("HOST_ROOT"); env.IsContainerized() && hostRoot != "" {
		scanPath = hostRoot
	}
	return NewScanRequest(scanPath, newFS("/"))
}

// Collector returns the collector name
func (r scanRequest) Collector() string {
	return "host"
}

// Type returns the scan request type
func (r scanRequest) Type(types.ScanOptions) string {
	return types.ScanFilesystemType
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.Path
}
