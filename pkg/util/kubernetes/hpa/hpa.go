// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/util/sets"

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
func DiffExternalMetrics(lhs []*autoscalingv2.HorizontalPodAutoscaler, rhs []custommetrics.ExternalMetricValue) (toDelete []custommetrics.ExternalMetricValue) {
	uids := sets.NewString()
	for _, hpa := range lhs {
		uids.Insert(string(hpa.UID))
	}
	for _, em := range rhs {
		if !uids.Has(em.HPA.UID) {
			toDelete = append(toDelete, em)
		}
	}
	return
}
