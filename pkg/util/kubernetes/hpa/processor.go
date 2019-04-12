// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	datadog "gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	externalMaxAge := math.Max(config.Datadog.GetFloat64("external_metrics_provider.max_age"), 3*config.Datadog.GetFloat64("external_metrics_provider.rollup"))
	return &Processor{
		externalMaxAge: time.Duration(externalMaxAge) * time.Second,
		datadogClient:  datadogCl,
	}, nil
}

// UpdateExternalMetrics does the validation and processing of the ExternalMetrics
// TODO if a metric's ts in emList is too recent, no need to add it to the batchUpdate.
func (p *Processor) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) (updated map[string]custommetrics.ExternalMetricValue) {
	maxAge := int64(p.externalMaxAge.Seconds())
	var err error
	updated = make(map[string]custommetrics.ExternalMetricValue)
	metrics, err := p.validateExternalMetric(emList)
	if len(metrics) == 0 && err != nil {
		log.Errorf("Error getting metrics from Datadog: %v", err.Error())
		// If no metrics can be retrieved from Datadog in a given list, we need to invalidate them
		// To avoid undesirable autoscaling behaviors
		return invalidate(emList)
	}

	for id, em := range emList {
		// use query (metricName{scope}) as a key to avoid conflict if multiple hpas are using the same metric with different scopes.
		metricIdentifier := getKey(em.MetricName, em.Labels)
		metric := metrics[metricIdentifier]

		if time.Now().Unix()-metric.timestamp > maxAge || !metric.valid {
			// invalidating sparse metrics that are outdated
			em.Valid = false
			em.Value = metric.value
			em.Timestamp = time.Now().Unix()
			updated[id] = em
			continue
		}

		em.Valid = true
		em.Value = metric.value
		em.Timestamp = metric.timestamp
		log.Debugf("Updated the external metric %#v", em)
		updated[id] = em
	}
	return updated
}

// ProcessHPAs processes the HorizontalPodAutoscalers into a list of ExternalMetricValues.
func (p *Processor) ProcessHPAs(hpa *autoscalingv2.HorizontalPodAutoscaler) map[string]custommetrics.ExternalMetricValue {
	externalMetrics := make(map[string]custommetrics.ExternalMetricValue)
	emList := Inspect(hpa)
	for _, em := range emList {
		em.Value = 0
		em.Timestamp = time.Now().Unix()
		em.Valid = false
		log.Tracef("Created a boilerplate for the external metrics %#v", em)
		id := custommetrics.ExternalMetricValueKeyFunc(em)
		externalMetrics[id] = em
	}
	return externalMetrics
}

// validateExternalMetric queries Datadog to validate the availability and value of one or more external metrics
func (p *Processor) validateExternalMetric(emList map[string]custommetrics.ExternalMetricValue) (processed map[string]Point, err error) {
	var batch []string
	for _, e := range emList {
		q := getKey(e.MetricName, e.Labels)
		batch = append(batch, q)
	}
	return p.queryDatadogExternal(batch)
}

func invalidate(emList map[string]custommetrics.ExternalMetricValue) (invList map[string]custommetrics.ExternalMetricValue) {
	for id, e := range emList {
		e.Valid = false
		e.Timestamp = metav1.Now().Unix()
		invList[id] = e
	}
	return invList
}

func getKey(name string, labels map[string]string) string {
	// Support queries with no tags
	if len(labels) == 0 {
		return fmt.Sprintf("%s{*}", name)
	}

	datadogTags := []string{}
	for key, val := range labels {
		datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", key, val))
	}
	sort.Strings(datadogTags)
	tags := strings.Join(datadogTags, ",")

	return fmt.Sprintf("%s{%s}", name, tags)
}
