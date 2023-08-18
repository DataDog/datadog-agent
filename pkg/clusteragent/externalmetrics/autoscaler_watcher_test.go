// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	autoscaler "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamic_informer "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	kube_informer "k8s.io/client-go/informers"
	kube_fake "k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
)

const (
	autoscalingGroup = "autoscaling"
	hpaResource      = "horizontalpodautoscalers"
)

func init() {
	autoscaler.AddToScheme(scheme)
	v1alpha1.AddToScheme(scheme)
}

// Test fixture
type autoscalerFixture struct {
	t *testing.T

	// Objects to put in the store.
	hpaLister []*autoscaler.HorizontalPodAutoscaler
	wpaLister []*unstructured.Unstructured
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

func (f *autoscalerFixture) newAutoscalerWatcher() (*AutoscalerWatcher, kube_informer.SharedInformerFactory, dynamic_informer.DynamicSharedInformerFactory) {
	for _, hpa := range f.hpaLister {
		f.kubeObjects = append(f.kubeObjects, hpa)
	}
	kubeClient := kube_fake.NewSimpleClientset(f.kubeObjects...)
	kubeClient.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: fmt.Sprintf("%s/%s", autoscalingGroup, "v2beta1"),
			APIResources: []metav1.APIResource{
				{
					Name:    hpaResource,
					Group:   autoscalingGroup,
					Version: "v2beta1",
				},
			},
		},
	}
	kubeInformer := kube_informer.NewSharedInformerFactory(kubeClient, noResyncPeriodFunc())

	for _, wpa := range f.wpaLister {
		f.wpaObjects = append(f.wpaObjects, wpa)
	}
	wpaClient := fake.NewSimpleDynamicClient(scheme, f.wpaObjects...)
	wpaInformer := dynamic_informer.NewDynamicSharedInformerFactory(wpaClient, noResyncPeriodFunc())

	autoscalerWatcher, err := NewAutoscalerWatcher(0, true, 1, "default", kubeClient, kubeInformer, wpaInformer, getIsLeaderFunction(true), &f.store)
	if err != nil {
		return nil, nil, nil
	}
	autoscalerWatcher.autoscalerListerSynced = alwaysReady
	autoscalerWatcher.wpaListerSynced = alwaysReady

	for _, hpa := range f.hpaLister {
		kubeInformer.Autoscaling().V2beta1().HorizontalPodAutoscalers().Informer().GetIndexer().Add(hpa)
	}

	for _, wpa := range f.wpaLister {
		wpaInformer.ForResource(gvr).Informer().GetIndexer().Add(wpa)
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

func newFakeWatermarkPodAutoscaler(ns, name string, metrics []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "datadoghq.com/v1alpha1",
			"kind":       "WatermarkPodAutoscaler",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"metrics": metrics,
			},
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

	f.wpaLister = []*unstructured.Unstructured{
		newFakeWatermarkPodAutoscaler("ns0", "wpa0", []interface{}{
			map[string]interface{}{
				"external": map[string]interface{}{
					"metricName": "datadogmetric@default:dd-metric-1",
				},
				"type": "External",
			},
		}),
	}

	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     false,
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Active:     true,
		Valid:      true,
		Value:      11.0,
		UpdateTime: updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query1")
	f.store.Set("default/dd-metric-1", ddm, "utest")

	ddm = model.DatadogMetricInternal{
		ID:                   "default/dd-metric-2",
		Active:               true,
		Valid:                true,
		Value:                12.0,
		UpdateTime:           updateTime,
		AutoscalerReferences: "hpa:ns1/hpa1",
		Error:                nil,
	}
	ddm.SetQueries("metric query2")
	f.store.Set("default/dd-metric-2", ddm, "utest")

	f.runWatcherUpdate()

	// Check internal store content
	assert.Equal(t, 3, f.store.Count())
	ddm = model.DatadogMetricInternal{
		ID:                   "default/dd-metric-0",
		Active:               true,
		Valid:                true,
		Value:                10.0,
		UpdateTime:           updateTime,
		Error:                nil,
		AutoscalerReferences: "hpa:ns0/hpa0",
	}
	ddm.SetQueries("metric query0")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dd-metric-0"))

	ddm = model.DatadogMetricInternal{
		ID:                   "default/dd-metric-1",
		Active:               true,
		Valid:                true,
		Value:                11.0,
		UpdateTime:           updateTime,
		Error:                nil,
		AutoscalerReferences: "wpa:ns0/wpa0",
	}
	ddm.SetQueries("metric query1")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dd-metric-1"))

	ddm = model.DatadogMetricInternal{
		ID:                   "default/dd-metric-2",
		Active:               false,
		Valid:                false,
		Value:                12.0,
		UpdateTime:           updateTime,
		Error:                nil,
		AutoscalerReferences: "",
	}
	ddm.SetQueries("metric query2")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dd-metric-2"))
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
		newFakeHorizontalPodAutoscaler("ns0", "hpa1", []autoscaler.MetricSpec{
			{
				Type: autoscaler.ExternalMetricSourceType,
				External: &autoscaler.ExternalMetricSource{
					MetricName: "datadogmetric@ns0:donotexist",
				},
			},
		}),
	}

	f.wpaLister = []*unstructured.Unstructured{
		newFakeWatermarkPodAutoscaler("ns0", "wpa0", []interface{}{
			map[string]interface{}{
				"external": map[string]interface{}{
					"metricName": "docker.cpu.usage",
					"metricSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"bar": "foo",
						},
					},
				},
				"type": "External",
			},
		}),
	}

	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     true,
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}
	ddm.SetQuery("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	f.runWatcherUpdate()

	// Check internal store content
	assert.Equal(t, 3, f.store.Count())
	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     false,
		Valid:      false,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}
	ddm.SetQuery("metric query0")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dd-metric-0"))

	ddm = model.DatadogMetricInternal{
		ID:                   "default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d22",
		Active:               true,
		Valid:                false,
		Autogen:              true,
		ExternalMetricName:   "docker.cpu.usage",
		Value:                0.0,
		UpdateTime:           updateTime,
		Error:                nil,
		AutoscalerReferences: "hpa:ns0/hpa0",
	}
	ddm.SetQuery("avg:docker.cpu.usage{foo:bar}.rollup(30)")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d22"))

	ddm = model.DatadogMetricInternal{
		ID:                   "default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412",
		Active:               true,
		Valid:                false,
		Autogen:              true,
		ExternalMetricName:   "docker.cpu.usage",
		Value:                0.0,
		UpdateTime:           updateTime,
		Error:                nil,
		AutoscalerReferences: "wpa:ns0/wpa0",
	}
	ddm.SetQuery("avg:docker.cpu.usage{bar:foo}.rollup(30)")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412"))
}

func TestDisableDatadogMetricAutogen(t *testing.T) {
	f := newAutoscalerFixture(t)

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

	f.wpaLister = []*unstructured.Unstructured{
		newFakeWatermarkPodAutoscaler("ns0", "wpa0", []interface{}{
			map[string]interface{}{
				"external": map[string]interface{}{
					"metricName": "docker.cpu.usage",
					"metricSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"bar": "foo",
						},
					},
				},
				"type": "External",
			},
		}),
	}

	autoscalerWatcher, kubeInformer, wpaInformer := f.newAutoscalerWatcher()
	autoscalerWatcher.autogenEnabled = false

	stopCh := make(chan struct{})
	defer close(stopCh)
	kubeInformer.Start(stopCh)
	wpaInformer.Start(stopCh)

	autoscalerWatcher.processAutoscalers()

	// The non-DatadogMetric HPA and WPA should not create any autogenerated
	// DatadogMetrics; they should be ignored.
	assert.Equal(t, 0, f.store.Count())
}

func TestCleanUpAutogenDatadogMetrics(t *testing.T) {
	f := newAutoscalerFixture(t)
	// AutogenExpirationPeriod is set to 1 hour in our unit tests
	oldUpdateTime := time.Now().Add(time.Duration(-30) * time.Minute)
	expiredUpdateTime := time.Now().Add(time.Duration(-90) * time.Minute)

	// This DatadogMetric is expired, but it's not an autogen one - should not touch it
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     true,
		Valid:      true,
		Value:      10.0,
		UpdateTime: expiredUpdateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	// HPA has been deleted but last update time was 30 minutes ago, we should keep it
	ddm = model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d22",
		Active:             true,
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            false,
		Value:              0.0,
		UpdateTime:         oldUpdateTime,
		Error:              nil,
	}
	ddm.SetQueries("avg:docker.cpu.usage{foo:bar}.rollup(30)")
	f.store.Set("default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d22", ddm, "utest")

	// WPA has been deleted for 90 minutes, we should flag this as deleted
	ddm = model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412",
		Active:             true,
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            true,
		Value:              0.0,
		UpdateTime:         expiredUpdateTime,
		Error:              nil,
	}
	ddm.SetQueries("avg:docker.cpu.usage{bar:foo}.rollup(30)")
	f.store.Set("default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412", ddm, "utest")

	f.runWatcherUpdate()

	// Check internal store content
	assert.Equal(t, 3, f.store.Count())
	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Active:     false,
		Valid:      false,
		Deleted:    false,
		Value:      10.0,
		UpdateTime: expiredUpdateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dd-metric-0"))

	ddm = model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d22",
		Active:             false,
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            false,
		Value:              0.0,
		UpdateTime:         oldUpdateTime,
		Error:              nil,
	}
	ddm.SetQueries("avg:docker.cpu.usage{foo:bar}.rollup(30)")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dcaautogen-f311ac1e6b29e3723d1445645c43afd4340d22"))

	ddm = model.DatadogMetricInternal{
		ID:                 "default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412",
		Active:             false,
		Valid:              false,
		Autogen:            true,
		ExternalMetricName: "docker.cpu.usage",
		Deleted:            true,
		Value:              0.0,
		UpdateTime:         expiredUpdateTime,
		Error:              nil,
	}
	ddm.SetQueries("avg:docker.cpu.usage{bar:foo}.rollup(30)")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dcaautogen-b6ea72b610c00aba6791b5eca1912e68dc7412"))
}
