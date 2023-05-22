// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	utilserror "k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	ddRequests = telemetry.NewCounterWithOpts("", "datadog_requests",
		[]string{"status", le.JoinLeaderLabel}, "Counter of requests made to Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	metricsEval = telemetry.NewGaugeWithOpts("", "external_metrics_processed_value",
		[]string{"metric", le.JoinLeaderLabel}, "value processed from querying Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	metricsDelay = telemetry.NewGaugeWithOpts("", "external_metrics_delay_seconds",
		[]string{"metric", le.JoinLeaderLabel}, "freshness of the metric evaluated from querying Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsRemaining = telemetry.NewGaugeWithOpts("", "rate_limit_queries_remaining",
		[]string{"endpoint", le.JoinLeaderLabel}, "number of queries remaining before next reset",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsReset = telemetry.NewGaugeWithOpts("", "rate_limit_queries_reset",
		[]string{"endpoint", le.JoinLeaderLabel}, "number of seconds before next reset",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsPeriod = telemetry.NewGaugeWithOpts("", "rate_limit_queries_period",
		[]string{"endpoint", le.JoinLeaderLabel}, "period of rate limiting",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsLimit = telemetry.NewGaugeWithOpts("", "rate_limit_queries_limit",
		[]string{"endpoint", le.JoinLeaderLabel}, "maximum number of queries allowed in the period",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

// Point represents a metric data point
type Point struct {
	Value     float64
	Timestamp int64
	Valid     bool
	Error     error
}

const (
	value         = 1
	timestamp     = 0
	queryEndpoint = "/api/v1/query"
)

// queryDatadogExternal converts the metric name and labels from the Ref format into a Datadog metric.
// It returns the last value for a bucket of 5 minutes,
func (p *Processor) queryDatadogExternal(ddQueries []string, timeWindow time.Duration) (map[string]Point, error) {
	ddQueriesLen := len(ddQueries)
	if ddQueriesLen == 0 {
		log.Tracef("No query in input - nothing to do")
		return nil, nil
	}

	query := strings.Join(ddQueries, ",")
	currentTime := time.Now()
	seriesSlice, err := p.datadogClient.QueryMetrics(currentTime.Add(-timeWindow).Unix(), currentTime.Unix(), query)
	if err != nil {
		ddRequests.Inc("error", le.JoinLeaderValue)
		return nil, fmt.Errorf("error while executing metric query %s: %w", query, err)
	}
	ddRequests.Inc("success", le.JoinLeaderValue)

	processedMetrics := make(map[string]Point, ddQueriesLen)
	for _, serie := range seriesSlice {
		if serie.Metric == nil {
			log.Infof("Could not collect values for all processedMetrics in the query %s", query)
			continue
		}

		// Perform matching between query and reply, using query order and `QueryIndex` from API reply (QueryIndex is 0-based)
		var queryIndex int
		if ddQueriesLen > 1 {
			if serie.QueryIndex != nil && *serie.QueryIndex < ddQueriesLen {
				queryIndex = *serie.QueryIndex
			} else {
				log.Errorf("Received Serie without QueryIndex or invalid QueryIndex while we sent multiple queries. Full query: %s / Serie expression: %v / QueryIndex: %v", query, serie.Expression, serie.QueryIndex)
				continue
			}
		}

		// Check if we already have a Serie result for this query. We expect query to result in a single Serie
		// Otherwise we are not able to determine which value we should take for Autoscaling
		if existingPoint, found := processedMetrics[ddQueries[queryIndex]]; found {
			if existingPoint.Valid {
				existingPoint.Valid = false
				existingPoint.Timestamp = currentTime.Unix()
				existingPoint.Error = errors.New("multiple series found. Please change your query to return a single serie")
				processedMetrics[ddQueries[queryIndex]] = existingPoint
			}
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
			point.Value = *serie.Points[i][value]                       // store the original value
			point.Timestamp = int64(*serie.Points[i][timestamp] / 1000) // Datadog's API returns timestamps in s
			point.Valid = true

			m := fmt.Sprintf("%s{%s}", *serie.Metric, *serie.Scope)
			processedMetrics[ddQueries[queryIndex]] = point

			// Prometheus submissions on the processed external metrics
			metricsEval.Set(point.Value, m, le.JoinLeaderValue)
			precision := currentTime.Unix() - point.Timestamp
			metricsDelay.Set(float64(precision), m, le.JoinLeaderValue)

			log.Debugf("Validated %s | Value:%v at %d after %d/%d buckets", ddQueries[queryIndex], point.Value, point.Timestamp, i+1, len(serie.Points))
			break
		}
	}

	// If the returned Series is empty for one or more processedMetrics, add it as invalid
	for _, ddQuery := range ddQueries {
		if _, found := processedMetrics[ddQuery]; !found {
			processedMetrics[ddQuery] = Point{
				Timestamp: time.Now().Unix(),
				Error:     fmt.Errorf("no serie returned for this query, check data is available in the last %.0f seconds", math.Ceil(timeWindow.Seconds())),
			}
		}
	}

	return processedMetrics, nil
}

// setTelemetryMetric is a helper to submit telemetry metrics
func setTelemetryMetric(val string, metric telemetry.Gauge) error {
	valFloat, err := strconv.Atoi(val)
	if err == nil {
		metric.Set(float64(valFloat), queryEndpoint, le.JoinLeaderValue)
	}
	return err
}

func (p *Processor) updateRateLimitingMetrics() error {
	updateMap := p.datadogClient.GetRateLimitStats()
	queryLimits := updateMap[queryEndpoint]

	errors := []error{
		setTelemetryMetric(queryLimits.Limit, rateLimitsLimit),
		setTelemetryMetric(queryLimits.Remaining, rateLimitsRemaining),
		setTelemetryMetric(queryLimits.Period, rateLimitsPeriod),
		setTelemetryMetric(queryLimits.Reset, rateLimitsReset),
	}

	return utilserror.NewAggregate(errors)
}
