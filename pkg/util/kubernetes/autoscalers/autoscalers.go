// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package autoscalers

import (
	"reflect"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
)

// InspectHPA returns the list of external metrics from the hpa to use for autoscaling.
func InspectHPA(hpa *autoscalingv2.HorizontalPodAutoscaler) (emList []custommetrics.ExternalMetricValue) {
	for _, metricSpec := range hpa.Spec.Metrics {
		switch metricSpec.Type {
		case autoscalingv2.ExternalMetricSourceType:
			if metricSpec.External == nil {
				log.Errorf("Missing required \"external\" section in the %s/%s Ref, skipping processing", hpa.Namespace, hpa.Name)
				continue
			}

			em := custommetrics.ExternalMetricValue{
				MetricName: metricSpec.External.MetricName,
				Ref: custommetrics.ObjectReference{
					Type:      "horizontal",
					Name:      hpa.Name,
					Namespace: hpa.Namespace,
					UID:       string(hpa.UID),
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
func DiffExternalMetrics(informerList []*autoscalingv2.HorizontalPodAutoscaler, wpaInformerList []*v1alpha1.WatermarkPodAutoscaler, storedMetricsList []custommetrics.ExternalMetricValue) (toDelete []custommetrics.ExternalMetricValue) {
	autoscalerMetrics := map[string][]custommetrics.ExternalMetricValue{}

	for _, hpa := range informerList {
		autoscalerMetrics[string(hpa.UID)] = InspectHPA(hpa)
	}

	for _, wpa := range wpaInformerList {
		autoscalerMetrics[string(wpa.UID)] = InspectWPA(wpa)
	}

	for _, em := range storedMetricsList {
		log.Infof("Evaluating DiffExternalM %v", em)
		var found bool
		emList := autoscalerMetrics[em.Ref.UID]
		log.Infof("Evaluating enList  DiffExternalM %v", emList)

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
func AutoscalerMetricsUpdate(new, old *autoscalingv2.HorizontalPodAutoscaler) bool {
	var oldAnn, newAnn string
	if val, ok := old.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		oldAnn = val
	}
	if val, ok := new.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		newAnn = val
	}

	return old.ResourceVersion == new.ResourceVersion || oldAnn != newAnn
}

// WPAutoscalerMetricsUpdate will return true if the applied configuration of the Autoscaler has changed.
// We only care about updates of the metrics or their scopes.
// We also want to process the resync events, which can be identified with the resver.
func WPAutoscalerMetricsUpdate(new, old *v1alpha1.WatermarkPodAutoscaler) bool {
	var oldAnn, newAnn string
	if val, ok := old.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		oldAnn = val
	}
	if val, ok := new.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		newAnn = val
	}

	return old.ResourceVersion == new.ResourceVersion || oldAnn != newAnn
}
