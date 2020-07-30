// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// metricTransformerFunc is used to tweak or generate new metrics from a given KSM metric
// For name translation only please use metricNamesMapper instead
type metricTransformerFunc = func(aggregator.Sender, string, ksmstore.DDMetric, []string)

var (
	// metricTransformers contains KSM metric names and their corresponding transformer functions
	// These metrics require more than a name translation to generate Datadog metrics, as opposed to the metrics in metricNamesMapper
	// For reference see METRIC_TRANSFORMERS in KSM check V1
	metricTransformers = map[string]metricTransformerFunc{
		"kube_pod_status_phase":                       podPhaseTransformer,
		"kube_pod_container_status_waiting_reason":    containerWaitingReasonTransformer,
		"kube_pod_container_status_terminated_reason": containerTerminatedReasonTransformer,
		"kube_cronjob_next_schedule_time":             cronJobNextScheduleTransformer,
		"kube_job_complete":                           jobCompleteTransformer,
		"kube_job_failed":                             jobFailedTransformer,
		"kube_job_status_failed":                      jobStatusFailedTransformer,
		"kube_job_status_succeeded":                   jobStatusSucceededTransformer,
		"kube_node_status_condition":                  nodeConditionTransformer,
		"kube_node_spec_unschedulable":                nodeUnschedulableTransformer,
		"kube_resourcequota":                          resourcequotaTransformer,
		"kube_limitrange":                             limitrangeTransformer,
		"kube_persistentvolume_status_phase":          pvPhaseTransformer,
		"kube_service_spec_type":                      serviceTypeTransformer,
	}
)

// nodeConditionTransformer generates service checks based on the metric kube_node_status_condition
// It also submit the metric kubernetes_state.node.by_condition
func nodeConditionTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	if metric.Val != 1.0 {
		// Only consider active metrics
		return
	}

	condition, found := metric.Labels["condition"]
	if !found {
		log.Debugf("Couldn't find 'condition' label, ignoring service check for metric '%s'", name)
		return
	}

	s.Gauge(ksmMetricPrefix+"node.by_condition", metric.Val, "", tags)
	statusLabel, found := metric.Labels["status"]
	if !found {
		log.Debugf("Couldn't find 'status' label, ignoring service check for metric '%s'", name)
		return
	}

	serviceCheckName := ""
	var eventStatus metrics.ServiceCheckStatus
	switch condition {
	case "Ready":
		serviceCheckName = ksmMetricPrefix + "node.ready"
		eventStatus = statusForCondition(statusLabel, true)
	case "OutOfDisk":
		serviceCheckName = ksmMetricPrefix + "node.out_of_disk"
		eventStatus = statusForCondition(statusLabel, false)
	case "DiskPressure":
		serviceCheckName = ksmMetricPrefix + "node.disk_pressure"
		eventStatus = statusForCondition(statusLabel, false)
	case "NetworkUnavailable":
		serviceCheckName = ksmMetricPrefix + "node.network_unavailable"
		eventStatus = statusForCondition(statusLabel, false)
	case "MemoryPressure":
		serviceCheckName = ksmMetricPrefix + "node.memory_pressure"
		eventStatus = statusForCondition(statusLabel, false)
	default:
		log.Debugf("Invalid 'condition' label '%s', ignoring service check for metric '%s'", condition, name)
		return
	}

	node, found := metric.Labels["node"]
	if !found {
		log.Debugf("Couldn't find 'node' label, ignoring service check for metric '%s'", name)
		return
	}

	message := fmt.Sprintf("%s is currently reporting %s = %s", node, condition, statusLabel)
	s.ServiceCheck(serviceCheckName, eventStatus, "", tags, message)
}

// statusForCondition returns the right service check status based on the KSM label 'status'
// and the nature of the event whether a positive event or not
// e.g Ready is a positive event while MemoryPressure is not
func statusForCondition(status string, positiveEvent bool) metrics.ServiceCheckStatus {
	switch status {
	case "true":
		if positiveEvent {
			return metrics.ServiceCheckOK
		}
		return metrics.ServiceCheckCritical
	case "false":
		if positiveEvent {
			return metrics.ServiceCheckCritical
		}
		return metrics.ServiceCheckOK
	case "unknown":
		return metrics.ServiceCheckUnknown
	default:
		log.Debugf("Unknown 'status' label: '%s'", status)
		return metrics.ServiceCheckUnknown
	}
}

// nodeUnschedulableTransformer reports whether a node can schedule new pods
// It adds a tag 'status' that can be either 'schedulable' or 'unschedulable' and always report the metric value '1'
func nodeUnschedulableTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	status := ""
	switch metric.Val {
	case 0.0:
		status = "schedulable"
	case 1.0:
		status = "unschedulable"
	default:
		log.Debugf("Invalid metric value '%v', ignoring metric '%s'", metric.Val, name)
		return
	}
	tags = append(tags, "status:"+status)
	s.Gauge(ksmMetricPrefix+"node.status", 1, "", tags)
}

// podPhaseTransformer sends status phase metrics for pods, the tag phase has the pod status
func podPhaseTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	submitActiveMetric(s, ksmMetricPrefix+"pod.status_phase", metric, tags)
}

var allowedWaitingReasons = map[string]struct{}{
	"errimagepull":      {},
	"imagepullbackoff":  {},
	"crashloopbackoff":  {},
	"containercreating": {},
}

// containerWaitingReasonTransformer validates the container waiting reasons for metric kube_pod_container_status_waiting_reason
func containerWaitingReasonTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	if reason, found := metric.Labels["reason"]; found {
		// Filtering according to the reason here is paramount to limit cardinality
		if _, allowed := allowedWaitingReasons[strings.ToLower(reason)]; allowed {
			s.Gauge(ksmMetricPrefix+"container.status_report.count.waiting", metric.Val, "", tags)
		}
	}
}

var allowedTerminatedReasons = map[string]struct{}{
	"oomkilled":          {},
	"containercannotrun": {},
	"error":              {},
}

// containerTerminatedReasonTransformer validates the container waiting reasons for metric kube_pod_container_status_terminated_reason
func containerTerminatedReasonTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	if reason, found := metric.Labels["reason"]; found {
		// Filtering according to the reason here is paramount to limit cardinality
		if _, allowed := allowedTerminatedReasons[strings.ToLower(reason)]; allowed {
			s.Gauge(ksmMetricPrefix+"container.status_report.count.terminated", metric.Val, "", tags)
		}
	}
}

// now allows testing
var now = time.Now

// cronJobNextScheduleTransformer sends a service check to alert if the cronjob's next schedule is in the past
func cronJobNextScheduleTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	message := ""
	var status metrics.ServiceCheckStatus
	timeDiff := int64(metric.Val) - now().Unix()
	if timeDiff >= 0 {
		status = metrics.ServiceCheckOK
	} else {
		status = metrics.ServiceCheckCritical
		message = fmt.Sprintf("The cron job check scheduled at %s is %d seconds late", time.Unix(int64(metric.Val), 0).UTC(), -timeDiff)
	}
	s.ServiceCheck(ksmMetricPrefix+"cronjob.on_schedule_check", status, "", tags, message)
}

// jobCompleteTransformer sends a service check based on kube_job_complete
func jobCompleteTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	jobServiceCheck(s, metric, metrics.ServiceCheckOK, tags)
}

// jobFailedTransformer sends a service check based on kube_job_failed
func jobFailedTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	jobServiceCheck(s, metric, metrics.ServiceCheckCritical, tags)
}

// jobTimestampPattern extracts the timestamp in the job name label
// Used by trimJobTag to remove the timestamp from the job tag
// Expected job tag format: <job>-<timestamp>
var jobTimestampPattern = regexp.MustCompile(`(-\d{4,10}$)`)

// trimJobTag removes the timestamp from the job name tag
// Expected job tag format: <job>-<timestamp>
func trimJobTag(tag string) string {
	return jobTimestampPattern.ReplaceAllString(tag, "")
}

// validateJob detects active jobs and strips the timestamp from the job_name tag
func validateJob(val float64, tags []string) ([]string, bool) {
	if val != 1.0 {
		// Only consider active metrics
		return nil, false
	}

	for i, tag := range tags {
		split := strings.Split(tag, ":")
		if len(split) == 2 && split[0] == "job" || split[0] == "job_name" {
			// Trim the timestamp suffix to avoid high cardinality
			tags[i] = fmt.Sprintf("%s:%s", split[0], trimJobTag(split[1]))
			return tags, true
		}
	}

	return tags, true
}

// jobServiceCheck sends a service check for jobs
func jobServiceCheck(s aggregator.Sender, metric ksmstore.DDMetric, status metrics.ServiceCheckStatus, tags []string) {
	if strippedTags, valid := validateJob(metric.Val, tags); valid {
		s.ServiceCheck(ksmMetricPrefix+"job.complete", status, "", strippedTags, "")
	}
}

// jobStatusSucceededTransformer sends a metric based on kube_job_status_succeeded
func jobStatusSucceededTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	jobMetric(s, metric, ksmMetricPrefix+"job.succeeded", tags)
}

// jobStatusFailedTransformer sends a metric based on kube_job_status_failed
func jobStatusFailedTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	jobMetric(s, metric, ksmMetricPrefix+"job.failed", tags)
}

// jobMetric sends a gauge for job status
func jobMetric(s aggregator.Sender, metric ksmstore.DDMetric, metricName string, tags []string) {
	if strippedTags, valid := validateJob(metric.Val, tags); valid {
		// TODO: Many problems have been reported about job metrics in the v1 check
		// This is different compared to what we do in the v1 check already but let's investigate more
		s.Gauge(metricName, 1, "", strippedTags)
	}
}

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

// constraintsMapper is used by the kube_limitrange metric transformer
var constraintsMapper = map[string]string{
	"min":                  "min",
	"max":                  "max",
	"default":              "default",
	"defaultRequest":       "default_request",
	"maxLimitRequestRatio": "max_limit_request_ratio",
}

// limitrangeTransformer generates dedicated metrics per resource per type from the kube_limitrange metric
func limitrangeTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	constraintLabel, found := metric.Labels["constraint"]
	if !found {
		log.Debugf("Couldn't find 'constraint' label, ignoring metric '%s'", name)
		return
	}
	constraint, found := constraintsMapper[constraintLabel]
	if !found {
		log.Debugf("Constraint '%s' unsupported for metric '%s'", constraint, name)
		return
	}
	resource, found := metric.Labels["resource"]
	if !found {
		log.Debugf("Couldn't find 'resource' label, ignoring metric '%s'", name)
		return
	}
	metricName := ksmMetricPrefix + fmt.Sprintf("limitrange.%s.%s", resource, constraint)
	s.Gauge(metricName, metric.Val, "", tags)
}

// submitActiveMetrics only sends metrics with value '1'
func submitActiveMetric(s aggregator.Sender, metricName string, metric ksmstore.DDMetric, tags []string) {
	if metric.Val != 1.0 {
		// Only consider active metrics
		return
	}
	s.Gauge(metricName, 1, "", tags)
}

// pvPhaseTransformer generates metrics per persistentvolume and per phase from the kube_persistentvolume_status_phase metric
func pvPhaseTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	submitActiveMetric(s, ksmMetricPrefix+"persistentvolume.by_phase", metric, tags)
}

// serviceTypeTransformer generates metrics per service, namespace, and type from the kube_service_spec_type metric
func serviceTypeTransformer(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {
	submitActiveMetric(s, ksmMetricPrefix+"service.type", metric, tags)
}
