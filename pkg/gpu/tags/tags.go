// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tags provides gpu host tags to the host payload
package tags

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// procFSRoot retrieves the current procfs dir we should use
// this function is copied from kernel/fs.go because using that function directly violates the check_size,
// as some artifacts have a major binary size increase
var procFSRoot = funcs.MemoizeNoError(func() string {
	if v := os.Getenv("HOST_PROC"); v != "" {
		return v
	}
	if os.Getenv("DOCKER_DD_AGENT") != "" {
		if _, err := os.Stat("/host"); err == nil {
			return "/host/proc"
		}
	}
	return "/proc"
})

// GetTags returns a slice of tags indicating GPU presence
func GetTags() []string {
	// Get the host's proc directory path
	procPath := procFSRoot()
	nvidiaPath := filepath.Join(procPath, "driver", "nvidia", "gpus")

	// Check if the NVIDIA directory exists
	if _, err := os.Stat(nvidiaPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		log.Warnf("Failed to check NVIDIA GPU directory: %v", err)
		return nil
	}

	// Read the directory to count GPU entries
	entries, err := os.ReadDir(nvidiaPath)
	if err != nil {
		log.Warnf("Failed to read NVIDIA GPU directory: %v", err)
		return nil
	}

	// If we have at least one entry, we have a GPU
	if len(entries) > 0 {
		return []string{"gpu_host:true"}
	}

	return nil
}
