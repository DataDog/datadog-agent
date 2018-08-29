// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	datadog "gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DatadogClient interface {
	QueryMetrics(from, to int64, query string) ([]datadog.Series, error)
}

// Processor embeds the configuration to refresh metrics from Datadog and process HPA structs to ExternalMetrics.
type Processor struct {
	externalMaxAge time.Duration
	datadogClient  DatadogClient
}

// NewProcessor returns a new Processor
func NewProcessor(datadogCl DatadogClient) (*Processor, error) {
	externalMaxAge := config.Datadog.GetInt("external_metrics_provider.max_age")
	return &Processor{
		externalMaxAge: time.Duration(externalMaxAge) * time.Second,
		datadogClient:  datadogCl,
	}, nil
}

// UpdateExternalMetrics does the validation and processing of the ExternalMetrics
func (p *Processor) UpdateExternalMetrics(emList []custommetrics.ExternalMetricValue) (updated []custommetrics.ExternalMetricValue) {
	maxAge := int64(p.externalMaxAge.Seconds())
	var err error

	for _, em := range emList {
		if metav1.Now().Unix()-em.Timestamp <= maxAge && em.Valid {
			continue
		}
		em.Valid = false
		em.Timestamp = metav1.Now().Unix()
		em.Value, em.Valid, err = p.validateExternalMetric(em.MetricName, em.Labels)
		if err != nil {
			log.Debugf("Could not fetch the external metric %s from Datadog, metric is no longer valid: %s", em.MetricName, err)
		}
		log.Debugf("Updated the external metric %#v", em)
		updated = append(updated, em)
	}
	return updated
}

// ProcessHPAs processes the HorizontalPodAutoscalers into a list of ExternalMetricValues.
func (p *Processor) ProcessHPAs(hpa *autoscalingv2.HorizontalPodAutoscaler) []custommetrics.ExternalMetricValue {
	var externalMetrics []custommetrics.ExternalMetricValue
	var err error

	emList := Inspect(hpa)

	for _, em := range emList {
		em.Timestamp = metav1.Now().Unix()
		em.Value, em.Valid, err = p.validateExternalMetric(em.MetricName, em.Labels)
		if err != nil {
			log.Debugf("Could not fetch the external metric %s from Datadog, metric is no longer valid: %s", em.MetricName, err)
		}
		externalMetrics = append(externalMetrics, em)
	}
	return externalMetrics
}

// validateExternalMetric queries Datadog to validate the availability and value of an external metric
func (p *Processor) validateExternalMetric(metricName string, labels map[string]string) (value int64, valid bool, err error) {
	val, err := p.queryDatadogExternal(metricName, labels)
	if err != nil {
		return val, false, err
	}
	return val, true, nil
}
