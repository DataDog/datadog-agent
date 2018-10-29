// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"reflect"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Inspect returns the list of external metrics from the hpa to use for autoscaling.
func Inspect(hpa *autoscalingv2.HorizontalPodAutoscaler) (emList []custommetrics.ExternalMetricValue) {
	for _, metricSpec := range hpa.Spec.Metrics {
		switch metricSpec.Type {
		case autoscalingv2.ExternalMetricSourceType:
			emList = append(emList, custommetrics.ExternalMetricValue{
				MetricName: metricSpec.External.MetricName,
				HPA: custommetrics.ObjectReference{
					Name:      hpa.Name,
					Namespace: hpa.Namespace,
					UID:       string(hpa.UID),
				},
				Labels: metricSpec.External.MetricSelector.MatchLabels,
			})
		default:
			log.Debugf("Unsupported metric type %s", metricSpec.Type)
		}
	}
	return
}

// DiffExternalMetrics returns the list of external metrics that reference hpas that are not in the given list of hpas.
func DiffExternalMetrics(informerList []*autoscalingv2.HorizontalPodAutoscaler, storedMetricsList []custommetrics.ExternalMetricValue) (toDelete []custommetrics.ExternalMetricValue) {
	hpaMetrics := map[string][]custommetrics.ExternalMetricValue{}

	for _, hpa := range informerList {
		hpaMetrics[string(hpa.UID)] = Inspect(hpa)
	}

	for _, em := range storedMetricsList {
		var found bool
		emList := hpaMetrics[em.HPA.UID]
		if emList == nil {
			toDelete = append(toDelete, em)
			continue
		}
		for _, m := range emList {
			// We have previously processed an external metric from this HPA.
			// Check that it's still the same. If not, remove the entry from the Global Store.
			if em.MetricName == m.MetricName && reflect.DeepEqual(em.Labels, m.Labels) {
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
