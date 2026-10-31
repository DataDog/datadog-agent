// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	ksmutil "k8s.io/kube-state-metrics/v2/pkg/util"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func findGenerator(t *testing.T, generators []generator.FamilyGenerator, name string) generator.FamilyGenerator {
	t.Helper()
	for _, g := range generators {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("generator %q not found", name)
	return generator.FamilyGenerator{}
}

func TestExtendedHorizontalPodAutoscalerFactory_Name(t *testing.T) {
	client := &apiserver.APIClient{InformerCl: fake.NewSimpleClientset()}
	factory := NewExtendedHorizontalPodAutoscalerFactory(client)

	assert.Equal(t, "horizontalpodautoscalers_extended", factory.Name())
}

func TestExtendedHorizontalPodAutoscalerFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{InformerCl: fake.NewSimpleClientset()}
	factory := NewExtendedHorizontalPodAutoscalerFactory(client)

	_, ok := factory.ExpectedType().(*autoscalingv2.HorizontalPodAutoscaler)
	assert.True(t, ok, "Expected type should be *autoscalingv2.HorizontalPodAutoscaler")
}

func TestExtendedHorizontalPodAutoscalerFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{InformerCl: fake.NewSimpleClientset()}
	factory := NewExtendedHorizontalPodAutoscalerFactory(client)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)
	assert.Equal(t, "kube_horizontalpodautoscaler_ownerref", generators[0].Name)

	t.Run("with owner reference", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hpa",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ScaledObject", Name: "my-scaledobject"},
				},
			},
		}

		family := generators[0].Generate(hpa)
		require.Len(t, family.Metrics, 1)
		m := family.Metrics[0]
		assert.Equal(t, []string{"namespace", "horizontalpodautoscaler", "ownerref_kind", "ownerref_name"}, m.LabelKeys)
		assert.Equal(t, []string{"default", "my-hpa", "scaledobject", "my-scaledobject"}, m.LabelValues)
	})

	t.Run("without owner reference", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hpa",
				Namespace: "default",
			},
		}

		family := generators[0].Generate(hpa)
		assert.Empty(t, family.Metrics)
	})
}

func TestHPAV2Beta2Factory_OwnerRefGenerator(t *testing.T) {
	client := &apiserver.APIClient{}
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(client)

	ownerRefGen := findGenerator(t, factory.MetricFamilyGenerators(), "kube_horizontalpodautoscaler_ownerref")

	t.Run("with owner reference", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hpa",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ScaledObject", Name: "my-scaledobject"},
				},
			},
		}

		family := ownerRefGen.Generate(hpa)
		require.Len(t, family.Metrics, 1)
		m := family.Metrics[0]
		assert.Equal(t, []string{"namespace", "horizontalpodautoscaler", "ownerref_kind", "ownerref_name"}, m.LabelKeys)
		assert.Equal(t, []string{"default", "my-hpa", "scaledobject", "my-scaledobject"}, m.LabelValues)
	})

	t.Run("without owner reference", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hpa",
				Namespace: "default",
			},
		}

		family := ownerRefGen.Generate(hpa)
		assert.Empty(t, family.Metrics)
	})
}

// TestHPAV2Beta2Factory_SpecTargetMetricGenerator checks that targets leaving
// some of their values unset, like an Object metric with an AverageValue
// target, don't crash the generator.
func TestHPAV2Beta2Factory_SpecTargetMetricGenerator(t *testing.T) {
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(&apiserver.APIClient{})
	targetGen := findGenerator(t, factory.MetricFamilyGenerators(), "kube_horizontalpodautoscaler_spec_target_metric")

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "my-hpa", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ObjectMetricSourceType,
					Object: &autoscalingv2.ObjectMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "requests"},
						Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: resource.NewQuantity(5, resource.DecimalSI)},
					},
				},
				{
					Type: autoscalingv2.PodsMetricSourceType,
					Pods: &autoscalingv2.PodsMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "packets"},
					},
				},
			},
		},
	}

	var family *metric.Family
	require.NotPanics(t, func() { family = targetGen.Generate(hpa) })
	require.Len(t, family.Metrics, 1)
	assert.Equal(t, []string{"namespace", "horizontalpodautoscaler", "metric_name", "metric_target_type"}, family.Metrics[0].LabelKeys)
	assert.Equal(t, []string{"default", "my-hpa", "requests", "average"}, family.Metrics[0].LabelValues)
	assert.Equal(t, float64(5), family.Metrics[0].Value)
}

// TestHPAV2Beta2Factory_ExpectedType pins the store key of the factory, which
// manageResourcesReplacement enables instead of the KSM built-in store.
func TestHPAV2Beta2Factory_ExpectedType(t *testing.T) {
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(&apiserver.APIClient{})

	_, ok := factory.ExpectedType().(*autoscalingv2.HorizontalPodAutoscaler)
	assert.True(t, ok, "Expected type should be *autoscalingv2.HorizontalPodAutoscaler")

	gvr, err := ksmutil.GVRFromType(factory.Name(), factory.ExpectedType())
	require.NoError(t, err)
	assert.Equal(t, "autoscaling/v2beta2, Resource=horizontalpodautoscalers", gvr.String())
}

func newV2beta2HPA(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2beta2",
		"kind":       "HorizontalPodAutoscaler",
		"metadata":   map[string]interface{}{"namespace": namespace, "name": name},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "name": name},
			"minReplicas":    int64(1),
			"maxReplicas":    int64(5),
			"metrics": []interface{}{map[string]interface{}{
				"type": "External",
				"external": map[string]interface{}{
					"metric": map[string]interface{}{"name": "requests_per_s"},
					"target": map[string]interface{}{"type": "Value", "value": "10"},
				},
			}},
		},
	}}
}

// TestHPAV2Beta2Factory_ListWatch checks that the autoscaling/v2beta2 HPAs
// served by the dynamic client are handed out as autoscaling/v2 ones.
func TestHPAV2Beta2Factory_ListWatch(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{hpav2beta2GVR: "HorizontalPodAutoscalerList"},
		newV2beta2HPA("default", "hpa-0"),
	)
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(&apiserver.APIClient{DynamicInformerCl: client})
	c, err := factory.CreateClient(nil)
	require.NoError(t, err)
	lw := factory.ListWatch(c, metav1.NamespaceAll, "").(cache.ListerWatcherWithContext)

	list, err := lw.ListWithContext(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	hpaList, ok := list.(*autoscalingv2.HorizontalPodAutoscalerList)
	require.True(t, ok, "list should be a *autoscalingv2.HorizontalPodAutoscalerList, got %T", list)
	require.Len(t, hpaList.Items, 1)
	hpa := hpaList.Items[0]
	assert.Equal(t, "hpa-0", hpa.Name)
	assert.Equal(t, int32(5), hpa.Spec.MaxReplicas)
	assert.Equal(t, "hpa-0", hpa.Spec.ScaleTargetRef.Name)
	require.Len(t, hpa.Spec.Metrics, 1)
	assert.Equal(t, "requests_per_s", hpa.Spec.Metrics[0].External.Metric.Name)
	assert.Equal(t, int64(10), hpa.Spec.Metrics[0].External.Target.Value.Value())

	fakeWatcher := watch.NewFakeWithChanSize(3, false)
	client.PrependWatchReactor("horizontalpodautoscalers", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, fakeWatcher, nil
	})
	w, err := lw.WatchWithContext(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	defer w.Stop()

	status := &metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonExpired}
	fakeWatcher.Add(newV2beta2HPA("default", "hpa-1"))
	fakeWatcher.Error(status)
	fakeWatcher.Modify(&unstructured.Unstructured{Object: map[string]interface{}{"spec": "invalid"}})

	nextEvent := func() watch.Event {
		select {
		case event := <-w.ResultChan():
			return event
		case <-time.After(5 * time.Second):
			require.FailNow(t, "timeout waiting for a watch event")
			return watch.Event{}
		}
	}

	event := nextEvent()
	assert.Equal(t, watch.Added, event.Type)
	addedHPA, ok := event.Object.(*autoscalingv2.HorizontalPodAutoscaler)
	require.True(t, ok, "watched object should be a *autoscalingv2.HorizontalPodAutoscaler, got %T", event.Object)
	assert.Equal(t, "hpa-1", addedHPA.Name)
	require.Len(t, addedHPA.Spec.Metrics, 1)
	assert.Equal(t, "requests_per_s", addedHPA.Spec.Metrics[0].External.Metric.Name)

	// error events are passed through untouched
	assert.Equal(t, watch.Event{Type: watch.Error, Object: status}, nextEvent())

	// objects that cannot be converted are turned into error events
	event = nextEvent()
	assert.Equal(t, watch.Error, event.Type)
	assert.IsType(t, &metav1.Status{}, event.Object)
}

func TestHPAV2Beta2Factory_InfoGeneratorHasNoOwnerRefLabels(t *testing.T) {
	client := &apiserver.APIClient{}
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(client)

	infoGen := findGenerator(t, factory.MetricFamilyGenerators(), "kube_horizontalpodautoscaler_info")

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-hpa",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ScaledObject", Name: "my-scaledobject"},
			},
		},
	}

	family := infoGen.Generate(hpa)
	require.Len(t, family.Metrics, 1)
	assert.NotContains(t, family.Metrics[0].LabelKeys, "ownerref_kind")
	assert.NotContains(t, family.Metrics[0].LabelKeys, "ownerref_name")
}
