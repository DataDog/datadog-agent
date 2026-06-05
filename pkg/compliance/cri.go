// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package compliance

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
	"go.yaml.in/yaml/v3"
)

const (
	criRuntimeContainerd = "containerd"
	criRuntimeDocker     = "docker"
	criRuntimeCRIO       = "crio"
)

// getKubeletCRIRuntime returns "containerd", "docker", "crio", or "" by
// inspecting the running kubelet. hostroot is the agent's view of the host
// filesystem; kubelet --config paths are resolved relative to it.
func getKubeletCRIRuntime(ctx context.Context, hostroot string) string {
	return criRuntimeFromEndpoint(findKubeletCRIEndpoint(ctx, hostroot))
}

// findKubeletCRIEndpoint reads --container-runtime-endpoint from the kubelet
// process, falling back to containerRuntimeEndpoint in its --config YAML.
func findKubeletCRIEndpoint(ctx context.Context, hostroot string) string {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return ""
	}
	for _, p := range procs {
		name, _ := p.Name()
		cmdline, err := p.CmdlineSlice()
		if err != nil || len(cmdline) == 0 {
			continue
		}
		isKubelet := name == "kubelet" || (name == "hyperkube" && len(cmdline) >= 2 && cmdline[1] == "kubelet")
		if !isKubelet {
			continue
		}
		flags := parseCmdlineFlags(cmdline)
		if v := flags["--container-runtime-endpoint"]; v != "" {
			return v
		}
		if cfgPath := flags["--config"]; cfgPath != "" {
			return readCRIEndpointFromConfig(filepath.Join(hostroot, cfgPath))
		}
		return ""
	}
	return ""
}

// readCRIEndpointFromConfig returns the containerRuntimeEndpoint value from
// the kubelet config YAML at path, or "" if unset or unreadable.
func readCRIEndpointFromConfig(path string) string {
	const maxConfigSize = 1 << 19 // 512 KiB
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() > maxConfigSize {
		return ""
	}
	var cfg struct {
		ContainerRuntimeEndpoint string `yaml:"containerRuntimeEndpoint"`
	}
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return ""
	}
	return cfg.ContainerRuntimeEndpoint
}

// criRuntimeFromEndpoint maps an endpoint URI to a runtime tag. cri-dockerd
// and dockershim shim Docker, so they must match before "containerd".
func criRuntimeFromEndpoint(endpoint string) string {
	e := strings.ToLower(endpoint)
	switch {
	case e == "":
		return ""
	case strings.Contains(e, "cri-dockerd"),
		strings.Contains(e, "dockershim"),
		strings.Contains(e, "docker.sock"):
		return criRuntimeDocker
	case strings.Contains(e, "containerd"):
		return criRuntimeContainerd
	case strings.Contains(e, "crio"):
		return criRuntimeCRIO
	default:
		return ""
	}
}
