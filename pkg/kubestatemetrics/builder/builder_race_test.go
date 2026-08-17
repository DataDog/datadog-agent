// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package builder

import (
	"sync"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

// fakeFactory is a minimal customresource.RegistryFactory implementation for
// testing. It lets us call WithCustomResourceStoreFactories without pulling in
// real Kubernetes clients.
type fakeFactory struct {
	name string
}

func (f *fakeFactory) Name() string { return f.name }
func (f *fakeFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return nil, nil
}
func (f *fakeFactory) MetricFamilyGenerators() []generator.FamilyGenerator { return nil }
func (f *fakeFactory) ExpectedType() interface{}                           { return nil }
func (f *fakeFactory) ListWatch(_ interface{}, _ string, _ string) cache.ListerWatcher {
	return nil
}

var _ customresource.RegistryFactory = &fakeFactory{}

// TestWithCustomResourceStoreFactoriesConcurrent verifies that concurrent
// calls to WithCustomResourceStoreFactories on different Builder instances
// don't race on the kube-state-metrics library's package-level availableStores
// map. Without the ksmBuilderMu lock, this test fails under -race.
func TestWithCustomResourceStoreFactoriesConcurrent(t *testing.T) {
	t.Parallel()

	const goroutines = 100
	var wg sync.WaitGroup

	for range goroutines {
		wg.Go(func() {
			b := New()
			b.WithCustomResourceStoreFactories(&fakeFactory{name: "fake-resource"})
		})
	}

	wg.Wait()
}
