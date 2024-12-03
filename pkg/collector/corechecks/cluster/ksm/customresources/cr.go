// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

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
	} else {
		return f.factory.CreateClient(cfg)
	}
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
