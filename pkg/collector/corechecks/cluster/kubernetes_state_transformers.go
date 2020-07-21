// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// metricTransformerFunc is used to tweak or generate new metrics from a given KSM metric
// For name translation only please use metricNamesMapper instead
type metricTransformerFunc = func(aggregator.Sender, string, ksmstore.DDMetric, []string)

var (
	// metricTransformers contains KSM metric names and their corresponding transformer functions
	// These metrics require more than a name translation to generate Datadog metrics, as opposed to the metrics in metricNamesMapper
	// TODO: implement the metric transformers of these metrics and unit test them
	// For reference see METRIC_TRANSFORMERS in KSM check V1
	metricTransformers = map[string]metricTransformerFunc{
		"kube_pod_status_phase":                       func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_pod_container_status_waiting_reason":    func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_pod_container_status_terminated_reason": func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_cronjob_next_schedule_time":             func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_job_complete":                           func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_job_failed":                             func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_job_status_failed":                      func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_job_status_succeeded":                   func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_node_status_condition":                  func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_node_spec_unschedulable":                func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_resourcequota":                          resourcequotaTransformer,
		"kube_limitrange":                             func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_persistentvolume_status_phase":          func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_service_spec_type":                      func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
	}
)

// resourcequotaTransformer generates dedicated metrics per resource per type from the kube_resourcequota metric
func resourcequotaTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	resource, found := metric.Labels["resource"]
	if !found {
		log.Debugf("Couldn't find 'resource' label, ignoring metric '%s'", name)
		return
	}
	quotaType, found := metric.Labels["type"]
	if !found {
		log.Debugf("Couldn't find 'type' label, ignoring metric '%s'", name)
		return
	}
	if quotaType == "hard" {
		quotaType = "limit"
	}
	metricName := ksmMetricPrefix + fmt.Sprintf("resourcequota.%s.%s", resource, quotaType)
	s.Gauge(metricName, metric.Val, "", tags)
}
