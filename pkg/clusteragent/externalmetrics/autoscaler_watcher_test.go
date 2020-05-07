// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"testing"
	"time"

	autoscaler "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kube_informer "k8s.io/client-go/informers"
	kube_fake "k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	datadoghq "github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
	dd_fake_clientset "github.com/DataDog/watermarkpodautoscaler/pkg/client/clientset/versioned/fake"
	wpa_informer "github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions"

	"github.com/stretchr/testify/assert"
)

// Test fixture
type autoscalerFixture struct {
	t *testing.T

	// Objects to put in the store.
	hpaLister []*autoscaler.HorizontalPodAutoscaler
	wpaLister []*datadoghq.WatermarkPodAutoscaler
	// Objects from here preloaded into fake clients.
	kubeObjects []runtime.Object
	wpaObjects  []runtime.Object
	// Local store.
	store DatadogMetricsInternalStore
}

func newAutoscalerFixture(t *testing.T) *autoscalerFixture {
	return &autoscalerFixture{
		t:           t,
		kubeObjects: []runtime.Object{},
		wpaObjects:  []runtime.Object{},
		store:       NewDatadogMetricsInternalStore(),
	}
}

func (f *autoscalerFixture) newAutoscalerWatcher() (*AutoscalerWatcher, kube_informer.SharedInformerFactory, wpa_informer.SharedInformerFactory) {
	for _, hpa := range f.hpaLister {
		f.kubeObjects = append(f.kubeObjects, hpa)
	}
	kubeClient := kube_fake.NewSimpleClientset(f.kubeObjects...)
	kubeInformer := kube_informer.NewSharedInformerFactory(kubeClient, noResyncPeriodFunc())

	for _, wpa := range f.wpaLister {
		f.wpaObjects = append(f.wpaObjects, wpa)
	}
	wpaClient := dd_fake_clientset.NewSimpleClientset(f.wpaObjects...)
	wpaInformer := wpa_informer.NewSharedInformerFactory(wpaClient, noResyncPeriodFunc())

	autoscalerWatcher, err := NewAutoscalerWatcher(0, 1, "default", kubeInformer, wpaInformer, getIsLeaderFunction(true), &f.store)
	if err != nil {
		return nil, nil, nil
	}
	autoscalerWatcher.autoscalerListerSynced = alwaysReady
	autoscalerWatcher.wpaListerSynced = alwaysReady

	for _, hpa := range f.hpaLister {
		kubeInformer.Autoscaling().V2beta1().HorizontalPodAutoscalers().Informer().GetIndexer().Add(hpa)
	}

	for _, wpa := range f.wpaLister {
		wpaInformer.Datadoghq().V1alpha1().WatermarkPodAutoscalers().Informer().GetIndexer().Add(wpa)
	}

	return autoscalerWatcher, kubeInformer, wpaInformer
}

func (f *autoscalerFixture) runWatcherUpdate() {
	autoscalerWatcher, kubeInformer, wpaInformer := f.newAutoscalerWatcher()
	stopCh := make(chan struct{})
	defer close(stopCh)
	kubeInformer.Start(stopCh)
	wpaInformer.Start(stopCh)

	autoscalerWatcher.processAutoscalers()
}

func newFakeHorizontalPodAutoscaler(ns, name string, metrics []autoscaler.MetricSpec) *autoscaler.HorizontalPodAutoscaler {
	return &autoscaler.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: autoscaler.HorizontalPodAutoscalerSpec{
			Metrics: metrics,
		},
	}
}

func newFakeWatermarkPodAutoscaler(ns, name string, metrics []datadoghq.MetricSpec) *datadoghq.WatermarkPodAutoscaler {
	return &datadoghq.WatermarkPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: datadoghq.WatermarkPodAutoscalerSpec{
			Metrics: metrics,
		},
	}
}

func TestUpdateAutoscalerReferences(t *testing.T) {
	f := newAutoscalerFixture(t)
	updateTime := time.Now()

	f.hpaLister = []*autoscaler.HorizontalPodAutoscaler{
		newFakeHorizontalPodAutoscaler("ns0", "hpa0", []autoscaler.MetricSpec{
			{
				Type: autoscaler.ExternalMetricSourceType,
				External: &autoscaler.ExternalMetricSource{
					MetricName: "datadogmetric@default:dd-metric-0",
				},
			},
		}),
		newFakeHorizontalPodAutoscaler("ns1", "hpa1", []autoscaler.MetricSpec{
			{
				Type: autoscaler.ResourceMetricSourceType,
			},
		}),
	}

	f.wpaLister = []*datadoghq.WatermarkPodAutoscaler{
		newFakeWatermarkPodAutoscaler("ns0", "wpa0", []datadoghq.MetricSpec{
			{
				Type: datadoghq.ExternalMetricSourceType,
				External: &datadoghq.ExternalMetricSource{
					MetricName: "datadogmetric@default:dd-metric-1",
				},
			},
		}),
	}

	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     false,
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")
	f.store.Set("default/dd-metric-1", model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Active:     false,
		Query:      "metric query1",
		Valid:      true,
		Value:      11.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")
	f.store.Set("default/dd-metric-2", model.DatadogMetricInternal{
		ID:         "default/dd-metric-2",
		Active:     true,
		Query:      "metric query2",
		Valid:      true,
		Value:      12.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")

	f.runWatcherUpdate()

	// Check internal store content
	assert.Equal(t, 3, f.store.Count())
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     true,
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Active:     true,
		Query:      "metric query1",
		Valid:      true,
		Value:      11.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-1"))
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-2",
		Active:     false,
		Query:      "metric query2",
		Valid:      false,
		Value:      12.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-2"))
}

func TestCreateAutogenDatadogMetrics(t *testing.T) {
	f := newAutoscalerFixture(t)
	updateTime := time.Now()

	f.hpaLister = []*autoscaler.HorizontalPodAutoscaler{
		newFakeHorizontalPodAutoscaler("ns0", "hpa0", []autoscaler.MetricSpec{
			{
				Type: autoscaler.ExternalMetricSourceType,
				External: &autoscaler.ExternalMetricSource{
					MetricName: "docker.cpu.usage",
					MetricSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		}),
	}

	f.wpaLister = []*datadoghq.WatermarkPodAutoscaler{
		newFakeWatermarkPodAutoscaler("ns0", "wpa0", []datadoghq.MetricSpec{
			{
				Type: datadoghq.ExternalMetricSourceType,
				External: &datadoghq.ExternalMetricSource{
					MetricName: "docker.cpu.usage",
					MetricSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"bar": "foo",
						},
					},
				},
			},
		}),
	}

	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     true,
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")

	f.runWatcherUpdate()

	// Check internal store content
	assert.Equal(t, 3, f.store.Count())
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     false,
		Query:      "metric query0",
		Valid:      false,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d2265",
		Active:             true,
		Query:              "avg:docker.cpu.usage{foo:bar}.rollup(30)",
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Value:              0.0,
		UpdateTime:         updateTime,
		Error:              nil,
	}, f.store.Get("default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d2265"))
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412f8",
		Active:             true,
		Query:              "avg:docker.cpu.usage{bar:foo}.rollup(30)",
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Value:              0.0,
		UpdateTime:         updateTime,
		Error:              nil,
	}, f.store.Get("default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412f8"))
}

func TestCleanUpAutogenDatadogMetrics(t *testing.T) {
	f := newAutoscalerFixture(t)
	// AutogenExpirationPeriod is set to 1 hour in our unit tests
	oldUpdateTime := time.Now().Add(time.Duration(-30) * time.Minute)
	expiredUpdateTime := time.Now().Add(time.Duration(-90) * time.Minute)

	// This DatadogMetric is expired, but it's not an autogen one - should not touch it
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     true,
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: expiredUpdateTime,
		Error:      nil,
	}, "utest")
	// HPA has been deleted but last update time was 30 minutes ago, we should keep it
	f.store.Set("default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d2265", model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d2265",
		Active:             true,
		Query:              "avg:docker.cpu.usage{foo:bar}.rollup(30)",
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            false,
		Value:              0.0,
		UpdateTime:         oldUpdateTime,
		Error:              nil,
	}, "utest")
	// WPA has been deleted for 90 minutes, we should flag this as deleted
	f.store.Set("default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412f8", model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412f8",
		Active:             true,
		Query:              "avg:docker.cpu.usage{bar:foo}.rollup(30)",
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            true,
		Value:              0.0,
		UpdateTime:         expiredUpdateTime,
		Error:              nil,
	}, "utest")

	f.runWatcherUpdate()

	// Check internal store content
	assert.Equal(t, 3, f.store.Count())
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     false,
		Query:      "metric query0",
		Valid:      false,
		Deleted:    false,
		Value:      10.0,
		UpdateTime: expiredUpdateTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d2265",
		Active:             false,
		Query:              "avg:docker.cpu.usage{foo:bar}.rollup(30)",
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            false,
		Value:              0.0,
		UpdateTime:         oldUpdateTime,
		Error:              nil,
	}, f.store.Get("default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d2265"))
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412f8",
		Active:             false,
		Query:              "avg:docker.cpu.usage{bar:foo}.rollup(30)",
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            true,
		Value:              0.0,
		UpdateTime:         expiredUpdateTime,
		Error:              nil,
	}, f.store.Get("default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412f8"))
}
