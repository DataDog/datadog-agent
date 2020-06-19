// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/pkg/options"
)

const (
	// TODO rename correctly once we deprecate the python check
	kubeStateMetricsCheckName = "kube-state-metrics-alpha"
)

type KSMConfig struct {
	// TODO fill in all the configurations.
	Collectors []string `yaml:"collectors"`
}

type KSMCheck struct {
	core.CheckBase
	instance *KSMConfig
	store    []cache.Store
}

func init() {
	core.RegisterCheck(kubeStateMetricsCheckName, KubeStateMetricsFactory)
}

func (k *KSMCheck) Configure(config, initConfig integration.Data, source string) error {
	err := k.CommonConfigure(config, source)
	if err != nil {
		return err
	}
	err = k.instance.parse(config)
	if err != nil {
		return err
	}

	builder := kubestatemetrics.New()

	// Prepare the collectors for the resources specified in the configuration file.
	if err := builder.WithEnabledResources(k.instance.Collectors); err != nil {
		return err
	}

	// TODO namespaces should be configurable
	builder.WithNamespaces(options.DefaultNamespaces)

	// TODO Metrics exclusion/inclusion needs to be configurable
	allowDenyList, err := allowdenylist.New(options.MetricSet{}, options.MetricSet{})
	if err != nil {
		return err
	}

	if err := allowDenyList.Parse(); err != nil {
		return err
	}
	builder.WithAllowDenyList(allowDenyList)

	c, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}

	builder.WithKubeClient(c.Cl)
	builder.WithContext(context.Background())
	builder.WithResync(30 * time.Second) // TODO resync period should be configurable
	builder.WithGenerateStoreFunc(builder.GenerateStore)

	// Start the collection process
	k.store = builder.Build()

	return nil
}

func (c *KSMConfig) parse(data []byte) error {
	// TODO specify the default values
	return yaml.Unmarshal(data, c)
}

func (k *KSMCheck) Run() error {
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}

	defer sender.Commit()

	for _, store := range k.store {
		metrics := store.(*ksmstore.MetricsStore).Push()
		processMetrics(sender, metrics)
	}
	return nil
}

func processMetrics(sender aggregator.Sender, metrics map[string][]ksmstore.DDMetricsFam) {
	for _, metricsList := range metrics {
		for _, metricFamily := range metricsList {
			for _, m := range metricFamily.ListMetrics {
				sender.Gauge(metricFamily.Name, m.Val, "", joinLabels(m.Labels))
			}
		}
	}
}

func joinLabels(labels map[string]string) (tags []string) {
	for k, v := range labels {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}
	return tags
}

func KubeStateMetricsFactory() check.Check {
	return newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), &KSMConfig{})
}

func newKSMCheck(base core.CheckBase, instance *KSMConfig) *KSMCheck {
	return &KSMCheck{
		CheckBase: base,
		instance:  instance,
	}
}
