// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type metricAggregator interface {
	accumulate(ksmstore.DDMetric)
	flush(aggregator.Sender, *KSMCheck, *labelJoiner)
}

// maxNumberOfAllowedLabels contains the maximum number of labels that can be used to aggregate metrics.
// The only reason why there is a maximum is because the `accumulator` map is indexed on the label values
// and GO accepts arrays as valid map key type, but not slices.
// This hard coded limit is fine because the metrics to aggregate and the label list to use are hardcoded
// in the code and cannot be arbitrarily set by the end-user.
const maxNumberOfAllowedLabels = 3

type counterAggregator struct {
	metricName    string
	allowedLabels []string

	accumulator map[[maxNumberOfAllowedLabels]string]float64
}

func newCounterAggregator(metricName string, allowedLabels []string) metricAggregator {
	if len(allowedLabels) > maxNumberOfAllowedLabels {
		log.Error("BUG in KSM metric aggregator")
		return nil
	}

	return &counterAggregator{
		metricName:    metricName,
		allowedLabels: allowedLabels,
		accumulator:   make(map[[maxNumberOfAllowedLabels]string]float64),
	}
}

func (a *counterAggregator) accumulate(metric ksmstore.DDMetric) {
	var labelValues [3]string

	for i, allowedLabel := range a.allowedLabels {
		if allowedLabel == "" {
			break
		}

		labelValues[i] = metric.Labels[allowedLabel]
	}

	a.accumulator[labelValues] += metric.Val
}

func (a *counterAggregator) flush(sender aggregator.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	for labelValues, count := range a.accumulator {

		labels := make(map[string]string)
		for i, allowedLabel := range a.allowedLabels {
			if allowedLabel == "" {
				break
			}

			labels[allowedLabel] = labelValues[i]
		}

		hostname, tags := k.hostnameAndTags(labels, labelJoiner)

		sender.Gauge(ksmMetricPrefix+a.metricName, count, hostname, tags)
	}

	a.accumulator = make(map[[maxNumberOfAllowedLabels]string]float64)
}

var (
	metricAggregators = map[string]metricAggregator{
		"kube_persistentvolume_status_phase": newCounterAggregator(
			"persistentvolumes.by_phase",
			[]string{"storageclass", "phase"},
		),
		"kube_service_spec_type": newCounterAggregator(
			"service.count",
			[]string{"namespace", "type"},
		),
		"kube_namespace_status_phase": newCounterAggregator(
			"namespace.count",
			[]string{"phase"},
		),
		"kube_replicaset_owner": newCounterAggregator(
			"replicaset.count",
			[]string{"namespace", "owner_name", "owner_kind"},
		),
		"kube_job_owner": newCounterAggregator(
			"job.count",
			[]string{"namespace", "owner_name", "owner_kind"},
		),
		"kube_deployment_status_observed_generation": newCounterAggregator(
			"deployment.count",
			[]string{"namespace"},
		),
	}
)
