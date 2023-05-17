// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
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
const maxNumberOfAllowedLabels = 4

type counterAggregator struct {
	ddMetricName  string
	ksmMetricName string
	allowedLabels []string

	accumulator map[[maxNumberOfAllowedLabels]string]float64
}

type sumValuesAggregator struct {
	counterAggregator
}

type countObjectsAggregator struct {
	counterAggregator
}

type cronJob struct {
	namespace string
	name      string
}

type cronJobState struct {
	id    int
	state metrics.ServiceCheckStatus
}

type lastCronJobAggregator struct {
	accumulator map[cronJob]cronJobState
}

type lastCronJobCompleteAggregator struct {
	aggregator *lastCronJobAggregator
}

type lastCronJobFailedAggregator struct {
	aggregator *lastCronJobAggregator
}

func newSumValuesAggregator(ddMetricName, ksmMetricName string, allowedLabels []string) metricAggregator {
	if len(allowedLabels) > maxNumberOfAllowedLabels {
		// `maxNumberOfAllowedLabels` is hardcoded to the maximum number of labels passed to this function from the metricsAggregators definition below.
		// The only possibility to arrive here is to add a new aggregator in `metricAggregator` below and to forget to update `maxNumberOfAllowedLabels` accordingly.
		log.Error("BUG in KSM metric aggregator")
		return nil
	}

	return &sumValuesAggregator{
		counterAggregator{
			ddMetricName:  ddMetricName,
			ksmMetricName: ksmMetricName,
			allowedLabels: allowedLabels,
			accumulator:   make(map[[maxNumberOfAllowedLabels]string]float64),
		},
	}
}

func newCountObjectsAggregator(ddMetricName, ksmMetricName string, allowedLabels []string) metricAggregator {
	if len(allowedLabels) > maxNumberOfAllowedLabels {
		// `maxNumberOfAllowedLabels` is hardcoded to the maximum number of labels passed to this function from the metricsAggregators definition below.
		// The only possibility to arrive here is to add a new aggregator in `metricAggregator` below and to forget to update `maxNumberOfAllowedLabels` accordingly.
		log.Error("BUG in KSM metric aggregator")
		return nil
	}

	return &countObjectsAggregator{
		counterAggregator{
			ddMetricName:  ddMetricName,
			ksmMetricName: ksmMetricName,
			allowedLabels: allowedLabels,
			accumulator:   make(map[[maxNumberOfAllowedLabels]string]float64),
		},
	}
}

func newLastCronJobAggregator() *lastCronJobAggregator {
	return &lastCronJobAggregator{
		accumulator: make(map[cronJob]cronJobState),
	}
}

func (a *sumValuesAggregator) accumulate(metric ksmstore.DDMetric) {
	var labelValues [maxNumberOfAllowedLabels]string

	for i, allowedLabel := range a.allowedLabels {
		if allowedLabel == "" {
			break
		}

		labelValues[i] = metric.Labels[allowedLabel]
	}

	a.accumulator[labelValues] += metric.Val
}

func (a *countObjectsAggregator) accumulate(metric ksmstore.DDMetric) {
	var labelValues [maxNumberOfAllowedLabels]string

	for i, allowedLabel := range a.allowedLabels {
		if allowedLabel == "" {
			break
		}

		labelValues[i] = metric.Labels[allowedLabel]
	}

	a.accumulator[labelValues]++
}

func (a *lastCronJobCompleteAggregator) accumulate(metric ksmstore.DDMetric) {
	a.aggregator.accumulate(metric, metrics.ServiceCheckOK)
}

func (a *lastCronJobFailedAggregator) accumulate(metric ksmstore.DDMetric) {
	a.aggregator.accumulate(metric, metrics.ServiceCheckCritical)
}

func (a *lastCronJobAggregator) accumulate(metric ksmstore.DDMetric, state metrics.ServiceCheckStatus) {
	if condition, found := metric.Labels["condition"]; !found || condition != "true" {
		return
	}
	if metric.Val != 1 {
		return
	}

	namespace, found := metric.Labels["namespace"]
	if !found {
		return
	}

	jobName, found := metric.Labels["job_name"]
	if !found {
		return
	}

	cronjobName, id := kubernetes.ParseCronJobForJob(jobName)
	if cronjobName == "" {
		log.Debugf("%q isn't a valid CronJob name", jobName)
		return
	}

	if lastCronJob, found := a.accumulator[cronJob{namespace: namespace, name: cronjobName}]; !found || lastCronJob.id < id {
		a.accumulator[cronJob{namespace: namespace, name: cronjobName}] = cronJobState{
			id:    id,
			state: state,
		}
	}
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

		hostname, tags := k.hostnameAndTags(labels, labelJoiner, labelsMapperOverride(a.ksmMetricName))

		sender.Gauge(ksmMetricPrefix+a.ddMetricName, count, hostname, tags)
	}

	a.accumulator = make(map[[maxNumberOfAllowedLabels]string]float64)
}

func (a *lastCronJobCompleteAggregator) flush(sender aggregator.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	a.aggregator.flush(sender, k, labelJoiner)
}

func (a *lastCronJobFailedAggregator) flush(sender aggregator.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	a.aggregator.flush(sender, k, labelJoiner)
}

func (a *lastCronJobAggregator) flush(sender aggregator.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	for cronjob, state := range a.accumulator {
		hostname, tags := k.hostnameAndTags(
			map[string]string{
				"namespace": cronjob.namespace,
				"cronjob":   cronjob.name,
			},
			labelJoiner,
			nil,
		)

		sender.ServiceCheck(ksmMetricPrefix+"cronjob.complete", state.state, hostname, tags, "")
	}

	a.accumulator = make(map[cronJob]cronJobState)
}

func defaultMetricAggregators() map[string]metricAggregator {
	cronJobAggregator := newLastCronJobAggregator()

	return map[string]metricAggregator{
		"kube_apiservice_labels": newCountObjectsAggregator(
			"apiservice.count",
			"kube_apiservice_labels",
			[]string{},
		),
		"kube_customresourcedefinition_labels": newCountObjectsAggregator(
			"crd.count",
			"kube_customresourcedefinition_labels",
			[]string{},
		),
		"kube_persistentvolume_status_phase": newSumValuesAggregator(
			"persistentvolumes.by_phase",
			"kube_persistentvolume_status_phase",
			[]string{"storageclass", "phase"},
		),
		"kube_service_spec_type": newCountObjectsAggregator(
			"service.count",
			"kube_service_spec_type",
			[]string{"namespace", "type"},
		),
		"kube_namespace_status_phase": newSumValuesAggregator(
			"namespace.count",
			"kube_namespace_status_phase",
			[]string{"phase"},
		),
		"kube_replicaset_owner": newCountObjectsAggregator(
			"replicaset.count",
			"kube_replicaset_owner",
			[]string{"namespace", "owner_name", "owner_kind"},
		),
		"kube_job_owner": newCountObjectsAggregator(
			"job.count",
			"kube_job_owner",
			[]string{"namespace", "owner_name", "owner_kind"},
		),
		"kube_deployment_labels": newCountObjectsAggregator(
			"deployment.count",
			"kube_deployment_labels",
			[]string{"namespace"},
		),
		"kube_daemonset_labels": newCountObjectsAggregator(
			"daemonset.count",
			"kube_daemonset_labels",
			[]string{"namespace"},
		),
		"kube_statefulset_labels": newCountObjectsAggregator(
			"statefulset.count",
			"kube_statefulset_labels",
			[]string{"namespace"},
		),
		"kube_cronjob_labels": newCountObjectsAggregator(
			"cronjob.count",
			"kube_cronjob_labels",
			[]string{"namespace"},
		),
		"kube_endpoint_labels": newCountObjectsAggregator(
			"endpoint.count",
			"kube_endpoint_labels",
			[]string{"namespace"},
		),
		"kube_horizontalpodautoscaler_labels": newCountObjectsAggregator(
			"hpa.count",
			"kube_horizontalpodautoscaler_labels",
			[]string{"namespace"},
		),
		"kube_verticalpodautoscaler_labels": newCountObjectsAggregator(
			"vpa.count",
			"kube_verticalpodautoscaler_labels",
			[]string{"namespace"},
		),
		"kube_node_info": newCountObjectsAggregator(
			"node.count",
			"kube_node_info",
			[]string{"kubelet_version", "container_runtime_version", "kernel_version", "os_image"},
		),
		"kube_pod_info": newCountObjectsAggregator(
			"pod.count",
			"kube_pod_info",
			[]string{"node", "namespace", "created_by_kind", "created_by_name"},
		),
		"kube_ingress_labels": newCountObjectsAggregator(
			"ingress.count",
			"kube_ingress_labels",
			[]string{"namespace"},
		),
		"kube_job_complete": &lastCronJobCompleteAggregator{aggregator: cronJobAggregator},
		"kube_job_failed":   &lastCronJobFailedAggregator{aggregator: cronJobAggregator},
	}
}
