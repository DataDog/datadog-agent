// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && orchestrator

package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewPodCheck returns an instance of the Pod check
func NewPodCheck() *PodCheck {
	return &PodCheck{
		config: oconfig.NewDefaultOrchestratorConfig(),
	}
}

// PodCheck is a check that returns container metadata and stats.
type PodCheck struct {
	hostInfo                *HostInfo
	containerFailedLogLimit *util.LogLimit
	processor               *processors.Processor
	config                  *oconfig.OrchestratorConfig
}

// Init initializes a PodCheck instance.
func (c *PodCheck) Init(_ *SysProbeConfig, hostInfo *HostInfo) error {
	c.hostInfo = hostInfo
	c.containerFailedLogLimit = util.NewLogLimit(10, time.Minute*10)
	c.processor = processors.NewProcessor(new(k8sProcessors.PodHandlers))
	return c.config.Load()
}

// IsEnabled returns true if the check is enabled by configuration
func (c *PodCheck) IsEnabled() bool {
	// activate the pod collection if enabled and we have the cluster name set
	orchestratorEnabled, kubeClusterName := oconfig.IsOrchestratorEnabled()
	if !orchestratorEnabled {
		return false
	}

	if kubeClusterName == "" {
		_ = log.Warnf("Failed to auto-detect a Kubernetes cluster name. Pod collection will not start. To fix this, set it manually via the cluster_name config option")
		return false
	}
	return true
}

// SupportsRunOptions returns true if the check supports RunOptions
func (c *PodCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the ProcessCheck.
func (c *PodCheck) Name() string { return PodCheckName }

// Realtime indicates if this check only runs in real-time mode.
func (c *PodCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (c *PodCheck) ShouldSaveLastRun() bool { return true }

// Run runs the PodCheck to collect a list of running pods
func (c *PodCheck) Run(nextGroupID func() int32, _ *RunOptions) (RunResult, error) {
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	clusterID, err := clustername.GetClusterID()
	if err != nil {
		return nil, err
	}

	podList, err := kubeUtil.GetRawLocalPodList(context.TODO())
	if err != nil {
		return nil, err
	}

	groupID := nextGroupID()
	ctx := &processors.ProcessorContext{
		ClusterID:          clusterID,
		Cfg:                c.config,
		HostName:           c.hostInfo.HostName,
		MsgGroupID:         groupID,
		NodeType:           orchestrator.K8sPod,
		ApiGroupVersionTag: fmt.Sprintf("kube_api_version:%s", "v1"),
	}

	processResult, processed := c.processor.Process(ctx, podList)

	if processed == -1 {
		return nil, fmt.Errorf("unable to process pods: a panic occurred")
	}

	// Append manifestMessages behind metadataMessages to avoiding modifying the func signature.
	// Split the messages during forwarding.
	metadataMessages := append(processResult.MetadataMessages, processResult.ManifestMessages...)

	orchestrator.SetCacheStats(len(podList), processed, ctx.NodeType)

	return StandardRunResult(metadataMessages), nil
}

// Cleanup frees any resource held by the PodCheck before the agent exits
func (c *PodCheck) Cleanup() {}
