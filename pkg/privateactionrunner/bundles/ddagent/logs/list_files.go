// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// ListFilesInputs are the inputs for the listFiles action.
type ListFilesInputs struct {
	Path string `json:"path"`
}

// FileEntry represents a single entry returned by the listFiles action.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"` // RFC3339
}

// ListFilesOutputs is the output returned by the listFiles action.
type ListFilesOutputs struct {
	Entries []FileEntry `json:"entries"`
	Error   string      `json:"error,omitempty"`
}

// ListFilesHandler implements the listFiles action.
type ListFilesHandler struct{}

// NewListFilesHandler creates a new ListFilesHandler.
func NewListFilesHandler() *ListFilesHandler {
	return &ListFilesHandler{}
}

// Run executes the listFiles action.
func (h *ListFilesHandler) Run(
	_ context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ListFilesInputs](task)
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(inputs.Path) {
		return nil, fmt.Errorf("path must be absolute: %s", inputs.Path)
	}

	hostPrefix := getHostPrefix()
	hostPath := toHostPath(hostPrefix, inputs.Path)

	dirEntries, err := os.ReadDir(hostPath)
	if err != nil {
		return &ListFilesOutputs{
			Entries: []FileEntry{},
			Error:   err.Error(),
		}, nil
	}

	entries := make([]FileEntry, 0, len(dirEntries))
	for _, d := range dirEntries {
		info, err := d.Info()
		if err != nil {
			continue
		}
		entries = append(entries, FileEntry{
			Name:    d.Name(),
			Path:    toOutputPath(hostPrefix, filepath.Join(hostPath, d.Name())),
			IsDir:   d.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	return &ListFilesOutputs{Entries: entries}, nil
}
