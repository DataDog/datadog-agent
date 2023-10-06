// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type metricAggregator interface {
	accumulate(ksmstore.DDMetric)
	flush(sender.Sender, *KSMCheck, *labelJoiner)
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

// resourceAggregator represents an aggregation on a resource metric, where the type of resource is part of the metric
// in the form of a label, instead of in the name.
type resourceAggregator struct {
	ddMetricPrefix   string
	ddMetricSuffix   string
	ksmMetricName    string
	allowedLabels    []string
	allowedResources []string

	accumulators map[string]map[[maxNumberOfAllowedLabels]string]float64
}

type cronJob struct {
	namespace string
	name      string
}

type cronJobState struct {
	id    int
	state servicecheck.ServiceCheckStatus
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

func newResourceValuesAggregator(ddMetricPrefix, ddMetricSuffix, ksmMetricName string, allowedLabels, allowedResources []string) metricAggregator {
	if len(allowedLabels) > maxNumberOfAllowedLabels {
		// `maxNumberOfAllowedLabels` is hardcoded to the maximum number of labels passed to this function from the metricsAggregators definition below.
		// The only possibility to arrive here is to add a new aggregator in `metricAggregator` below and to forget to update `maxNumberOfAllowedLabels` accordingly.
		log.Error("BUG in KSM metric aggregator")
		return nil
	}

	accumulators := make(map[string]map[[maxNumberOfAllowedLabels]string]float64)
	for _, allowedResource := range allowedResources {
		accumulators[allowedResource] = make(map[[maxNumberOfAllowedLabels]string]float64)
	}

	return &resourceAggregator{
		ddMetricPrefix:   ddMetricPrefix,
		ddMetricSuffix:   ddMetricSuffix,
		ksmMetricName:    ksmMetricName,
		allowedLabels:    allowedLabels,
		allowedResources: allowedResources,
		accumulators:     accumulators,
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

func (a *resourceAggregator) accumulate(metric ksmstore.DDMetric) {
	resource := renameResource(metric.Labels["resource"])

	if _, ok := a.accumulators[resource]; !ok {
		return
	}

	var labelValues [maxNumberOfAllowedLabels]string

	for i, allowedLabel := range a.allowedLabels {
		if allowedLabel == "" {
			break
		}

		labelValues[i] = metric.Labels[allowedLabel]
	}

	if _, ok := a.accumulators[resource]; ok {
		a.accumulators[resource][labelValues] += metric.Val
	}
}

func (a *lastCronJobCompleteAggregator) accumulate(metric ksmstore.DDMetric) {
	a.aggregator.accumulate(metric, servicecheck.ServiceCheckOK)
}

func (a *lastCronJobFailedAggregator) accumulate(metric ksmstore.DDMetric) {
	a.aggregator.accumulate(metric, servicecheck.ServiceCheckCritical)
}

func (a *lastCronJobAggregator) accumulate(metric ksmstore.DDMetric, state servicecheck.ServiceCheckStatus) {
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

func (a *counterAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
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

func (a *resourceAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	for _, resource := range a.allowedResources {
		metricName := fmt.Sprintf("%s%s.%s_%s", ksmMetricPrefix, a.ddMetricPrefix, resource, a.ddMetricSuffix)
		for labelValues, count := range a.accumulators[resource] {
			labels := make(map[string]string)
			for i, allowedLabel := range a.allowedLabels {
				if allowedLabel == "" {
					break
				}

				labels[allowedLabel] = labelValues[i]
			}

			hostname, tags := k.hostnameAndTags(labels, labelJoiner, labelsMapperOverride(a.ksmMetricName))
			sender.Gauge(metricName, count, hostname, tags)
		}
		a.accumulators[resource] = make(map[[maxNumberOfAllowedLabels]string]float64)
	}
}

func (a *lastCronJobCompleteAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	a.aggregator.flush(sender, k, labelJoiner)
}

func (a *lastCronJobFailedAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	a.aggregator.flush(sender, k, labelJoiner)
}

func (a *lastCronJobAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
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
		"kube_configmap_info": newCountObjectsAggregator(
			"configmap.count",
			"kube_configmap_info",
			[]string{"namespace"},
		),
		"kube_secret_info": newCountObjectsAggregator(
			"secret.count",
			"kube_secret_info",
			[]string{"namespace"},
		),
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
		"kube_node_status_allocatable": newResourceValuesAggregator(
			"node",
			"allocatable.total",
			"kube_node_status_allocatable",
			[]string{},
			[]string{"cpu", "memory", "gpu", "mig"},
		),
		"kube_node_status_capacity": newResourceValuesAggregator(
			"node",
			"capacity.total",
			"kube_node_status_capacity",
			[]string{},
			[]string{"cpu", "memory", "gpu", "mig"},
		),
		"kube_pod_container_resource_with_owner_tag_requests": newResourceValuesAggregator(
			"container",
			"requested.total",
			"kube_pod_container_resource_with_owner_tag_requests",
			[]string{"namespace", "container", "owner_name", "owner_kind"},
			[]string{"cpu", "memory"},
		),
		"kube_pod_container_resource_with_owner_tag_limits": newResourceValuesAggregator(
			"container",
			"limit.total",
			"kube_pod_container_resource_with_owner_tag_limits",
			[]string{"namespace", "container", "owner_name", "owner_kind"},
			[]string{"cpu", "memory", "gpu", "mig"},
		),
	}
}
