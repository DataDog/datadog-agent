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

	"github.com/paulbellamy/ratecounter"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	datadogStats          = expvar.NewMap("datadog-api")
	datadogErrors         = &expvar.Int{}
	datadogQueriesPerHour = &expvar.Int{}
	datadogQueriesCounter = ratecounter.NewRateCounter(1 * time.Hour)
)

func init() {
	datadogStats.Set("Errors", datadogErrors)
	datadogStats.Set("QueriesPerHour", datadogQueriesPerHour)
}

// queryDatadogExternal converts the metric name and labels from the HPA format into a Datadog metric.
// It returns the last value for a bucket of 5 minutes,
func (hpa *HPAProcessor) queryDatadogExternal(metricName string, tags map[string]string) (int64, error) {
	if metricName == "" || len(tags) == 0 {
		return 0, errors.New("invalid metric to query")
	}
	bucketSize := config.Datadog.GetInt64("external_metrics_provider.bucket_size")
	datadogTags := []string{}

	for key, val := range tags {
		datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", key, val))
	}
	tagString := strings.Join(datadogTags, ",")

	// TODO: offer other aggregations than avg.
	query := fmt.Sprintf("avg:%s{%s}", metricName, tagString)

	datadogQueriesCounter.Incr(1)
	datadogQueriesPerHour.Set(datadogQueriesCounter.Rate())

	seriesSlice, err := hpa.datadogClient.QueryMetrics(time.Now().Unix()-bucketSize, time.Now().Unix(), query)

	if err != nil {
		datadogErrors.Add(1)
		return 0, log.Errorf("Error while executing metric query %s: %s", query, err)
	}

	if len(seriesSlice) == 0 {
		return 0, log.Errorf("Returned series slice empty")
	}
	points := seriesSlice[0].Points

	if len(points) == 0 {
		return 0, log.Errorf("No points in series")
	}
	lastValue := int64(points[len(points)-1][1])
	return lastValue, nil
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
