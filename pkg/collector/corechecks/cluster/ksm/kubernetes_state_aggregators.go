// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type metricAggregator interface {
	accumulate(ksmstore.DDMetric, *labelJoiner)
	flush(sender.Sender, *KSMCheck, *labelJoiner)
}

const accumulateDelimiter = "|"

// accumulateKey is a key used to accumulate metrics in the aggregator.
// Go does not allow slices or map as valid map keys, so we make string instaed of slice of label keys and values.
// E.g., for labels: {key:"a",value:"1"}, {key:"b",value:"2"}, {key:"c",value:"3"}, the accumulate key will be "a|b|c" and "1|2|3".
type accumulateKey struct {
	keys   string
	values string
}

func makeAccumulateKey(labels []label) accumulateKey {
	keys := make([]string, len(labels))
	vals := make([]string, len(labels))
	// Sort labels to ensure the order is deterministic.
	slices.SortFunc(labels, func(a, b label) int {
		v := cmp.Compare(a.key, b.key)
		if v == 0 {
			return cmp.Compare(a.value, b.value)
		}
		return v
	})
	for i, l := range labels {
		keys[i] = l.key
		vals[i] = l.value
	}
	return accumulateKey{
		keys:   strings.Join(keys, accumulateDelimiter),
		values: strings.Join(vals, accumulateDelimiter),
	}
}

func (a accumulateKey) labels() map[string]string {
	keys := strings.Split(a.keys, accumulateDelimiter)
	values := strings.Split(a.values, accumulateDelimiter)
	if len(keys) != len(values) {
		log.Errorf("BUG in KSM metric aggregator: keys and values have different lengths")
		return nil
	}
	labels := make(map[string]string, len(keys))
	for i := range keys {
		labels[keys[i]] = values[i]
	}
	return labels

}

type counterAggregator struct {
	ddMetricName  string
	ksmMetricName string

	accumulator map[accumulateKey]float64
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
	allowedResources []string

	accumulators map[string]map[accumulateKey]float64
}

type cronJobState struct {
	id    int
	state servicecheck.ServiceCheckStatus
}

type lastCronJobAggregator struct {
	accumulator map[accumulateKey]cronJobState
}

type lastCronJobCompleteAggregator struct {
	aggregator *lastCronJobAggregator
}

type lastCronJobFailedAggregator struct {
	aggregator *lastCronJobAggregator
}

func newSumValuesAggregator(ddMetricName, ksmMetricName string) metricAggregator {
	return &sumValuesAggregator{
		counterAggregator{
			ddMetricName:  ddMetricName,
			ksmMetricName: ksmMetricName,
			accumulator:   make(map[accumulateKey]float64),
		},
	}
}

func newCountObjectsAggregator(ddMetricName, ksmMetricName string) metricAggregator {
	return &countObjectsAggregator{
		counterAggregator{
			ddMetricName:  ddMetricName,
			ksmMetricName: ksmMetricName,
			accumulator:   make(map[accumulateKey]float64),
		},
	}
}

func newResourceValuesAggregator(ddMetricPrefix, ddMetricSuffix, ksmMetricName string, allowedResources []string) metricAggregator {
	accumulators := make(map[string]map[accumulateKey]float64)
	for _, allowedResource := range allowedResources {
		accumulators[allowedResource] = make(map[accumulateKey]float64)
	}

	return &resourceAggregator{
		ddMetricPrefix:   ddMetricPrefix,
		ddMetricSuffix:   ddMetricSuffix,
		ksmMetricName:    ksmMetricName,
		allowedResources: allowedResources,
		accumulators:     accumulators,
	}
}

func newLastCronJobAggregator() *lastCronJobAggregator {
	return &lastCronJobAggregator{
		accumulator: make(map[accumulateKey]cronJobState),
	}
}

func (a *sumValuesAggregator) accumulate(metric ksmstore.DDMetric, lj *labelJoiner) {
	a.accumulator[makeAccumulateKey(lj.getLabelsToAdd(metric.Labels))] += metric.Val
}

func (a *countObjectsAggregator) accumulate(metric ksmstore.DDMetric, lj *labelJoiner) {
	a.accumulator[makeAccumulateKey(lj.getLabelsToAdd(metric.Labels))]++
}

func (a *resourceAggregator) accumulate(metric ksmstore.DDMetric, lj *labelJoiner) {
	resource := renameResource(metric.Labels["resource"])

	if _, ok := a.accumulators[resource]; !ok {
		return
	}

	ls := lj.getLabelsToAdd(metric.Labels)
	if _, ok := a.accumulators[resource]; ok {
		a.accumulators[resource][makeAccumulateKey(ls)] += metric.Val
	}
}

func (a *lastCronJobCompleteAggregator) accumulate(metric ksmstore.DDMetric, lj *labelJoiner) {
	a.aggregator.accumulate(metric, servicecheck.ServiceCheckOK, lj)
}

func (a *lastCronJobFailedAggregator) accumulate(metric ksmstore.DDMetric, lj *labelJoiner) {
	a.aggregator.accumulate(metric, servicecheck.ServiceCheckCritical, lj)
}

func (a *lastCronJobAggregator) accumulate(metric ksmstore.DDMetric, state servicecheck.ServiceCheckStatus, lj *labelJoiner) {
	if condition, found := metric.Labels["condition"]; !found || condition != "true" {
		return
	}
	if metric.Val != 1 {
		return
	}

	_, found := metric.Labels["namespace"]
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

	key := makeAccumulateKey(lj.getLabelsToAdd(metric.Labels))
	if lastCronJob, found := a.accumulator[key]; !found || lastCronJob.id < id {
		a.accumulator[key] = cronJobState{
			id:    id,
			state: state,
		}
	}
}

func (a *counterAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	for accumulatorKey, count := range a.accumulator {
		hostname, tags := k.hostnameAndTags(accumulatorKey.labels(), labelJoiner, labelsMapperOverride(a.ksmMetricName))
		sender.Gauge(ksmMetricPrefix+a.ddMetricName, count, hostname, tags)
	}
	a.accumulator = make(map[accumulateKey]float64)
}

func (a *resourceAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	for _, resource := range a.allowedResources {
		metricName := fmt.Sprintf("%s%s.%s_%s", ksmMetricPrefix, a.ddMetricPrefix, resource, a.ddMetricSuffix)
		for key, count := range a.accumulators[resource] {
			hostname, tags := k.hostnameAndTags(key.labels(), labelJoiner, labelsMapperOverride(a.ksmMetricName))
			sender.Gauge(metricName, count, hostname, tags)
		}
		a.accumulators[resource] = make(map[accumulateKey]float64)
	}
}

func (a *lastCronJobCompleteAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	a.aggregator.flush(sender, k, labelJoiner)
}

func (a *lastCronJobFailedAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	a.aggregator.flush(sender, k, labelJoiner)
}

func (a *lastCronJobAggregator) flush(sender sender.Sender, k *KSMCheck, labelJoiner *labelJoiner) {
	for accumulatorKey, state := range a.accumulator {
		hostname, tags := k.hostnameAndTags(accumulatorKey.labels(), labelJoiner, map[string]string{"job_name": "cronjob"})
		sender.ServiceCheck(ksmMetricPrefix+"cronjob.complete", state.state, hostname, tags, "")
	}

	a.accumulator = make(map[accumulateKey]cronJobState)
}

func defaultMetricAggregators() map[string]metricAggregator {
	cronJobAggregator := newLastCronJobAggregator()

	return map[string]metricAggregator{
		"kube_configmap_info": newCountObjectsAggregator(
			"configmap.count",
			"kube_configmap_info",
		),
		"kube_secret_info": newCountObjectsAggregator(
			"secret.count",
			"kube_secret_info",
		),
		"kube_apiservice_labels": newCountObjectsAggregator(
			"apiservice.count",
			"kube_apiservice_labels",
		),
		"kube_customresourcedefinition_labels": newCountObjectsAggregator(
			"crd.count",
			"kube_customresourcedefinition_labels",
		),
		"kube_persistentvolume_status_phase": newSumValuesAggregator(
			"persistentvolumes.by_phase",
			"kube_persistentvolume_status_phase",
		),
		"kube_service_spec_type": newCountObjectsAggregator(
			"service.count",
			"kube_service_spec_type",
		),
		"kube_namespace_status_phase": newSumValuesAggregator(
			"namespace.count",
			"kube_namespace_status_phase",
		),
		"kube_replicaset_owner": newCountObjectsAggregator(
			"replicaset.count",
			"kube_replicaset_owner",
		),
		"kube_job_owner": newCountObjectsAggregator(
			"job.count",
			"kube_job_owner",
		),
		"kube_deployment_labels": newCountObjectsAggregator(
			"deployment.count",
			"kube_deployment_labels",
		),
		"kube_daemonset_labels": newCountObjectsAggregator(
			"daemonset.count",
			"kube_daemonset_labels",
		),
		"kube_statefulset_labels": newCountObjectsAggregator(
			"statefulset.count",
			"kube_statefulset_labels",
		),
		"kube_cronjob_labels": newCountObjectsAggregator(
			"cronjob.count",
			"kube_cronjob_labels",
		),
		"kube_endpoint_labels": newCountObjectsAggregator(
			"endpoint.count",
			"kube_endpoint_labels",
		),
		"kube_horizontalpodautoscaler_labels": newCountObjectsAggregator(
			"hpa.count",
			"kube_horizontalpodautoscaler_labels",
		),
		"kube_verticalpodautoscaler_labels": newCountObjectsAggregator(
			"vpa.count",
			"kube_verticalpodautoscaler_labels",
		),
		"kube_node_info": newCountObjectsAggregator(
			"node.count",
			"kube_node_info",
		),
		"kube_pod_info": newCountObjectsAggregator(
			"pod.count",
			"kube_pod_info",
		),
		"kube_ingress_labels": newCountObjectsAggregator(
			"ingress.count",
			"kube_ingress_labels",
		),
		"kube_job_complete": &lastCronJobCompleteAggregator{aggregator: cronJobAggregator},
		"kube_job_failed":   &lastCronJobFailedAggregator{aggregator: cronJobAggregator},
		"kube_node_status_allocatable": newResourceValuesAggregator(
			"node",
			"allocatable.total",
			"kube_node_status_allocatable",
			[]string{"cpu", "memory", "gpu", "mig"},
		),
		"kube_node_status_capacity": newResourceValuesAggregator(
			"node",
			"capacity.total",
			"kube_node_status_capacity",
			[]string{"cpu", "memory", "gpu", "mig"},
		),
		"kube_pod_container_resource_with_owner_tag_requests": newResourceValuesAggregator(
			"container",
			"requested.total",
			"kube_pod_container_resource_with_owner_tag_requests",
			[]string{"cpu", "memory"},
		),
		"kube_pod_container_resource_with_owner_tag_limits": newResourceValuesAggregator(
			"container",
			"limit.total",
			"kube_pod_container_resource_with_owner_tag_limits",
			[]string{"cpu", "memory"},
		),
	}
}
