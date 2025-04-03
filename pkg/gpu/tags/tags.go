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

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetTags returns a slice of tags indicating GPU presence
func GetTags() []string {
	// Get the host's proc directory path
	procPath := kernel.ProcFSRoot()
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
