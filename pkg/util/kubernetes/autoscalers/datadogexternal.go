// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package autoscalers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
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
	metricsEval = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "external_metrics_processed_value",
		Help: "value processed from querying Datadog",
	},
		[]string{"metric"},
	)
	metricsDelay = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "external_metrics_delay_seconds",
		Help: "freshness of the metric evaluated from querying Datadog",
	},
		[]string{"metric"},
	)
)

func init() {
	prometheus.MustRegister(ddRequests)
	prometheus.MustRegister(metricsEval)
	prometheus.MustRegister(metricsDelay)
}

type Point struct {
	value     float64
	timestamp int64
	valid     bool
}

const (
	value     = 1
	timestamp = 0
)

// queryDatadogExternal converts the metric name and labels from the Ref format into a Datadog metric.
// It returns the last value for a bucket of 5 minutes,
func (p *Processor) queryDatadogExternal(metricNames []string) (map[string]Point, error) {
	if metricNames == nil {
		log.Tracef("No processed external metrics to query")
		return nil, nil
	}
	// TODO move viper parameters to the Processor struct
	bucketSize := config.Datadog.GetInt64("external_metrics_provider.bucket_size")

	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	rollup := config.Datadog.GetInt("external_metrics_provider.rollup")
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

		// Use on the penultimate bucket, since the very last bucket can be subject to variations due to late points.
		var skippedLastPoint bool
		var point Point
		// Find the most recent value.
		for i := len(serie.Points) - 1; i >= 0; i-- {
			if serie.Points[i][value] == nil {
				// We need this as if multiple metrics are queried, their points' timestamps align this can result in empty values.
				continue
			}
			// We need at least 2 points per window queried on batched metrics.
			// If a single sparse metric is processed and only has 1 point in the window, use the value.
			if !skippedLastPoint && len(serie.Points) > 1 {
				// Skip last point unless the query window only contains one valid point
				skippedLastPoint = true
				continue
			}
			point.value = *serie.Points[i][value]                       // store the original value
			point.timestamp = int64(*serie.Points[i][timestamp] / 1000) // Datadog's API returns timestamps in s
			point.valid = true

			m := fmt.Sprintf("%s{%s}", *serie.Metric, *serie.Scope)
			processedMetrics[m] = point

			// Prometheus submissions on the processed external metrics
			metricsEval.WithLabelValues(m).Set(float64(point.value))
			precision := time.Now().Unix() - point.timestamp
			metricsDelay.WithLabelValues(m).Set(float64(precision))

			log.Debugf("Validated %s | Value:%v at %d after %d/%d buckets", m, point.value, point.timestamp, i+1, len(serie.Points))
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

	client := datadog.NewClient(apiKey, appKey)
	client.HttpClient.Transport = httputils.CreateHTTPTransport()

	return client, nil
}
