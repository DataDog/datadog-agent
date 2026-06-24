// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package systemprobeimpl

import (
	"os"
	"path/filepath"

	"github.com/DataDog/ebpf-manager/tracefs"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// collectRuntimeSecurityKernelArtifacts gathers kernel and tracefs artifacts
// used by CWS (runtime-security) to diagnose eBPF probe attachment. Previously
// collected by the `security-agent flare` command.
func collectRuntimeSecurityKernelArtifacts(logger log.Component, files map[string][]byte) {
	addFile(logger, files, "kallsyms", "/proc/kallsyms")
	addFile(logger, files, "pid1_mountinfo", "/proc/1/mountinfo")

	traceFSPath, err := tracefs.Root()
	if err != nil {
		logger.Debugf("tracefs not available for flare collection: %v", err)
		return
	}
	for _, name := range []string{"kprobe_events", "available_events", "available_filter_functions"} {
		addFile(logger, files, name, filepath.Join(traceFSPath, name))
	}
}

func addFile(logger log.Component, files map[string][]byte, key, path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		logger.Debugf("failed to read %q for flare: %v", path, err)
		return
	}
	files[key] = content
}
