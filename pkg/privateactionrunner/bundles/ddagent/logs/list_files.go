// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// ListFilesInputs are the optional inputs for the listFiles action.
type ListFilesInputs struct {
	AdditionalDirs []string `json:"additionalDirs,omitempty"`
}

// FileEntry represents a single file discovered by the action.
type FileEntry struct {
	Path        string `json:"path"`                  // host-relative path
	Source      string `json:"source"`                // "process", "kubernetes", or "filesystem"
	ProcessName string `json:"processName,omitempty"` // set when source is "process"
	PID         int32  `json:"pid,omitempty"`         // set when source is "process"
	ServiceName string `json:"serviceName,omitempty"` // set when source is "process"
}

// ListFilesOutputs is the output returned by the listFiles action.
type ListFilesOutputs struct {
	Files  []FileEntry `json:"files"`
	Errors []string    `json:"errors,omitempty"`
}

// ListFilesHandler implements the listFiles action.
type ListFilesHandler struct {
	wmeta workloadmeta.Component
}

// NewListFilesHandler creates a new ListFilesHandler.
func NewListFilesHandler(wmeta workloadmeta.Component) *ListFilesHandler {
	return &ListFilesHandler{wmeta: wmeta}
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

	if err := validateAdditionalDirs(inputs.AdditionalDirs); err != nil {
		return nil, err
	}

	hostPrefix := getHostPrefix()
	seen := make(map[string]struct{})
	var allEntries []FileEntry
	var allErrors []string

	// 1. Process logs from workloadmeta
	for _, entry := range collectProcessLogs(h.wmeta) {
		if _, ok := seen[entry.Path]; ok {
			continue
		}
		seen[entry.Path] = struct{}{}
		allEntries = append(allEntries, entry)
	}

	// 2. Kubernetes logs
	k8sEntries, k8sErrs := collectK8sLogs(hostPrefix)
	allErrors = append(allErrors, k8sErrs...)
	for _, entry := range k8sEntries {
		if _, ok := seen[entry.Path]; ok {
			continue
		}
		seen[entry.Path] = struct{}{}
		allEntries = append(allEntries, entry)
	}

	// 3. Filesystem logs
	fsEntries, fsErrs := collectFilesystemLogs(hostPrefix, inputs.AdditionalDirs)
	allErrors = append(allErrors, fsErrs...)
	for _, entry := range fsEntries {
		if _, ok := seen[entry.Path]; ok {
			continue
		}
		seen[entry.Path] = struct{}{}
		allEntries = append(allEntries, entry)
	}

	if allEntries == nil {
		allEntries = []FileEntry{}
	}

	return &ListFilesOutputs{
		Files:  allEntries,
		Errors: allErrors,
	}, nil
}

// validateAdditionalDirs checks that all additional directories are absolute
// paths and do not contain path traversal components.
func validateAdditionalDirs(dirs []string) error {
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("additional directory must be absolute: %s", dir)
		}
		// Check the raw input for ".." path components before filepath.Clean
		// resolves them away.
		for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
			if part == ".." {
				return fmt.Errorf("additional directory must not contain '..': %s", dir)
			}
		}
	}
	return nil
}
