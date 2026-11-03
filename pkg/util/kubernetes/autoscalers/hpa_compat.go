// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

// This file supports Kubernetes < 1.23 clusters, which do not serve
// autoscaling/v2 HPAs but only autoscaling/v2beta2 and/or
// autoscaling/v2beta1 ones. The types, typed clients and informers of those
// versions are gone from k8s.io/api and k8s.io/client-go since v0.36, so these
// HPAs are watched with a dynamic informer and converted to autoscaling/v2 when
// read. It can be removed once support for those Kubernetes versions is
// officially dropped.

import (
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
)

// HPAInformer returns the informer to watch HPAs with the GroupVersionResource
// returned by DiscoverHPAGroupVersionResource. Use ToHPAv2 to read the objects
// it hands out. The dynamic informer factory is only needed on clusters that do
// not serve autoscaling/v2.
func HPAInformer(hpaGVR schema.GroupVersionResource, informerFactory informers.SharedInformerFactory, dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) (informers.GenericInformer, error) {
	if hpaGVR.GroupVersion() == autoscalingv2.SchemeGroupVersion {
		return informerFactory.ForResource(hpaGVR)
	}

	if dynamicInformerFactory == nil {
		return nil, fmt.Errorf("a dynamic informer factory is required to watch %s", hpaGVR)
	}
	return dynamicInformerFactory.ForResource(hpaGVR), nil
}

// ToHPAv2 returns an HPA handed out by an informer returned by HPAInformer as an
// autoscaling/v2 HPA.
func ToHPAv2(obj interface{}) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	switch hpa := obj.(type) {
	case *autoscalingv2.HorizontalPodAutoscaler:
		return hpa, nil
	case *unstructured.Unstructured:
		return hpaFromUnstructured(hpa)
	default:
		return nil, fmt.Errorf("object is not a HorizontalPodAutoscaler, %T instead", obj)
	}
}

func hpaFromUnstructured(obj *unstructured.Unstructured) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	// autoscaling/v2beta2 and autoscaling/v2 share the same schema, the latter
	// only adds optional fields. So does autoscaling/v2beta1, except for the
	// metric sources, which are converted below.
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), hpa); err != nil {
		return nil, fmt.Errorf("cannot convert %s HorizontalPodAutoscaler %s/%s: %w", obj.GetAPIVersion(), obj.GetNamespace(), obj.GetName(), err)
	}

	switch apiVersion := obj.GetAPIVersion(); apiVersion {
	case "autoscaling/v2beta2":
		return hpa, nil
	case "autoscaling/v2beta1":
		var v2beta1 struct {
			Spec struct {
				Metrics []v2beta1MetricSpec `json:"metrics"`
			} `json:"spec"`
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &v2beta1); err != nil {
			return nil, fmt.Errorf("cannot convert %s HorizontalPodAutoscaler %s/%s: %w", apiVersion, obj.GetNamespace(), obj.GetName(), err)
		}

		hpa.Spec.Metrics = make([]autoscalingv2.MetricSpec, 0, len(v2beta1.Spec.Metrics))
		for _, metric := range v2beta1.Spec.Metrics {
			hpa.Spec.Metrics = append(hpa.Spec.Metrics, metric.toV2())
		}
		// The current metrics are not used by the agent, don't bother converting them.
		hpa.Status.CurrentMetrics = nil
		return hpa, nil
	default:
		return nil, fmt.Errorf("unsupported HorizontalPodAutoscaler version %s", apiVersion)
	}
}

// v2beta1MetricSpec is an autoscaling/v2beta1 MetricSpec.
type v2beta1MetricSpec struct {
	Type              autoscalingv2.MetricSourceType `json:"type"`
	Object            *v2beta1MetricSource           `json:"object,omitempty"`
	Pods              *v2beta1MetricSource           `json:"pods,omitempty"`
	Resource          *v2beta1MetricSource           `json:"resource,omitempty"`
	ContainerResource *v2beta1MetricSource           `json:"containerResource,omitempty"`
	External          *v2beta1MetricSource           `json:"external,omitempty"`
}

// v2beta1MetricSource holds the fields of all the autoscaling/v2beta1 metric
// sources: object, pods, resource, containerResource and external.
type v2beta1MetricSource struct {
	Target                   autoscalingv2.CrossVersionObjectReference `json:"target"`
	Name                     corev1.ResourceName                       `json:"name"`
	Container                string                                    `json:"container"`
	MetricName               string                                    `json:"metricName"`
	Selector                 *metav1.LabelSelector                     `json:"selector,omitempty"`
	MetricSelector           *metav1.LabelSelector                     `json:"metricSelector,omitempty"`
	TargetValue              *resource.Quantity                        `json:"targetValue,omitempty"`
	TargetAverageValue       *resource.Quantity                        `json:"targetAverageValue,omitempty"`
	TargetAverageUtilization *int32                                    `json:"targetAverageUtilization,omitempty"`
	AverageValue             *resource.Quantity                        `json:"averageValue,omitempty"`
}

// toV2 follows the conversion done by the API server, see
// https://github.com/kubernetes/kubernetes/blob/v1.35.0/pkg/apis/autoscaling/v2beta1/conversion.go
func (m v2beta1MetricSpec) toV2() autoscalingv2.MetricSpec {
	out := autoscalingv2.MetricSpec{Type: m.Type}

	if s := m.Object; s != nil {
		targetType := autoscalingv2.ValueMetricType
		if s.AverageValue != nil {
			targetType = autoscalingv2.AverageValueMetricType
		}
		out.Object = &autoscalingv2.ObjectMetricSource{
			DescribedObject: s.Target,
			Metric:          autoscalingv2.MetricIdentifier{Name: s.MetricName, Selector: s.Selector},
			Target:          autoscalingv2.MetricTarget{Type: targetType, Value: s.TargetValue, AverageValue: s.AverageValue},
		}
	}

	if s := m.Pods; s != nil {
		out.Pods = &autoscalingv2.PodsMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: s.MetricName, Selector: s.Selector},
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: s.TargetAverageValue},
		}
	}

	if s := m.Resource; s != nil {
		out.Resource = &autoscalingv2.ResourceMetricSource{Name: s.Name, Target: s.resourceTarget()}
	}

	if s := m.ContainerResource; s != nil {
		out.ContainerResource = &autoscalingv2.ContainerResourceMetricSource{Name: s.Name, Container: s.Container, Target: s.resourceTarget()}
	}

	if s := m.External; s != nil {
		targetType := autoscalingv2.AverageValueMetricType
		if s.TargetValue != nil {
			targetType = autoscalingv2.ValueMetricType
		}
		out.External = &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: s.MetricName, Selector: s.MetricSelector},
			Target: autoscalingv2.MetricTarget{Type: targetType, Value: s.TargetValue, AverageValue: s.TargetAverageValue},
		}
	}

	return out
}

func (s *v2beta1MetricSource) resourceTarget() autoscalingv2.MetricTarget {
	targetType := autoscalingv2.AverageValueMetricType
	if s.TargetAverageUtilization != nil {
		targetType = autoscalingv2.UtilizationMetricType
	}
	return autoscalingv2.MetricTarget{Type: targetType, AverageValue: s.TargetAverageValue, AverageUtilization: s.TargetAverageUtilization}
}
