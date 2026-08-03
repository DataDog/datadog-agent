// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	model "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
)

// errTracerMemfdNotFound reports that a process has published no tracer
// metadata. It is distinct from the errors returned when the descriptors cannot
// be inspected at all, because a process that has not published yet will
// usually publish later, while one we are not allowed to read never will.
var errTracerMemfdNotFound = errors.New("tracer memfd not found")

// readTracerMetadata reads the tracer metadata a process has published.
//
// kernel.GetProcessMemFdFile does the same search, but reports every failure as
// a single "not found" error, which would collapse the two cases above into
// one. The search is a directory walk; the parts that define what the metadata
// actually is stay in the tracermetadata package.
func readTracerMetadata(
	procfsRoot string, pid int32,
) (model.TracerMetadata, error) {
	path, err := findTracerMemfd(procfsRoot, pid)
	if err != nil {
		return model.TracerMetadata{}, err
	}
	return tracermetadata.GetTracerMetadataFromPath(path)
}

// findTracerMemfd returns the path of the descriptor holding the process'
// tracer metadata.
func findTracerMemfd(procfsRoot string, pid int32) (string, error) {
	fdDir := filepath.Join(procfsRoot, strconv.Itoa(int(pid)), "fd")

	// Tracers open the memfd early in startup, so it is usually the first
	// descriptor the process opens itself.
	if path := filepath.Join(fdDir, "3"); isTracerMemfd(path) {
		return path, nil
	}

	dir, err := os.Open(fdDir)
	if err != nil {
		return "", err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return "", err
	}
	for _, name := range names {
		switch name {
		case "0", "1", "2", "3":
			continue
		}
		if path := filepath.Join(fdDir, name); isTracerMemfd(path) {
			return path, nil
		}
	}
	return "", errTracerMemfdNotFound
}

func isTracerMemfd(path string) bool {
	target, err := os.Readlink(path)
	return err == nil && tracermetadata.IsTracerMemfdPath(target)
}
