// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package containers

import (
	"errors"
	"fmt"
	"os"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// CDISpecDirs are the directories searched, in order, for the CDI specs that
// the NVIDIA DRA driver writes into the host's /var/run/cdi. That is a real
// directory on disk rather than a kernel filesystem, so a containerized Agent
// only sees it through a mount: the standard deployment mounts the host's
// /var/run at /host/var/run, so that prefix is tried first; the bare path
// covers a host-installed Agent and deployments that bind /var/run/cdi
// directly. It is a variable so tests can point it at fixtures.
var CDISpecDirs = []string{"/host/var/run/cdi", "/var/run/cdi"}

// NvidiaDRAClaimSpecPrefix is the file name prefix of the CDI spec the NVIDIA
// DRA driver writes for each claim: <prefix><claim UID>.yaml.
const NvidiaDRAClaimSpecPrefix = "k8s.gpu.nvidia.com-claim_"

// CheckNvidiaDRACDISpecs returns nil if at least one of CDISpecDirs contains
// NVIDIA DRA claim specs, and otherwise an error describing, for each
// directory, why it has none.
func CheckNvidiaDRACDISpecs() error {
	var errs []error
	for _, dir := range CDISpecDirs {
		err := checkDirHasNvidiaDRASpecs(dir)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func checkDirHasNvidiaDRASpecs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read CDI spec dir: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), NvidiaDRAClaimSpecPrefix) {
			return nil
		}
	}
	return fmt.Errorf("no NVIDIA DRA claim specs (%s*) in %s", NvidiaDRAClaimSpecPrefix, dir)
}

// HasUnresolvedDRAGPUClaim returns true if the container is running and holds
// an NVIDIA DRA GPU claim with CDI devices that has not been resolved to any
// GPU device. Stopped containers are excluded: their claims are released and
// their CDI specs removed while they can still be listed.
func HasUnresolvedDRAGPUClaim(container *workloadmeta.Container) bool {
	if !container.State.Running || len(container.GPUDeviceIDs) > 0 {
		return false
	}
	for _, resource := range container.ResolvedAllocatedResources {
		if resource.Name == string(gpuutil.GpuNvidiaDRA) && len(resource.CdiDevices) > 0 {
			return true
		}
	}
	return false
}
