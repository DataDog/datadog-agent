// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build orchestrator && kubeapiserver

// Package pod is used for the orchestrator pod check
package pod

import (
	"context"
	"errors"
	"fmt"

	"github.com/benbjohnson/clock"
	"go.uber.org/atomic"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// CheckName is the name of the check
const CheckName = "orchestrator_pod"

var groupID atomic.Int32

func nextGroupID() int32 {
	groupID.Add(1)
	return groupID.Load()
}

// checkConfig is the check instance configuration.
type checkConfig struct {
	// NodeSelector is a label selector of nodes whose pods the check reports, see runForSelectedNodes.
	// When empty the check reports the pods of the node the Agent runs on.
	NodeSelector string `yaml:"node_selector"`
}

// parseNodeSelector parses the node_selector option, nil when it is not set.
func parseNodeSelector(nodeSelector string) (labels.Selector, error) {
	if nodeSelector == "" {
		return nil, nil
	}
	selector, err := labels.Parse(nodeSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid node_selector %q: %w", nodeSelector, err)
	}
	return selector, nil
}

// validateNodeSelectorPlacement checks that node_selector is set exactly where the check does not run for the local node.
//
// In the Cluster Agent and as a cluster check, which runs once in a cluster check runner or a node agent,
// there is no local node to report, so node_selector is required.
// In a node agent configuration it is rejected: every node agent would report the pods of the selected nodes.
func validateNodeSelectorPlacement(provider string, hasNodeSelector bool) error {
	selectedNodes := flavor.GetFlavor() == flavor.ClusterAgent || provider == names.ClusterChecks
	switch {
	case selectedNodes && !hasNodeSelector:
		return errors.New("node_selector is required when the check runs in the Cluster Agent or as a cluster check")
	case !selectedNodes && hasNodeSelector:
		return errors.New("node_selector is not supported in node agents, configure the check in the Cluster Agent, optionally as a cluster check")
	}
	return nil
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nodeSelector labels.Selector
	// clusterName is the RFC1123 compliant cluster name used to build the hostnames of the selected nodes.
	clusterName string
	// clusterNameTags are added to the payloads of the selected nodes, as they have no host tags.
	clusterNameTags []string
	hostName        string
	clusterID       string
	sender          sender.Sender
	processor       *processors.Processor
	config          *oconfig.OrchestratorConfig
	systemInfo      *model.SystemInfo
	store           workloadmeta.Component
	cfg             config.Component
	tagger          tagger.Component
	agentVersion    *model.AgentVersion
}

// Factory creates a new check factory
func Factory(store workloadmeta.Component, cfg config.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(
		func() check.Check {
			return newCheck(store, cfg, tagger)
		},
	)
}

func newCheck(store workloadmeta.Component, cfg config.Component, tagger tagger.Component) check.Check {
	extraTags := cfg.GetStringSlice(oconfig.OrchestratorNSKey("extra_tags"))
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		config:    oconfig.NewDefaultOrchestratorConfig(extraTags),
		store:     store,
		cfg:       cfg,
		tagger:    tagger,
	}
}

// Configure the CPU check
// nil check to allow for overrides
func (c *Check) Configure(
	senderManager sender.SenderManager,
	integrationConfigDigest uint64,
	data integration.Data,
	initConfig integration.Data,
	source string,
	provider string,
) error {
	c.BuildID(integrationConfigDigest, data, initConfig)

	err := c.CommonConfigure(senderManager, initConfig, data, source, provider)
	if err != nil {
		return err
	}

	var instanceConfig checkConfig
	if err := yaml.Unmarshal(data, &instanceConfig); err != nil {
		return err
	}
	c.nodeSelector, err = parseNodeSelector(instanceConfig.NodeSelector)
	if err != nil {
		return err
	}
	if err := validateNodeSelectorPlacement(provider, c.nodeSelector != nil); err != nil {
		return err
	}

	err = c.config.Load()
	if err != nil {
		return err
	}
	if !c.config.OrchestrationCollectionEnabled {
		log.Warn("orchestrator pod check is configured but the feature is disabled")
		return nil
	}
	if c.config.KubeClusterName == "" {
		return errors.New("orchestrator check is configured but the cluster name is empty")
	}

	if c.processor == nil {
		c.processor = processors.NewProcessor(k8sProcessors.NewPodHandlers(c.cfg, c.store, c.tagger))
	}

	if c.sender == nil {
		sender, err := c.GetSender()
		if err != nil {
			return err
		}
		c.sender = sender
	}

	if c.hostName == "" {
		hname, _ := hostname.Get(context.TODO())
		c.hostName = hname
	}

	if c.nodeSelector != nil {
		c.clusterName = clustername.GetRFC1123CompliantClusterName(context.TODO(), c.hostName)
		if !c.cfg.GetBool("disable_cluster_name_tag_key") {
			if tag := clustername.GetClusterNameTagValue(context.TODO(), c.hostName); tag != "" {
				c.clusterNameTags = []string{"cluster_name:" + tag}
			}
		}
	}

	c.systemInfo, err = checks.CollectSystemInfo()
	if err != nil {
		log.Warnf("Failed to collect system info: %s", err)
	}

	agentVersion, err := version.Agent()
	if err != nil {
		log.Warnf("Failed to get agent version: %s", err)
	}
	c.agentVersion = &model.AgentVersion{
		Major:  agentVersion.Major,
		Minor:  agentVersion.Minor,
		Patch:  agentVersion.Patch,
		Pre:    agentVersion.Pre,
		Commit: agentVersion.Commit,
	}

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	if c.clusterID == "" {
		clusterID, err := clustername.GetClusterID()
		if err != nil {
			// Check if this is a temporary retry error from cluster agent client
			if retry.IsErrWillRetry(err) {
				log.Warnf("Cluster Agent not ready yet, skipping orchestrator_pod check run: %s", err)
				return nil
			}
			return err
		}
		c.clusterID = clusterID
	}

	if c.nodeSelector != nil {
		return c.runForSelectedNodes()
	}

	pods, err := listLocalPods(context.TODO())
	if err != nil {
		return err
	}

	listed, processed := c.processPods(pods, c.hostName, c.systemInfo, nil)
	if processed == -1 {
		return errors.New("unable to process pods: a panic occurred")
	}

	orchestrator.SetCacheStats(listed, processed, orchestrator.K8sPod)

	return nil
}

// processPods processes the pods of a node and sends the resulting payloads.
// hostName identifies the node, extraTags are added to the payload tags.
func (c *Check) processPods(pods []*corev1.Pod, hostName string, systemInfo *model.SystemInfo, extraTags []string) (listed, processed int) {
	groupID := nextGroupID()
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              c.config,
			Clock:            clock.New(),
			MsgGroupID:       groupID,
			NodeType:         orchestrator.K8sPod,
			ClusterID:        c.clusterID,
			ManifestProducer: true,
			Kind:             kubernetes.PodKind,
			APIVersion:       utilTypes.PodVersion,
			CollectorGroup:   utilTypes.PodGroup,
			CollectorName:    utilTypes.PodName,
			CollectorTags:    append([]string{"kube_api_version:" + utilTypes.PodVersion}, extraTags...),
			AgentVersion:     c.agentVersion,
		},
		HostName:   hostName,
		SystemInfo: systemInfo,
	}

	processResult, listed, processed := c.processor.Process(ctx, pods)
	if processed == -1 {
		return listed, processed
	}

	c.sender.OrchestratorMetadata(processResult.MetadataMessages, c.clusterID, int(orchestrator.K8sPod))
	c.sender.OrchestratorManifest(processResult.ManifestMessages, c.clusterID)

	return listed, processed
}
