// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	ddRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datadog_requests",
			Help: "Counter of requests made to Datadog",
		},
		[]string{"status"},
	)
	metricsRawVal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "external_metrics_raw",
		Help: "raw value gotten from querying Datadog",
	},
		[]string{"metric", "scope"},
	)
)

func init() {
	prometheus.MustRegister(ddRequests)
	prometheus.MustRegister(metricsRawVal)
}

type Point struct {
	value     int64
	timestamp int64
	valid     bool
}

const (
	value     = 1
	timestamp = 0
)

// queryDatadogExternal converts the metric name and labels from the HPA format into a Datadog metric.
// It returns the last value for a bucket of 5 minutes,
func (p *Processor) queryDatadogExternal(metricNames []string) (map[string]Point, error) {
	if metricNames == nil {
		log.Tracef("No processed external metrics to query")
		return nil, nil
	}
	bucketSize := config.Datadog.GetInt64("external_metrics_provider.bucket_size")

	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	rollup := config.Datadog.GetInt("external_metrics.query_rollup")
	var toQuery []string
	for _, metric := range metricNames {
		toQuery = append(toQuery, fmt.Sprintf("%s:%s.rollup(%d)", aggregator, metric, rollup))
	}

	query := strings.Join(toQuery, ",")

	seriesSlice, err := p.datadogClient.QueryMetrics(time.Now().Unix()-bucketSize, time.Now().Unix(), query)
	if err != nil {
		ddRequests.WithLabelValues("error").Inc()
		return nil, log.Errorf("Error while executing metric query %s: %s", query, err)
	}
	ddRequests.WithLabelValues("success").Inc()

	processedMetrics := make(map[string]Point)
	for _, name := range metricNames {
		// If the returned Series is empty for one or more processedMetrics, add it as invalid now
		// so it can be retried later.
		processedMetrics[name] = Point{
			timestamp: time.Now().Unix(),
		}
	}

	// Go through processedMetrics output, extract last value and timestamp - If no series found return invalid metrics.
	if len(seriesSlice) == 0 {
		return processedMetrics, log.Errorf("Returned series slice empty")
	}

	for _, serie := range seriesSlice {
		if serie.Metric == nil {
			log.Infof("Could not collect values for all processedMetrics in the query %s", query)
			continue
		}

		// Because of the intake pipeline, the very last bucket can be subject to variations - Let's use the previous bucket.
		fullBucketSeen := 0
		var point Point
		// Find the most recent value.
		for i := len(serie.Points) - 1; i >= 0; i-- {
			if serie.Points[i][value] == nil {
				// We need this as if multiple metrics are queried, their points' timestamps align this can result in empty values.
				continue
			}
			// If there is only one datapoint in the bucket, let's use it.
			// This can happen with the batch of sparse and a regular metric.
			if fullBucketSeen == 0 && len(serie.Points) > 2 {
				// Do not rely on the value stored in the last bucket
				fullBucketSeen = fullBucketSeen + 1
				continue
			}
			point.value = int64(*serie.Points[i][value])                // store the original value
			point.timestamp = int64(*serie.Points[i][timestamp] / 1000) // Datadog's API returns timestamps in ms
			point.valid = true

			metricsRawVal.WithLabelValues(*serie.Metric, *serie.Scope).Set(float64(point.value))

			m := fmt.Sprintf("%s{%s}", *serie.Metric, *serie.Scope)
			processedMetrics[m] = point

			log.Debugf("Validated %s | Value:%d at %d after %d/%d buckets", *serie.Metric, point.value, point.timestamp, i+1, len(serie.Points))
			break
		}
	}
	return processedMetrics, nil
}

// NewDatadogClient generates a new client to query metrics from Datadog
func NewDatadogClient() (*datadog.Client, error) {
	apiKey := config.Datadog.GetString("api_key")
	appKey := config.Datadog.GetString("app_key")

	if appKey == "" || apiKey == "" {
		return nil, errors.New("missing the api/app key pair to query Datadog")
	}
	log.Infof("Initialized the Datadog Client for HPA")
	return datadog.NewClient(apiKey, appKey), nil
}
