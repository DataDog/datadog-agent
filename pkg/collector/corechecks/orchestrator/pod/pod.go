// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package pod

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

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
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const checkName = "pod"

var groupID int32

func nextGroupID() int32 {
	atomic.AddInt32(&groupID, 1)
	return groupID
}

func init() {
	core.RegisterCheck(checkName, podFactory)
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	hostInfo                *checks.HostInfo
	clusterID               string
	containerFailedLogLimit *util.LogLimit
	sender                  sender.Sender
	processor               *processors.Processor
	config                  *oconfig.OrchestratorConfig
	instance                *oinstance.OrchestratorInstance
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
		HostName:           c.hostInfo.HostName,
		MsgGroupID:         groupID,
		NodeType:           orchestrator.K8sPod,
		ApiGroupVersionTag: fmt.Sprintf("kube_api_version:%s", "v1"),
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

// Configure the CPU check
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, data, initConfig, source)
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
	err = c.instance.Parse(data)
	if err != nil {
		_ = log.Error("could not parse check instance config")
		return err
	}

	c.clusterID, err = clustername.GetClusterID()
	if err != nil {
		return err
	}

	c.processor = processors.NewProcessor(new(k8sProcessors.PodHandlers))

	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	c.sender = sender

	return nil
}

func podFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
	}
}
