// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

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
	client := &apiserver.APIClient{Cl: fake.NewSimpleClientset()}
	factory := NewExtendedHorizontalPodAutoscalerFactory(client)

	assert.Equal(t, "horizontalpodautoscalers_extended", factory.Name())
}

func TestExtendedHorizontalPodAutoscalerFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{Cl: fake.NewSimpleClientset()}
	factory := NewExtendedHorizontalPodAutoscalerFactory(client)

	_, ok := factory.ExpectedType().(*autoscalingv2.HorizontalPodAutoscaler)
	assert.True(t, ok, "Expected type should be *autoscalingv2.HorizontalPodAutoscaler")
}

func TestExtendedHorizontalPodAutoscalerFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{Cl: fake.NewSimpleClientset()}
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
	client := &apiserver.APIClient{Cl: fake.NewSimpleClientset()}
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(client)

	ownerRefGen := findGenerator(t, factory.MetricFamilyGenerators(), "kube_horizontalpodautoscaler_ownerref")

	t.Run("with owner reference", func(t *testing.T) {
		hpa := &autoscalingv2beta2.HorizontalPodAutoscaler{
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
		hpa := &autoscalingv2beta2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hpa",
				Namespace: "default",
			},
		}

		family := ownerRefGen.Generate(hpa)
		assert.Empty(t, family.Metrics)
	})
}

func TestHPAV2Beta2Factory_InfoGeneratorHasNoOwnerRefLabels(t *testing.T) {
	client := &apiserver.APIClient{Cl: fake.NewSimpleClientset()}
	factory := NewHorizontalPodAutoscalerV2Beta2Factory(client)

	infoGen := findGenerator(t, factory.MetricFamilyGenerators(), "kube_horizontalpodautoscaler_info")

	hpa := &autoscalingv2beta2.HorizontalPodAutoscaler{
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
