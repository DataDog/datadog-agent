// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"fmt"
	"strconv"
	"strings"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
)

const (
	processCoreUsageMetric   = "process.core.usage"
	processMemoryUsageMetric = "process.memory.usage"

	// Container-level distribution metric names not in the gpu. namespace, and so
	// are not documented in spec/gpu_metrics.yaml.
	containerGPUCoreUsageDist   = "container.gpu.core.usage.dist"
	containerGPUMemoryUsageDist = "container.gpu.memory.usage.dist"
)

// containerGPUDistributionName maps a per-process source gauge name to its
// corresponding per-container distribution metric name. Returns false for any
// metric that does not have a distribution counterpart.
func containerGPUDistributionName(sourceMetric string) (string, bool) {
	switch sourceMetric {
	case processCoreUsageMetric:
		return containerGPUCoreUsageDist, true
	case processMemoryUsageMetric:
		return containerGPUMemoryUsageDist, true
	}
	return "", false
}

// autoscalingDistTagKeys is a copy of containerTagsToExtract from
// dd-go/process/apps/container-distributions/worker.go
var autoscalingDistTagKeys = map[string]struct{}{
	// Standard tags
	"env":     {},
	"service": {},
	"version": {},
	// Kube tags
	"kube_container_name":  {},
	"kube_deployment":      {},
	"kube_namespace":       {},
	"kube_ownerref_kind":   {},
	"kube_ownerref_name":   {},
	"kube_stateful_set":    {},
	"kube_daemon_set":      {},
	"kube_autoscaler_kind": {},
	"kube_argo_rollout":    {},
}

// filterAutoscalingDistTags keeps only the keys listed in autoscalingDistTagKeys.
func filterAutoscalingDistTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		i := strings.IndexByte(t, ':')
		if i <= 0 {
			continue
		}
		if _, ok := autoscalingDistTagKeys[t[:i]]; ok {
			out = append(out, t)
		}
	}
	return out
}

// GetOrchestratorCardContainerTags returns the tags for the given container ID, filtered through autoscalingDistTagKeys.
func (c *WorkloadTagCache) GetOrchestratorCardContainerTags(containerID string) ([]string, error) {
	if containerID == "" {
		return nil, nil
	}
	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
	tags, err := c.tagger.Tag(entityID, taggertypes.OrchestratorCardinality)
	if err != nil {
		return nil, err
	}
	return filterAutoscalingDistTags(tags), nil
}

// resolveContainerID returns the owning container ID for the given workload,
// or empty string if none is known. Returns an error only for unsupported
// workload kinds or unrecoverable lookup failures.
func (c *WorkloadTagCache) resolveContainerID(workloadID workloadmeta.EntityID) (string, error) {
	switch workloadID.Kind {
	case workloadmeta.KindContainer:
		return workloadID.ID, nil
	case workloadmeta.KindProcess:
		pidInt, err := strconv.ParseInt(workloadID.ID, 10, 32)
		if err != nil {
			return "", fmt.Errorf("error converting process ID to int: %w", err)
		}
		pid := int32(pidInt)
		process, perr := c.wmeta.GetProcess(pid)
		if perr != nil {
			process = nil
		}
		containerID, _, cerr := c.resolveContainerIDForProcess(process, pid)
		if cerr != nil && !agenterrors.IsNotFound(cerr) {
			return "", cerr
		}
		return containerID, nil
	}
	return "", fmt.Errorf("unsupported workload kind: %s", workloadID.Kind)
}

// containerDistKey identifies a single per-container distribution sample for a
// given distribution metric.
type containerDistKey struct {
	distName    string
	containerID string
}

// containerDistAccumulator sums per-process source metric values into a single total per container
type containerDistAccumulator struct {
	totals map[containerDistKey]float64
	order  []containerDistKey
}

func newContainerDistAccumulator() *containerDistAccumulator {
	return &containerDistAccumulator{totals: make(map[containerDistKey]float64)}
}

// accumulateContainerDistributions accumulates a source metric's value into the per-container total for its corresponding distribution metric.
func (c *Check) accumulateContainerDistributions(acc *containerDistAccumulator, metric *nvidia.Metric, metricWorkloads []workloadmeta.EntityID) {
	distName, ok := containerGPUDistributionName(metric.Name)
	if !ok {
		return
	}

	seenContainers := make(map[string]struct{}, len(metricWorkloads))
	for _, workloadID := range metricWorkloads {
		containerID, _ := c.workloadTagCache.resolveContainerID(workloadID)
		if containerID == "" {
			continue
		}
		if _, seen := seenContainers[containerID]; seen {
			continue
		}
		seenContainers[containerID] = struct{}{}

		key := containerDistKey{distName: distName, containerID: containerID}
		if _, exists := acc.totals[key]; !exists {
			acc.order = append(acc.order, key)
		}
		acc.totals[key] += metric.Value
	}
}

// emitContainerDistributions submits one aggregated distribution sample per container for each accumulated distribution metric.
func (c *Check) emitContainerDistributions(acc *containerDistAccumulator, snd sender.Sender) []error {
	var errs []error
	for _, key := range acc.order {
		distTags, terr := c.workloadTagCache.GetOrchestratorCardContainerTags(key.containerID)
		if terr != nil && !agenterrors.IsNotFound(terr) {
			errs = append(errs, fmt.Errorf("error collecting orchestrator card container tags for distribution %s container %s: %w", key.distName, key.containerID, terr))
			continue
		}

		snd.Distribution(key.distName, acc.totals[key], "", distTags)
	}
	return errs
}
