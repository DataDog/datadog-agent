// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"fmt"
	"reflect"
	"regexp"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InspectHPA returns the list of external metrics from the hpa to use for
// autoscaling. It can handle v2beta1, v2beta2 and v2 versions of HPA.
func InspectHPA(obj interface{}) []custommetrics.ExternalMetricValue {
	switch hpa := obj.(type) {
	case *autoscalingv2beta1.HorizontalPodAutoscaler:
		return inspectHPAv2beta1(hpa)
	case *autoscalingv2beta2.HorizontalPodAutoscaler:
		return inspectHPAv2beta2(hpa)
	case *autoscalingv2.HorizontalPodAutoscaler:
		return inspectHPAv2(hpa)
	default:
		log.Errorf("object is not a HorizontalPodAutoscaler, %T instead", obj)
		return nil
	}
}

func inspectHPAv2beta1(hpa *autoscalingv2beta1.HorizontalPodAutoscaler) []custommetrics.ExternalMetricValue {
	var emList []custommetrics.ExternalMetricValue
	for _, metricSpec := range hpa.Spec.Metrics {
		if metricSpec.Type != autoscalingv2beta1.ExternalMetricSourceType {
			continue
		}

		if metricSpec.External == nil {
			log.Errorf("missing required \"external\" section in the %s/%s Ref, skipping processing", hpa.Namespace, hpa.Name)
			continue
		}

		external := metricSpec.External
		em, err := buildExternalMetricValue(hpa.ObjectMeta, external.MetricName, external.MetricSelector)
		if err != nil {
			log.Errorf("cannot build external metric value for HPA %s/%s, skipping: %s", hpa.Namespace, hpa.Name, err)
			continue
		}

		emList = append(emList, em)
	}

	return emList
}

func inspectHPAv2beta2(hpa *autoscalingv2beta2.HorizontalPodAutoscaler) []custommetrics.ExternalMetricValue {
	var emList []custommetrics.ExternalMetricValue
	for _, metricSpec := range hpa.Spec.Metrics {
		if metricSpec.Type != autoscalingv2beta2.ExternalMetricSourceType {
			continue
		}

		if metricSpec.External == nil {
			log.Errorf("missing required \"external\" section in the %s/%s Ref, skipping processing", hpa.Namespace, hpa.Name)
			continue
		}

		external := metricSpec.External
		em, err := buildExternalMetricValue(hpa.ObjectMeta, external.Metric.Name, external.Metric.Selector)
		if err != nil {
			log.Errorf("cannot build external metric value for HPA %s/%s, skipping: %s", hpa.Namespace, hpa.Name, err)
			continue
		}

		emList = append(emList, em)
	}

	return emList
}

func inspectHPAv2(hpa *autoscalingv2.HorizontalPodAutoscaler) []custommetrics.ExternalMetricValue {
	var emList []custommetrics.ExternalMetricValue
	for _, metricSpec := range hpa.Spec.Metrics {
		if metricSpec.Type != autoscalingv2.ExternalMetricSourceType {
			continue
		}

		if metricSpec.External == nil {
			log.Errorf("missing required \"external\" section in the %s/%s Ref, skipping processing", hpa.Namespace, hpa.Name)
			continue
		}

		external := metricSpec.External
		em, err := buildExternalMetricValue(hpa.ObjectMeta, external.Metric.Name, external.Metric.Selector)
		if err != nil {
			log.Errorf("cannot build external metric value for HPA %s/%s, skipping: %s", hpa.Namespace, hpa.Name, err)
			continue
		}

		emList = append(emList, em)
	}

	return emList
}

func buildExternalMetricValue(meta metav1.ObjectMeta, metricName string, metricSelector *metav1.LabelSelector) (custommetrics.ExternalMetricValue, error) {
	if !IsValidMetricName(metricName) {
		return custommetrics.ExternalMetricValue{}, fmt.Errorf("metric name %q is invalid", metricName)
	}

	em := custommetrics.ExternalMetricValue{
		MetricName: metricName,
		Ref: custommetrics.ObjectReference{
			Type:      "horizontal",
			Name:      meta.Name,
			Namespace: meta.Namespace,
			UID:       string(meta.UID),
		},
	}

	if metricSelector != nil {
		em.Labels = metricSelector.MatchLabels
	}

	return em, nil
}

// InspectWPA returns the list of external metrics from the wpa to use for autoscaling.
func InspectWPA(wpa *v1alpha1.WatermarkPodAutoscaler) (emList []custommetrics.ExternalMetricValue) {
	for _, metricSpec := range wpa.Spec.Metrics {
		switch metricSpec.Type {
		case v1alpha1.ExternalMetricSourceType:
			if metricSpec.External == nil {
				log.Errorf("Missing required \"external\" section in the %s/%s WPA, skipping processing", wpa.Namespace, wpa.Name)
				continue
			}

			em := custommetrics.ExternalMetricValue{
				MetricName: metricSpec.External.MetricName,
				Ref: custommetrics.ObjectReference{
					Type:      "watermark",
					Name:      wpa.Name,
					Namespace: wpa.Namespace,
					UID:       string(wpa.UID),
				},
			}
			if metricSpec.External.MetricSelector != nil {
				em.Labels = metricSpec.External.MetricSelector.MatchLabels
			}
			emList = append(emList, em)
		default:
			log.Debugf("Unsupported metric type %s", metricSpec.Type)
		}
	}
	return
}

// DiffExternalMetrics returns the list of external metrics that reference hpas that are not in the given list of hpas.
func DiffExternalMetrics(hpaList []metav1.Object, wpaList []*v1alpha1.WatermarkPodAutoscaler, storedMetricsList []custommetrics.ExternalMetricValue) (toDelete []custommetrics.ExternalMetricValue) {
	autoscalerMetrics := map[string][]custommetrics.ExternalMetricValue{}

	for _, obj := range hpaList {
		autoscalerMetrics[string(obj.GetUID())] = InspectHPA(obj)
	}

	for _, wpa := range wpaList {
		autoscalerMetrics[string(wpa.UID)] = InspectWPA(wpa)
	}

	for _, em := range storedMetricsList {
		var found bool
		emList := autoscalerMetrics[em.Ref.UID]

		if emList == nil {
			toDelete = append(toDelete, em)
			continue
		}
		for _, m := range emList {
			// We have previously processed an external metric from this Ref.
			// Check that it's still the same. If not, remove the entry from the Global Store.
			// Use the Ref Type to get rid of the old template in the Store
			if em.MetricName == m.MetricName && reflect.DeepEqual(em.Labels, m.Labels) && em.Ref.Type == m.Ref.Type {
				found = true
				break
			}
		}
		if !found {
			toDelete = append(toDelete, em)
		}
	}
	return
}

// AutoscalerMetricsUpdate will return true if the applied configuration of the Autoscaler has changed.
// We only care about updates of the metrics or their scopes.
// We also want to process the resync events, which can be identified with the resver.
func AutoscalerMetricsUpdate(new, old metav1.Object) bool {
	var oldAnn, newAnn string
	if val, ok := old.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		oldAnn = val
	}
	if val, ok := new.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		newAnn = val
	}

	return old.GetResourceVersion() == new.GetResourceVersion() || oldAnn != newAnn
}

// IsValidMetricName will return true if the metric name follows the Datadog metric naming conventions.
// See https://docs.datadoghq.com/developers/metrics/#naming-custom-metrics
func IsValidMetricName(metricName string) bool {
	metricNamingConvention := regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9_.]{0,199}$")

	return metricNamingConvention.Match([]byte(metricName))
}
