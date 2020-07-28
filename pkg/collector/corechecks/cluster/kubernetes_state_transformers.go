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
	// TODO: implement the metric transformers of these metrics and unit test them
	// For reference see METRIC_TRANSFORMERS in KSM check V1
	metricTransformers = map[string]metricTransformerFunc{
		"kube_pod_status_phase":                       func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_pod_container_status_waiting_reason":    func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_pod_container_status_terminated_reason": func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_cronjob_next_schedule_time":             cronJobNextScheduleTransformer,
		"kube_job_complete":                           jobCompleteTransformer,
		"kube_job_failed":                             jobFailedTransformer,
		"kube_job_status_failed":                      jobStatusFailedTransformer,
		"kube_job_status_succeeded":                   jobStatusSucceededTransformer,
		"kube_node_status_condition":                  func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_node_spec_unschedulable":                func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_resourcequota":                          resourcequotaTransformer,
		"kube_limitrange":                             func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_persistentvolume_status_phase":          func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
		"kube_service_spec_type":                      func(s aggregator.Sender, name string, metric ksmstore.DDMetric, tags []string) {},
	}
)

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
