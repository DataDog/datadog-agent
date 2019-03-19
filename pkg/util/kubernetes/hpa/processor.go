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
func (p *Processor) UpdateExternalMetrics(emList []custommetrics.ExternalMetricValue) (updated []custommetrics.ExternalMetricValue) {
	maxAge := int64(p.externalMaxAge.Seconds())
	var err error

	metrics, err := p.validateExternalMetric(emList)
	if len(metrics) == 0 && err != nil {
		log.Errorf("Error getting metrics from Datadog: %v", err.Error())
		// If no metrics can be retrieved from Datadog in a given list, we need to invalidate them
		// To avoid undesirable autoscaling behaviors
		return invalidate(emList)
	}

	for _, em := range emList {
		// use query (metricName{scope}) as a key to avoid conflict if multiple hpas are using the same metric with different scopes.
		query := buildQuery(em)
		metric := metrics[query]

		if time.Now().Unix()-metric.timestamp > maxAge || !metric.valid {
			// invalidating sparse metrics that are outdated
			em.Valid = false
			em.Value = metric.value
			em.Timestamp = time.Now().Unix()
			updated = append(updated, em)
			continue
		}

		em.Valid = true
		em.Value = metric.value
		em.Timestamp = metric.timestamp
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
	metrics, err := p.validateExternalMetric(emList)
	if err != nil && len(metrics) == 0 {
		log.Errorf("Could not validate external metrics: %v", err)
		return invalidate(emList)
	}
	for _, em := range emList {
		maxAge := int64(p.externalMaxAge.Seconds())
		query := buildQuery(em)
		metric := metrics[query]
		em.Value = metric.value
		em.Timestamp = metric.timestamp
		em.Valid = metric.valid
		if metav1.Now().Unix()-metric.timestamp > maxAge {
			// If the maxAge is lower than the freshness of the metric, the metric is invalidated in the global store
			em.Valid = false
			em.Timestamp = metav1.Now().Unix() // The Timestamp is not the one of the metric, because we only rely on it to refresh.
		}
		log.Debugf("Added external metrics %#v", em)
		externalMetrics = append(externalMetrics, em)
	}
	return externalMetrics
}

// validateExternalMetric queries Datadog to validate the availability and value of one or more external metrics
func (p *Processor) validateExternalMetric(emList []custommetrics.ExternalMetricValue) (processed map[string]Point, err error) {
	var batch []string
	for _, e := range emList {
		q := buildQuery(e)
		batch = append(batch, q)
	}
	return p.queryDatadogExternal(batch)
}

func buildQuery(metric custommetrics.ExternalMetricValue) string {
	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	rollup := config.Datadog.GetInt("external_metrics_provider.rollup")

	if metric.CustomAggregator != "" {
		aggregator = metric.CustomAggregator
	}

	metricQuery := buildMetricQuery(metric)
	return fmt.Sprintf("%s:%s.rollup(%d)", aggregator, metricQuery, rollup)
}

func buildMetricQuery(metric custommetrics.ExternalMetricValue) string {
	if len(metric.Labels) == 0 {
		return fmt.Sprintf("%s{*}", metric.MetricName)
	}

	var datadogTags []string

	for tagName, tagValue := range metric.Labels {
		if customTagValue, found := metric.CustomTags[tagValue]; found {
			datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", tagName, customTagValue))
		} else {
			datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", tagName, tagValue))
		}
	}

	sort.Strings(datadogTags)
	tags := strings.Join(datadogTags, ",")

	return fmt.Sprintf("%s{%s}", metric.MetricName, tags)
}

func invalidate(emList []custommetrics.ExternalMetricValue) (invList []custommetrics.ExternalMetricValue) {
	for _, e := range emList {
		e.Valid = false
		e.Timestamp = metav1.Now().Unix()
		invList = append(invList, e)
	}
	return invList
}
