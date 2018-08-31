// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"errors"
	"expvar"
	"fmt"
	"strings"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	datadogStats   = expvar.NewMap("datadog-api")
	datadogErrors  = &expvar.Int{}
	datadogQueries = &expvar.Int{}
)

func init() {
	datadogStats.Set("Errors", datadogErrors)
	datadogStats.Set("Queries", datadogQueries)
}

type Metric struct {
	value     int64
	timestamp int64
	valid     bool
}

// queryDatadogExternal converts the metric name and labels from the HPA format into a Datadog metric.
// It returns the last value for a bucket of 5 minutes,
func (p *Processor) queryDatadogExternal(metricNames []string) (map[string]Metric, error) {
	if metricNames == nil {
		return nil, errors.New("no processedMetrics to query")
	}
	bucketSize := config.Datadog.GetInt64("external_metrics_provider.bucket_size")

	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	var toQuery []string
	for _, metric := range metricNames {
		toQuery = append(toQuery, fmt.Sprintf("%s:%s", aggregator, metric))
	}

	query := strings.Join(toQuery, ",")

	datadogQueries.Add(1)

	seriesSlice, err := p.datadogClient.QueryMetrics(time.Now().Unix()-bucketSize, time.Now().Unix(), query)
	if err != nil {
		datadogErrors.Add(1)
		return nil, log.Errorf("Error while executing metric query %s: %s", query, err)
	}

	processedMetrics := make(map[string]Metric)
	for _, name := range metricNames {
		// If the returned Series is empty for one or more processedMetrics
		// and retry later.
		processedMetrics[name] = Metric{
			timestamp: time.Now().Unix(),
		}
	}

	// Go through processedMetrics output, extract last value and timestamp - If not found, valid = false
	// metric.name without aggregator
	if len(seriesSlice) == 0 {
		return processedMetrics, log.Errorf("Returned series slice empty")
	}

	for _, serie := range seriesSlice {
		if serie.Metric == nil {
			log.Infof("Could not collect values for all processedMetrics in the query %s", query)
			continue
		}
		var metric Metric
		// find the most recent value
		// We need this as if multiple processedMetrics are queried, they align the timestamps and can result in empty values.
		for i := len(serie.Points) - 1; i >= 0; i-- {
			if serie.Points[i][1] == nil {
				// If a batch has several metrics with different granularity, some points can be nil.
				continue
			}
			metric.value = int64(*serie.Points[i][1])            // store the original value
			metric.timestamp = int64(*serie.Points[i][0] / 1000) // Datadog's API returns timestamps in ms
			metric.valid = true
			m := fmt.Sprintf("%s{%s}", *serie.Metric, *serie.Scope)
			processedMetrics[m] = metric

			log.Debugf("Validated %#v after %d/%d values", metric, i, len(serie.Points)-1)
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
