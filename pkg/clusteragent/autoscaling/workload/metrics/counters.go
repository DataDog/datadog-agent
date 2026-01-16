// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// SubmitReceivedRecommendationsVersion submits a gauge metric for received recommendations version
func SubmitReceivedRecommendationsVersion(s sender.Sender, version float64, namespace, targetName, autoscalerName string) {
	metricName := metricPrefix + ".received_recommendations_version"
	tags := baseAutoscalerTags(namespace, targetName, autoscalerName)
	s.Gauge(metricName, version, "", tags)
	s.Commit()
}

// SubmitHorizontalScaleAction submits a counter metric for a horizontal scaling action
func SubmitHorizontalScaleAction(s sender.Sender, namespace, targetName, autoscalerName, source, status string) {
	metricName := metricPrefix + ".horizontal_scaling_actions"
	tags := autoscalerTagsWithSource(namespace, targetName, autoscalerName, source)
	tags = append(tags, "status:"+status)
	s.Count(metricName, 1, "", tags)
	s.Commit()
}

// SubmitHorizontalScaleAppliedReplicas submits a gauge metric for applied horizontal scaling replicas
func SubmitHorizontalScaleAppliedReplicas(s sender.Sender, replicas float64, namespace, targetName, autoscalerName, source string) {
	metricName := metricPrefix + ".horizontal_scaling_applied_replicas"
	tags := autoscalerTagsWithSource(namespace, targetName, autoscalerName, source)
	s.Gauge(metricName, replicas, "", tags)
	s.Commit()
}

// SubmitVerticalRolloutTriggered submits a counter metric for a vertical rollout trigger
func SubmitVerticalRolloutTriggered(s sender.Sender, namespace, targetName, autoscalerName, status string) {
	metricName := metricPrefix + ".vertical_rollout_triggered"
	tags := baseAutoscalerTags(namespace, targetName, autoscalerName)
	tags = append(tags, "status:"+status)
	s.Count(metricName, 1, "", tags)
	s.Commit()
}
