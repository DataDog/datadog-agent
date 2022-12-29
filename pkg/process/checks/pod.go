// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && orchestrator
// +build kubelet,orchestrator

package checks

import (
	"context"
	"fmt"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// Pod is a singleton PodCheck.
var Pod = &PodCheck{
	config: oconfig.NewDefaultOrchestratorConfig(),
}

// PodCheck is a check that returns container metadata and stats.
type PodCheck struct {
	sysInfo                 *model.SystemInfo
	containerFailedLogLimit *util.LogLimit
	processor               *processors.Processor
	config                  *oconfig.OrchestratorConfig
}

// Init initializes a PodCheck instance.
func (c *PodCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) error {
	c.containerFailedLogLimit = util.NewLogLimit(10, time.Minute*10)
	c.processor = processors.NewProcessor(new(k8sProcessors.PodHandlers))
	c.sysInfo = info
	return c.config.Load()
}

// Name returns the name of the ProcessCheck.
func (c *PodCheck) Name() string { return config.PodCheckName }

// RealTime indicates if this check only runs in real-time mode.
func (c *PodCheck) RealTime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (c *PodCheck) ShouldSaveLastRun() bool { return true }

// Run runs the PodCheck to collect a list of running pods
func (c *PodCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
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

	ctx := &processors.ProcessorContext{
		ClusterID:          clusterID,
		Cfg:                c.config,
		HostName:           cfg.HostName,
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

	return metadataMessages, nil
}

// Cleanup frees any resource held by the PodCheck before the agent exits
func (c *PodCheck) Cleanup() {}
