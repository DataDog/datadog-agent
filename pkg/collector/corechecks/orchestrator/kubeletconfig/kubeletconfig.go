// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build kubelet && orchestrator

// Package kubeletconfig is used `for the orchestrator kubelet_config check
package kubeletconfig

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/twmb/murmur3"
	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
)

// CheckName is the name of the check
const CheckName = "orchestrator_kubelet_config"

const collectionInterval = 10 * time.Minute
const kubeletVirtualKind = "KubeletConfiguration"
const kubeletVirtualAPIVersion = "virtual.datadoghq.com/v1"

var getClusterAgentClient = clusteragent.GetClusterAgentClient

var groupID atomic.Int32

func nextGroupID() int32 {
	groupID.Add(1)
	return groupID.Load()
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	hostName     string
	clusterID    string
	sender       sender.Sender
	config       *oconfig.OrchestratorConfig
	systemInfo   *model.SystemInfo
	store        workloadmeta.Component
	cfg          config.Component
	tagger       tagger.Component
	agentVersion *model.AgentVersion
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

// Configure the kubelet_config check
func (c *Check) Configure(
	senderManager sender.SenderManager,
	integrationConfigDigest uint64,
	data integration.Data,
	initConfig integration.Data,
	source string,
) error {
	c.BuildID(integrationConfigDigest, data, initConfig)

	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	err = c.config.Load()
	if err != nil {
		return err
	}

	if !c.config.KubeletConfigCheckEnabled {
		return fmt.Errorf("%w: orchestrator kubelet_config check is disabled", check.ErrSkipCheckInstance)
	}

	if !c.config.OrchestrationCollectionEnabled {
		return fmt.Errorf("%w: orchestrator kubelet_config check is enabled but the orchestration collection is disabled", check.ErrSkipCheckInstance)
	}

	if c.config.KubeClusterName == "" {
		return errors.New("orchestrator kubelet_config check is configured but the cluster name is empty")
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
		id, err := clustername.GetClusterID()
		if err != nil {
			return err
		}
		c.clusterID = id
	}

	kubelet, err := c.store.GetKubelet()
	if err != nil {
		return err
	}

	nodeName := kubelet.NodeName
	uid, err := getNodeUID(nodeName)
	if err != nil {
		return err
	}

	rawKubeletConfig := kubelet.RawConfig
	if rawKubeletConfig == nil {
		return errors.New("kubelet config not found in workloadmeta store")
	}

	rv := strconv.FormatUint(murmur3.Sum64(rawKubeletConfig), 10)

	tags := []string{}

	manifest := &model.Manifest{
		Type:            int32(orchestrator.K8sKubeletConfig),
		Uid:             uid,
		ResourceVersion: rv,
		Content:         rawKubeletConfig,
		ContentType:     "application/json",
		Version:         "v1",
		Tags:            tags,
		IsTerminated:    false,
		Kind:            kubeletVirtualKind,
		ApiVersion:      kubeletVirtualAPIVersion,
		NodeName:        nodeName,
	}

	msg := []model.MessageBody{
		&model.CollectorManifest{
			ClusterName:     c.config.KubeClusterName,
			ClusterId:       c.clusterID,
			GroupId:         nextGroupID(),
			HostName:        c.hostName,
			Manifests:       []*model.Manifest{manifest},
			Tags:            c.config.ExtraTags,
			AgentVersion:    c.agentVersion,
			OriginCollector: model.OriginCollector_datadogAgent,
		},
	}

	c.sender.OrchestratorManifest(msg, c.clusterID)
	return nil
}

// Interval returns the scheduling time for the check.
func (c *Check) Interval() time.Duration {
	return collectionInterval
}

func getNodeUID(nodeName string) (string, error) {
	if pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") {
		cl, err := getClusterAgentClient()
		if err != nil {
			return "", err
		}
		return cl.GetNodeUID(nodeName)
	}

	return "", errors.New("cluster_agent isn't enabled")
}
