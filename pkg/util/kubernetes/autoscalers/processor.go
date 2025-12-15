// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/watermarkpodautoscaler/apis/datadoghq/v1alpha1"
	"golang.org/x/sync/errgroup"

	datadogclientcomp "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/custommetrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxCharactersPerChunk is the maximum size of a single chunk to avoid 414 Request-URI Too Large
	maxCharactersPerChunk = 7000
	// extraQueryCharacters accounts for the extra characters added to form a query to Datadog's API (e.g.: `avg:`, `.rollup(X)` ...)
	extraQueryCharacters = 16
	// maxParallelQueries returns the maximum number of parallel queries to Datadog.
	// This value corresponds to a very high usage (max seen internally) and is just there to avoid mistakes in the configuration.
	maxParallelQueries = 300
)

// ProcessorInterface is used to easily mock the interface for testing
type ProcessorInterface interface {
	UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue
	QueryExternalMetric(queries []string, timeWindow time.Duration) map[string]Point
	ProcessEMList(emList []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue
}

// Processor embeds the configuration to refresh metrics from Datadog and process Ref structs to ExternalMetrics.
type Processor struct {
	externalMaxAge  time.Duration
	datadogClient   datadogclientcomp.Component
	parallelQueries int
}

// NewProcessor returns a new Processor
func NewProcessor(datadogCl datadogclientcomp.Component) *Processor {
	externalMaxAge := math.Max(pkgconfigsetup.Datadog().GetFloat64("external_metrics_provider.max_age"), 3*pkgconfigsetup.Datadog().GetFloat64("external_metrics_provider.rollup"))
	parallelQueries := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.max_parallel_queries")
	if parallelQueries > maxParallelQueries || parallelQueries <= 0 {
		parallelQueries = maxParallelQueries
	}

	return &Processor{
		externalMaxAge:  time.Duration(externalMaxAge) * time.Second,
		datadogClient:   datadogCl,
		parallelQueries: parallelQueries,
	}
}

// ProcessEMList processes a list of ExternalMetricValue.
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
func (p *Processor) ProcessHPAs(hpa interface{}) map[string]custommetrics.ExternalMetricValue {
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

// GetDefaultMaxAge returns the configured default max age.
func GetDefaultMaxAge() time.Duration {
	return time.Duration(pkgconfigsetup.Datadog().GetInt64("external_metrics_provider.max_age")) * time.Second
}

// GetDefaultTimeWindow returns the configured default time window
func GetDefaultTimeWindow() time.Duration {
	return time.Duration(pkgconfigsetup.Datadog().GetInt64("external_metrics_provider.bucket_size")) * time.Second
}

// GetDefaultMaxTimeWindow returns the configured max time window
func GetDefaultMaxTimeWindow() time.Duration {
	return time.Duration(pkgconfigsetup.Datadog().GetInt64("external_metrics_provider.max_time_window")) * time.Second
}

// UpdateExternalMetrics does the validation and processing of the ExternalMetrics
// TODO if a metric's ts in emList is too recent, no need to add it to the batchUpdate.
func (p *Processor) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) (updated map[string]custommetrics.ExternalMetricValue) {
	aggregator := pkgconfigsetup.Datadog().GetString("external_metrics.aggregator")
	rollup := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.rollup")
	maxAge := int64(p.externalMaxAge.Seconds())

	updated = make(map[string]custommetrics.ExternalMetricValue)
	if len(emList) == 0 {
		return updated
	}

	uniqueQueries := make(map[string]struct{}, len(emList))
	batch := make([]string, 0, len(emList))
	for _, e := range emList {
		q := getKey(e.MetricName, e.Labels, aggregator, rollup)
		if _, found := uniqueQueries[q]; !found {
			uniqueQueries[q] = struct{}{}
			batch = append(batch, q)
		}
	}

	// In non-DatadogMetric path, we don't have any custom maxAge possible, always use default time window
	metrics := p.QueryExternalMetric(batch, GetDefaultTimeWindow())
	if len(metrics) == 0 {
		log.Errorf("Unexpected return from QueryExternalMetric, no data and no error")
	}

	for id, em := range emList {
		metricIdentifier := getKey(em.MetricName, em.Labels, aggregator, rollup)
		metric := metrics[metricIdentifier]

		if metric.Error != nil || time.Now().Unix()-metric.Timestamp > maxAge || !metric.Valid {
			// invalidating if error found it to avoid autoscaling
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

// QueryExternalMetric queries Datadog to validate the availability and value of one or more external metrics
// Also updates the rate limits statistics as a result of the query.
func (p *Processor) QueryExternalMetric(queries []string, timeWindow time.Duration) map[string]Point {
	if len(queries) == 0 {
		return nil
	}
	// Set query time
	currentTime := time.Now()

	// Preparing storage for results
	responses := make(map[string]Point, len(queries))
	responsesGlobalErrors := 0
	// Protect both responses and responsesGlobalError
	responsesLock := sync.Mutex{}

	// Chunk the queries
	chunks := makeChunks(queries)

	var group errgroup.Group
	group.SetLimit(p.parallelQueries)

	for _, chunk := range chunks {
		group.Go(func() error {
			// Either resp or err is nil
			resp, err := p.queryDatadogExternal(currentTime, chunk, timeWindow)

			// Not holding lock during network calls
			responsesLock.Lock()
			defer responsesLock.Unlock()

			// Process the response
			if err != nil {
				responsesGlobalErrors++
				for _, q := range chunk {
					responses[q] = Point{Error: err}
				}
			} else {
				maps.Copy(responses, resp)
			}
			return nil
		})
	}
	// Errors are handled in `responses`, so we don't need to check the group error
	_ = group.Wait()

	log.Debugf("Processed %d chunks with %d chunks in global error", len(chunks), responsesGlobalErrors)
	return responses
}

func isURLBeyondLimits(uriLength, numBuckets int) (bool, error) {
	// The metric name can be at maximum 200 characters. Kubernetes limits the labels to 63 characters.
	// Autoscalers with enough labels to form single a query of more than 7k characters are not supported.
	lengthOverspill := uriLength >= maxCharactersPerChunk
	if lengthOverspill && numBuckets == 0 {
		return true, errors.New("Query is too long, could yield a server side error. Dropping")
	}

	chunkSize := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.chunk_size")

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
			log.Errorf("%v: %s", err, val)
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

func getKey(name string, labels map[string]string, aggregator string, rollup int) string {
	// Support queries with no tags
	var result string

	if len(labels) == 0 {
		result = name + "{*}"
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
