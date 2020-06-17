// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	keyDelimeter                  = "-"
	storeLastUpdatedAnnotationKey = "custom-metrics.datadoghq.com/last-updated"
)

var (
	externalTotal = telemetry.NewGaugeWithOpts("", "external_metrics",
		[]string{"valid"}, "Number of external metrics tagged.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

// Store is an interface for persistent storage of custom and external metrics.
type Store interface {
	SetExternalMetricValues(map[string]ExternalMetricValue) error

	DeleteExternalMetricValues(*MetricsBundle) error

	ListAllExternalMetricValues() (*MetricsBundle, error)

	GetMetrics() (*MetricsBundle, error)
}

// ExternalMetricValueKeyFunc knows how to make keys for storing external metrics. The key
// is unique for each metric of an Autoscaler. This means that the keys for the same metric from two
// different HPAs will be different (important for external metrics that may use different labels
// for the same metric).
func ExternalMetricValueKeyFunc(val ExternalMetricValue) string {
	parts := []string{
		"external_metric",
		val.Ref.Type,
		val.Ref.Namespace,
		val.Ref.Name,
		val.MetricName,
	}
	return strings.Join(parts, keyDelimeter)
}

func DeprecatedExternalMetricValueKeyFunc(val DeprecatedExternalMetricValue) string {
	parts := []string{
		"external_metric",
		val.HPA.Namespace,
		val.HPA.Name,
		val.MetricName,
	}
	return strings.Join(parts, keyDelimeter)
}

func isExternalMetricValueKey(key string) bool {
	return strings.HasPrefix(key, "external_metric")
}
