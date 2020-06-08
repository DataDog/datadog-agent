// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build kubeapiserver

package autoscalers

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilserror "k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
)

const (
	// chunkSize ensures batch queries are limited in size.
	chunkSize = 35
	// maxCharactersPerChunk is the maximum size of a single chunk to avoid 414 Request-URI Too Large
	maxCharactersPerChunk = 7000
	// extraQueryCharacters accounts for the extra characters added to form a query to Datadog's API (e.g.: `avg:`, `.rollup(X)` ...)
	extraQueryCharacters = 16
)

type DatadogClient interface {
	QueryMetrics(from, to int64, query string) ([]datadog.Series, error)
	GetRateLimitStats() map[string]datadog.RateLimit
}

// ProcessorInterface is used to easily mock the interface for testing
type ProcessorInterface interface {
	UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue
	QueryExternalMetric(queries []string) (map[string]Point, error)
	ProcessEMList(emList []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue
}

// Processor embeds the configuration to refresh metrics from Datadog and process Ref structs to ExternalMetrics.
type Processor struct {
	externalMaxAge time.Duration
	datadogClient  DatadogClient
}

// queryResponse ensures that we capture all the signals from the call to Datadog's backend.
type queryResponse struct {
	metrics map[string]Point
	err     error
}

// NewProcessor returns a new Processor
func NewProcessor(datadogCl DatadogClient) *Processor {
	externalMaxAge := math.Max(config.Datadog.GetFloat64("external_metrics_provider.max_age"), 3*config.Datadog.GetFloat64("external_metrics_provider.rollup"))
	return &Processor{
		externalMaxAge: time.Duration(externalMaxAge) * time.Second,
		datadogClient:  datadogCl,
	}
}

// ProcessHPAs processes the HorizontalPodAutoscalers into a list of ExternalMetricValues.
func (p *Processor) ProcessEMList(emList []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	externalMetrics := make(map[string]custommetrics.ExternalMetricValue)
	for _, em := range emList {
		em.Value = 0
		em.Timestamp = time.Now().Unix()
		em.Valid = false
		log.Tracef("Created a boilerplate for the external metrics %s{%v} for %s %s/%s", em.MetricName, em.Labels, em.Ref.Type, em.Ref.Namespace, em.Ref.Name)
		id := custommetrics.ExternalMetricValueKeyFunc(em)
		externalMetrics[id] = em
	}
	return externalMetrics
}

// ProcessHPAs processes the HorizontalPodAutoscalers into a list of ExternalMetricValues.
func (p *Processor) ProcessHPAs(hpa *autoscalingv2.HorizontalPodAutoscaler) map[string]custommetrics.ExternalMetricValue {
	externalMetrics := make(map[string]custommetrics.ExternalMetricValue)
	emList := InspectHPA(hpa)
	for _, em := range emList {
		em.Value = 0
		em.Timestamp = time.Now().Unix()
		em.Valid = false
		log.Tracef("Created a boilerplate for the external metrics %s{%v} for %s %s/%s", em.MetricName, em.Labels, em.Ref.Type, em.Ref.Namespace, em.Ref.Name)
		id := custommetrics.ExternalMetricValueKeyFunc(em)
		externalMetrics[id] = em
	}
	return externalMetrics
}

// ProcessWPAs processes the WatermarkPodAutoscalers into a list of ExternalMetricValues.
func (p *Processor) ProcessWPAs(wpa *v1alpha1.WatermarkPodAutoscaler) map[string]custommetrics.ExternalMetricValue {
	externalMetrics := make(map[string]custommetrics.ExternalMetricValue)
	emList := InspectWPA(wpa)
	for _, em := range emList {
		em.Value = 0
		em.Timestamp = time.Now().Unix()
		em.Valid = false
		log.Tracef("Created a boilerplate for the external metrics %s{%v} for %s %s/%s", em.MetricName, em.Labels, em.Ref.Type, em.Ref.Namespace, em.Ref.Name)
		id := custommetrics.ExternalMetricValueKeyFunc(em)
		externalMetrics[id] = em
	}
	return externalMetrics
}

// UpdateExternalMetrics does the validation and processing of the ExternalMetrics
// TODO if a metric's ts in emList is too recent, no need to add it to the batchUpdate.
func (p *Processor) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) (updated map[string]custommetrics.ExternalMetricValue) {
	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	rollup := config.Datadog.GetInt("external_metrics_provider.rollup")
	maxAge := int64(p.externalMaxAge.Seconds())
	var err error
	updated = make(map[string]custommetrics.ExternalMetricValue)

	batch := []string{}
	for _, e := range emList {
		q := getKey(e.MetricName, e.Labels, aggregator, rollup)
		batch = append(batch, q)
	}

	metrics, err := p.QueryExternalMetric(batch)
	if len(metrics) == 0 && err != nil {
		log.Errorf("Error getting metrics from Datadog: %v", err.Error())
		// If no metrics can be retrieved from Datadog in a given list, we need to invalidate them
		// To avoid undesirable autoscaling behaviors
		return invalidate(emList)
	}

	for id, em := range emList {
		metricIdentifier := getKey(em.MetricName, em.Labels, aggregator, rollup)
		metric := metrics[metricIdentifier]

		if time.Now().Unix()-metric.Timestamp > maxAge || !metric.Valid {
			// invalidating sparse metrics that are outdated
			em.Valid = false
			em.Value = metric.Value
			em.Timestamp = time.Now().Unix()
			updated[id] = em
			continue
		}

		em.Valid = true
		em.Value = metric.Value
		em.Timestamp = metric.Timestamp
		log.Debugf("Updated the external metric %s{%v} for %s %s/%s", em.MetricName, em.Labels, em.Ref.Type, em.Ref.Namespace, em.Ref.Name)
		updated[id] = em
	}
	return updated
}

// queryExternalMetric queries Datadog to validate the availability and value of one or more external metrics
// Also updates the rate limits statistics as a result of the query.
func (p *Processor) QueryExternalMetric(queries []string) (processed map[string]Point, err error) {
	processed = make(map[string]Point)
	if len(queries) == 0 {
		return processed, nil
	}

	bucketSize := config.Datadog.GetInt64("external_metrics_provider.bucket_size")
	chunks := makeChunks(queries)
	log.Tracef("List of batches %v", chunks)

	// we have a number of chunks with `chunkSize` metrics.
	responses := make(chan queryResponse, len(queries))

	var waitResp sync.WaitGroup
	waitResp.Add(len(chunks))
	for _, c := range chunks {
		go func(chunk []string) {
			defer waitResp.Done()
			resp, err := p.queryDatadogExternal(chunk, bucketSize)
			responses <- queryResponse{resp, err}
		}(c)
	}
	waitResp.Wait()
	close(responses)
	var errors []error
	for elem := range responses {
		for k, v := range elem.metrics {
			processed[k] = v
		}
		if elem.err != nil {
			errors = append(errors, elem.err)
		}
	}
	log.Debugf("Processed %d chunks", len(chunks))

	if err := p.updateRateLimitingMetrics(); err != nil {
		errors = append(errors, err)
	}
	return processed, utilserror.NewAggregate(errors)
}

func isURLBeyondLimits(uriLength, numBuckets int) (bool, error) {
	// The metric name can be at maximum 200 characters. Kubernetes limits the labels to 63 characters.
	// Autoscalers with enough labels to form single a query of more than 7k characters are not supported.
	lengthOverspill := uriLength >= maxCharactersPerChunk
	if lengthOverspill && numBuckets == 0 {
		return true, fmt.Errorf("Query is too long, could yield a server side error. Dropping")
	}
	return uriLength >= maxCharactersPerChunk || numBuckets >= chunkSize, nil
}

func makeChunks(batch []string) (chunks [][]string) {
	// uriLength is used to avoid making a query that goes beyond the maximum URI size.
	var uriLength int
	var tempBucket []string

	for _, val := range batch {
		// Length of the query plus comma, time and space aggregators that come later on.
		tempSize := len(url.QueryEscape(val)) + extraQueryCharacters
		uriLength = uriLength + tempSize
		beyond, err := isURLBeyondLimits(uriLength, len(tempBucket))
		if err != nil {
			log.Errorf(fmt.Sprintf("%s: %s", err.Error(), val))
			continue
		}
		if beyond {
			chunks = append(chunks, tempBucket)
			uriLength = tempSize
			tempBucket = []string{val}
			continue
		}
		tempBucket = append(tempBucket, val)
	}
	chunks = append(chunks, tempBucket)
	return chunks
}

func invalidate(emList map[string]custommetrics.ExternalMetricValue) (invList map[string]custommetrics.ExternalMetricValue) {
	invList = make(map[string]custommetrics.ExternalMetricValue)
	for id, e := range emList {
		e.Valid = false
		e.Timestamp = metav1.Now().Unix()
		invList[id] = e
	}
	return invList
}

func getKey(name string, labels map[string]string, aggregator string, rollup int) string {
	// Support queries with no tags
	var result string

	if len(labels) == 0 {
		result = fmt.Sprintf("%s{*}", name)
	} else {
		datadogTags := []string{}
		for key, val := range labels {
			datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", key, val))
		}
		sort.Strings(datadogTags)
		tags := strings.Join(datadogTags, ",")
		result = fmt.Sprintf("%s{%s}", name, tags)
	}

	return fmt.Sprintf("%s:%s.rollup(%d)", aggregator, result, rollup)
}
