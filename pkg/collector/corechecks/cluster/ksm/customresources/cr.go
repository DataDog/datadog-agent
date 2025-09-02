// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/customresourcestate"
	"k8s.io/kube-state-metrics/v2/pkg/discovery"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

// NewCustomResourceFactory returns a new custom resource factory that uses the provided client for all CRDs
func NewCustomResourceFactory(factory customresource.RegistryFactory, client dynamic.Interface) customresource.RegistryFactory {
	return &crFactory{
		factory: factory,
		client:  client,
	}
}

type crFactory struct {
	factory customresource.RegistryFactory
	client  dynamic.Interface
}

func (f *crFactory) Name() string {
	return f.factory.Name()
}

// Hack to force the re-use of our own client for all CRDs
func (f *crFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	if u, ok := f.factory.ExpectedType().(*unstructured.Unstructured); ok {
		gvr := schema.GroupVersionResource{
			Group:    u.GroupVersionKind().Group,
			Version:  u.GroupVersionKind().Version,
			Resource: f.factory.Name(),
		}
		return f.client.Resource(gvr), nil
	}
	return f.factory.CreateClient(cfg)
}

func (f *crFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return f.factory.MetricFamilyGenerators()
}

func (f *crFactory) ExpectedType() interface{} {
	return f.factory.ExpectedType()
}

func (f *crFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	return f.factory.ListWatch(customResourceClient, ns, fieldSelector)
}

// GetCustomMetricNamesMapper returns a map KSM metric names to Datadog metric names for custom resources
func GetCustomMetricNamesMapper(resources []customresourcestate.Resource) (mapper map[string]string) {
	mapper = make(map[string]string)

	for _, customResource := range resources {
		for _, generator := range customResource.Metrics {
			if generator.Each.Type == metric.Gauge ||
				generator.Each.Type == metric.StateSet {
				if customResource.GetMetricNamePrefix() == "kube_customresource" {
					mapper[customResource.GetMetricNamePrefix()+"_"+generator.Name] = "customresource." + generator.Name
				} else {
					mapper[customResource.GetMetricNamePrefix()+"_"+generator.Name] = "customresource." + customResource.GetMetricNamePrefix() + "_" + generator.Name
				}
			}
		}
	}

	return mapper
}

// Those Prometheus counters are currently not used, but they are required by the KSM API
var (
	crdsAddEventsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "kube_state_metrics_custom_resource_state_add_events_total",
		Help: "Number of times that the CRD informer triggered the add event.",
	})
	crdsDeleteEventsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "kube_state_metrics_custom_resource_state_delete_events_total",
		Help: "Number of times that the CRD informer triggered the remove event.",
	})
	crdsCacheCountGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kube_state_metrics_custom_resource_state_cache",
		Help: "Net amount of CRDs affecting the cache currently.",
	})
)

type customResourceDecoder struct {
	data customresourcestate.Metrics
}

// Decode decodes the custom resource state metrics configuration.
func (d customResourceDecoder) Decode(v interface{}) error {
	return mapstructure.Decode(d.data, v)
}

// GetCustomResourceFactories returns a list of custom resource factories
func GetCustomResourceFactories(resources customresourcestate.Metrics, c *apiserver.APIClient) (factories []customresource.RegistryFactory) {
	discovererInstance := &discovery.CRDiscoverer{
		CRDsAddEventsCounter:    crdsAddEventsCounter,
		CRDsDeleteEventsCounter: crdsDeleteEventsCounter,
		CRDsCacheCountGauge:     crdsCacheCountGauge,
	}
	clientConfig, err := apiserver.GetClientConfig(time.Duration(setup.Datadog().GetInt64("kubernetes_apiserver_client_timeout"))*time.Second, 10, 20)
	if err != nil {
		panic(err)
	}
	if err := discovererInstance.StartDiscovery(context.Background(), clientConfig); err != nil {
		log.Errorf("failed to start custom resource discovery: %v", err)
	}
	customResourceStateMetricFactoriesFunc, err := customresourcestate.FromConfig(customResourceDecoder{resources}, discovererInstance)

	if err != nil {
		log.Errorf("failed to create custom resource state metrics: %v", err)
	} else {
		customResourceStateMetricFactories, err := customResourceStateMetricFactoriesFunc()
		if err != nil {
			log.Errorf("failed to create custom resource state metrics: %v", err)
		} else {
			factories = make([]customresource.RegistryFactory, 0, len(customResourceStateMetricFactories))
			for _, factory := range customResourceStateMetricFactories {
				factories = append(factories, NewCustomResourceFactory(factory, c.DynamicCl))
			}
		}
	}

	return factories
}

// GetCustomResourceClientsAndCollectors returns a map of custom resource clients and a list of collectors
func GetCustomResourceClientsAndCollectors(resources []customresourcestate.Resource, c *apiserver.APIClient) (clients map[string]interface{}, collectors []string) {
	clients = make(map[string]interface{})
	collectors = make([]string, 0, len(resources))

	for _, cr := range resources {
		gvr := schema.GroupVersionResource{
			Group:    cr.GroupVersionKind.Group,
			Version:  cr.GroupVersionKind.Version,
			Resource: cr.GetResourceName(),
		}

		cl := c.DynamicCl.Resource(gvr)
		clients[cr.GetResourceName()] = cl
		collectors = append(collectors, gvr.String())
	}

	return clients, collectors
}
