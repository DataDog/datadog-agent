// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package kubelet implements the Kubelet check.
package kubelet

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "kubelet"
)

// Provider provides the metrics related to a given Kubelet endpoint
type Provider interface {
	Provide(kubelet.KubeUtilInterface, sender.Sender) error
}

// KubeletCheck wraps the config and the metric stores needed to run the check
type KubeletCheck struct {
	core.CheckBase
	instance    *common.KubeletConfig
	providers   []Provider
	podUtils    *common.PodUtils
	filterStore workloadfilter.Component
	store       workloadmeta.Component
	tagger      tagger.Component
}

// NewKubeletCheck returns a new KubeletCheck
func NewKubeletCheck(base core.CheckBase, instance *common.KubeletConfig, store workloadmeta.Component, filterStore workloadfilter.Component, tagger tagger.Component) *KubeletCheck {
	return &KubeletCheck{
		CheckBase:   base,
		instance:    instance,
		filterStore: filterStore,
		store:       store,
		tagger:      tagger,
	}
}

func initProviders(filterStore workloadfilter.Component, config *common.KubeletConfig, podUtils *common.PodUtils, store workloadmeta.Component, tagger tagger.Component) []Provider {
	podProvider := pod.NewProvider(filterStore, config, podUtils, tagger)
	// nodeProvider collects from the /spec endpoint, which was hidden by default in k8s 1.18 and removed in k8s 1.19.
	// It is here for backwards compatibility.
	nodeProvider := node.NewProvider(config)
	healthProvider := health.NewProvider(config)
	summaryProvider := summary.NewProvider(filterStore, config, store, tagger)

	sliProvider, err := slis.NewProvider(config, store)
	if err != nil {
		log.Warnf("Can't get sli provider: %v", err)
	}

	probeProvider, err := probe.NewProvider(filterStore, config, store, tagger)
	if err != nil {
		log.Warnf("Can't get probe provider: %v", err)
	}

	kubeletProvider, err := kube.NewProvider(filterStore, config, store, podUtils, tagger)
	if err != nil {
		log.Warnf("Can't get kubelet provider: %v", err)
	}

	cadvisorProvider, err := cadvisor.NewProvider(filterStore, config, store, podUtils, tagger)
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

// Factory returns a new KubeletCheck factory
func Factory(store workloadmeta.Component, filterStore workloadfilter.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return NewKubeletCheck(core.NewCheckBase(CheckName), &common.KubeletConfig{}, store, filterStore, tagger)
	})
}

// Configure configures the check
func (k *KubeletCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if !pkgconfigsetup.Datadog().GetBool("kubelet_core_check_enabled") {
		return fmt.Errorf("%w: kubelet core check is disabled", check.ErrSkipCheckInstance)
	}

	err := k.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	err = k.instance.Parse(config)
	if err != nil {
		return err
	}

	// Set sane defaults
	if k.instance.Timeout == 0 {
		// old default was 10 seconds, let's keep it
		k.instance.Timeout = 10
	}

	k.instance.Namespace = common.KubeletMetricsPrefix
	if k.instance.SendHistogramBuckets == nil {
		sendBuckets := true
		k.instance.SendHistogramBuckets = &sendBuckets
	}

	k.podUtils = common.NewPodUtils(k.tagger)
	k.providers = initProviders(k.filterStore, k.instance, k.podUtils, k.store, k.tagger)

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
