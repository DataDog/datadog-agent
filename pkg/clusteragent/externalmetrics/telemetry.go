// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

const (
	ddmTelemetryValid   = "true"
	ddmTelemetryInvalid = "false"
)

var (
	ddmTelemetryValues = []string{ddmTelemetryValid, ddmTelemetryInvalid}

	ddmTelemetry = telemetry.NewGaugeWithOpts("external_metrics", "datadog_metrics",
		[]string{"namespace", "name", "valid", "active", le.JoinLeaderLabel}, "The label valid is true if the DatadogMetric CR is valid, false otherwise. The label active is true if DatadogMetrics CR is used, false otherwise.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	requestsTelemetry = telemetry.NewGaugeWithOpts("external_metrics", "api_requests",
		[]string{"namespace", "handler", "in_error"}, "Count of API Requests received",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	elapsedTelemetry = telemetry.NewHistogramWithOpts("external_metrics", "api_elapsed",
		[]string{"namespace", "handler", "in_error"}, "Wall time spent on API request (seconds)",
		prometheus.DefBuckets,
		telemetry.Options{NoDoubleUnderscoreSep: true})

	retrieverElapsed = telemetry.NewHistogramWithOpts("external_metrics", "retriever_elapsed",
		[]string{}, "Wall time spent to retrieve metrics (seconds)",
		[]float64{0.5, 1, 5, 10, 20, 30, 60, 120, 300},
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

func setDatadogMetricTelemetry(ddm *datadoghq.DatadogMetric) {
	unsetDatadogMetricTelemetry(ddm.Namespace, ddm.Name)

	ddmTelemetry.Set(1.0, ddm.Namespace, ddm.Name, getDatadogMetricValidValue(ddm), getDatadogMetricActiveValue(ddm), le.JoinLeaderValue)
}

func unsetDatadogMetricTelemetry(ns, name string) {
	for _, valValid := range ddmTelemetryValues {
		for _, valActive := range ddmTelemetryValues {
			ddmTelemetry.Delete(ns, name, valValid, valActive, le.JoinLeaderValue)
		}
	}
}

func getDatadogMetricValidValue(ddm *datadoghq.DatadogMetric) string {
	if isDatadogMetricConditionTrue(ddm, datadoghq.DatadogMetricConditionTypeValid) {
		return ddmTelemetryValid
	}
	return ddmTelemetryInvalid
}

func getDatadogMetricActiveValue(ddm *datadoghq.DatadogMetric) string {
	if isDatadogMetricConditionTrue(ddm, datadoghq.DatadogMetricConditionTypeActive) {
		return ddmTelemetryValid
	}
	return ddmTelemetryInvalid
}

func isDatadogMetricConditionTrue(ddm *datadoghq.DatadogMetric, conditionType datadoghq.DatadogMetricConditionType) bool {
	for _, condition := range ddm.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

func setQueryTelemtry(handler, namespace string, startTime time.Time, err error) {
	// Handle telemtry
	inErrror := "false"
	if err != nil {
		inErrror = "true"
	}

	requestsTelemetry.Inc(namespace, handler, inErrror)
	elapsedTelemetry.Observe(time.Since(startTime).Seconds(), namespace, handler, inErrror)
}
