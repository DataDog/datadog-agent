// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package kubelet implements the Kubelet check.
package kubelet

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/cadvisor"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/health"
	kube "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/kubelet"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/node"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/pod"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/probe"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/slis"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/summary"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeletCheckName = "kubelet_core"
)

// Provider provides the metrics related to a given Kubelet endpoint
type Provider interface {
	Provide(kubelet.KubeUtilInterface, sender.Sender) error
}

// KubeletCheck wraps the config and the metric stores needed to run the check
type KubeletCheck struct {
	core.CheckBase
	instance  *common.KubeletConfig
	filter    *containers.Filter
	providers []Provider
	podUtils  *common.PodUtils
}

// NewKubeletCheck returns a new KubeletCheck
func NewKubeletCheck(base core.CheckBase, instance *common.KubeletConfig) *KubeletCheck {
	return &KubeletCheck{
		CheckBase: base,
		instance:  instance,
	}
}

func initProviders(filter *containers.Filter, config *common.KubeletConfig, podUtils *common.PodUtils) []Provider {
	podProvider := pod.NewProvider(filter, config, podUtils)
	// nodeProvider collects from the /spec endpoint, which was hidden by default in k8s 1.18 and removed in k8s 1.19.
	// It is here for backwards compatibility.
	nodeProvider := node.NewProvider(config)
	healthProvider := health.NewProvider(config)
	summaryProvider := summary.NewProvider(filter, config, workloadmeta.GetGlobalStore())

	sliProvider, err := slis.NewProvider(filter, config, workloadmeta.GetGlobalStore())
	if err != nil {
		log.Warnf("Can't get sli provider: %v", err)
	}

	probeProvider, err := probe.NewProvider(filter, config, workloadmeta.GetGlobalStore())
	if err != nil {
		log.Warnf("Can't get probe provider: %v", err)
	}

	kubeletProvider, err := kube.NewProvider(filter, config, workloadmeta.GetGlobalStore(), podUtils)
	if err != nil {
		log.Warnf("Can't get kubelet provider: %v", err)
	}

	cadvisorProvider, err := cadvisor.NewProvider(filter, config, workloadmeta.GetGlobalStore(), podUtils)
	if err != nil {
		log.Warnf("Can't get cadvisor provider: %v", err)
	}

	return []Provider{
		healthProvider,
		podProvider,
		nodeProvider,
		summaryProvider,
		cadvisorProvider,
		kubeletProvider,
		probeProvider,
		sliProvider,
	}
}

// KubeletFactory returns a new KubeletCheck
func KubeletFactory() check.Check {
	return NewKubeletCheck(core.NewCheckBase(kubeletCheckName), &common.KubeletConfig{})
}

// Configure configures the check
func (k *KubeletCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	err := k.CommonConfigure(senderManager, integrationConfigDigest, initConfig, config, source)
	if err != nil {
		return err
	}

	filter, err := containers.GetSharedMetricFilter()
	if err != nil {
		log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %v", err)
	}
	k.filter = filter

	err = k.instance.Parse(config)
	if err != nil {
		return err
	}

	k.instance.Namespace = common.KubeletMetricsPrefix
	if k.instance.SendHistogramBuckets == nil {
		sendBuckets := true
		k.instance.SendHistogramBuckets = &sendBuckets
	}

	k.podUtils = common.NewPodUtils()
	k.providers = initProviders(filter, k.instance, k.podUtils)

	return nil
}

// Run runs the check
func (k *KubeletCheck) Run() error {
	sender, err := k.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()
	defer k.podUtils.Reset()

	// Get client
	kc, err := kubelet.GetKubeUtil()
	if err != nil {
		_ = k.Warnf("Error initialising check: %s", err)
		return err
	}

	for _, provider := range k.providers {
		if provider != nil {
			err = provider.Provide(kc, sender)
			if err != nil {
				_ = k.Warnf("Error reporting metrics: %s", err)
			}
		}
	}

	return nil
}

func init() {
	core.RegisterCheck(kubeletCheckName, KubeletFactory)
}
