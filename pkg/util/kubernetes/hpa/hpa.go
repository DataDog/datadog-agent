// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type DatadogClient interface {
	QueryMetrics(from, to int64, query string) ([]datadog.Series, error)
}

// HPAProcessor embeds the API Server client and the configuration to refresh metrics from Datadog and watch the HPA Objects' activities
type HPAProcessor struct {
	//readTimeout    time.Duration
	//refreshItl     *time.Ticker
	//pollItl        *time.Ticker
	//gcTicker       *time.Ticker // how often to gc metrics in the store
	externalMaxAge time.Duration
	datadogClient  DatadogClient
}

// NewHPAWatcherClient returns a new HPAProcessor
func NewHPAWatcherClient(datadogCl DatadogClient) (*HPAProcessor, error) {
//	pollInterval := config.Datadog.GetInt("hpa_watcher_polling_freq")
	//refreshInterval := config.Datadog.GetInt("external_metrics_provider.polling_freq")
	externalMaxAge := config.Datadog.GetInt("external_metrics_provider.max_age")
	//gcPeriodSeconds := config.Datadog.GetInt("hpa_watcher_gc_period")
	return &HPAProcessor{
		externalMaxAge: time.Duration(externalMaxAge) * time.Second,
		datadogClient:  datadogCl,
	}, nil
}

// ComputeDeleteExternalMetrics compares a list of ExternalMetrics with the HPA Objects.
func (c *HPAProcessor) ComputeDeleteExternalMetrics(list *autoscalingv2.HorizontalPodAutoscalerList, emList []custommetrics.ExternalMetricValue) (toDelete []custommetrics.ExternalMetricValue) {
	uids := make(map[string]struct{})
	for _, hpa := range list.Items {
		uids[string(hpa.UID)] = struct{}{}
	}
	var deleted []custommetrics.ExternalMetricValue
	for _, em := range emList {
		if _, ok := uids[em.HPA.UID]; !ok {
			deleted = append(deleted, em)
		}
	}
	return deleted
}


// UpdateExternalMetrics does the validation and processing of the ExternalMetrics
func (c *HPAProcessor) UpdateExternalMetrics(emList []custommetrics.ExternalMetricValue) (updated []custommetrics.ExternalMetricValue){
	maxAge := int64(c.externalMaxAge.Seconds())
	var err error

	for _, em := range emList {
		if metav1.Now().Unix()-em.Timestamp <= maxAge && em.Valid {
			continue
		}
		em.Valid = false
		em.Timestamp = metav1.Now().Unix()
		em.Value, em.Valid, err = c.validateExternalMetric(em.MetricName, em.Labels)
		if err != nil {
			log.Debugf("Could not fetch the external metric %s from Datadog, metric is no longer valid: %s", em.MetricName, err)
		}
		log.Debugf("Updated the external metric %#v", em)
		updated = append(updated, em)
	}
	return updated
}

// ProcessHPAs processes the HorizontalPodAutoscalers into a list of ExternalMetricValues.
func (c *HPAProcessor) ProcessHPAs(added *autoscalingv2.HorizontalPodAutoscaler) []custommetrics.ExternalMetricValue {
	var externalMetrics []custommetrics.ExternalMetricValue
	var err error
	log.Infof("attempting to process %v", added)

	for _, metricSpec := range added.Spec.Metrics {
		switch metricSpec.Type {
		case autoscalingv2.ExternalMetricSourceType:
			m := custommetrics.ExternalMetricValue{
				MetricName: metricSpec.External.MetricName,
				Timestamp:  metav1.Now().Unix(),
				HPA: custommetrics.ObjectReference{
					Name:      added.Name,
					Namespace: added.Namespace,
					UID:       string(added.UID),
				},
				Labels: metricSpec.External.MetricSelector.MatchLabels,
			}
			m.Value, m.Valid, err = c.validateExternalMetric(m.MetricName, m.Labels)
			if err != nil {
				log.Debugf("Could not fetch the external metric %s from Datadog, metric is no longer valid: %s", m.MetricName, err)
			}
			externalMetrics = append(externalMetrics, m)
		default:
			log.Debugf("Unsupported metric type %s", metricSpec.Type)
		}
	}
	return externalMetrics
}

// validateExternalMetric queries Datadog to validate the availability of an external metric
func (c *HPAProcessor) validateExternalMetric(metricName string, labels map[string]string) (value int64, valid bool, err error) {
	val, err := c.queryDatadogExternal(metricName, labels)
	log.Infof("got val %v and err %v", val, err)
	if err != nil {
		return val, false, err
	}
	return val, true, nil
}
