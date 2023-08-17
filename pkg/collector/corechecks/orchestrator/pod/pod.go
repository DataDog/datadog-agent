// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build kubelet && orchestrator


package pod

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	oinstance "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const checkName = "pod"

var groupID atomic.Int32

func nextGroupID() int32 {
	groupID.Add(1)
	return groupID.Load()
}

func init() {
	core.RegisterCheck(checkName, podFactory)
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	hostInfo  *checks.HostInfo
	clusterID string
	sender    sender.Sender
	processor *processors.Processor
	config    *oconfig.OrchestratorConfig
	instance  *oinstance.OrchestratorInstance
}

func podFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
	}
}

// Configure the CPU check
// nil check to allow for overrides
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	c.BuildID(integrationConfigDigest, data, initConfig)

	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	err = c.config.Load()
	if err != nil {
		return err
	}

	if !c.config.OrchestrationCollectionEnabled {
		return errors.New("orchestrator check is configured but the feature is disabled")
	}
	if c.config.KubeClusterName == "" {
		return errors.New("orchestrator check is configured but the cluster name is empty")
	}

	// load instance level config
	if c.instance == nil {
		err = c.instance.Parse(data)
		if err != nil {
			_ = log.Error("could not parse check instance config")
			return err
		}
	}

	if c.clusterID == "" {
		c.clusterID, err = clustername.GetClusterID()
		if err != nil {
			return err
		}
	}

	if c.processor == nil {
		c.processor = processors.NewProcessor(new(k8sProcessors.PodHandlers))
	}

	if c.sender == nil {
		sender, err := c.GetSender()
		if err != nil {
			return err
		}
		c.sender = sender
	}

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return err
	}

	podList, err := kubeUtil.GetRawLocalPodList(context.TODO())
	if err != nil {
		return err
	}

	groupID := nextGroupID()
	ctx := &processors.ProcessorContext{
		ClusterID:          c.clusterID,
		Cfg:                c.config,
		MsgGroupID:         groupID,
		NodeType:           orchestrator.K8sPod,
		ApiGroupVersionTag: fmt.Sprintf("kube_api_version:%s", "v1"),
	}

	if c.hostInfo != nil {
		ctx.HostName = c.hostInfo.HostName
	}

	processResult, processed := c.processor.Process(ctx, podList)
	if processed == -1 {
		return fmt.Errorf("unable to process pods: a panic occurred")
	}

	// Append manifestMessages behind metadataMessages to avoiding modifying the func signature.
	// Split the messages during forwarding.
	metadataMessages := append(processResult.MetadataMessages, processResult.ManifestMessages...)

	orchestrator.SetCacheStats(len(podList), processed, ctx.NodeType)

	c.sender.OrchestratorMetadata(metadataMessages, c.clusterID, int(orchestrator.K8sPod))

	return nil

}
