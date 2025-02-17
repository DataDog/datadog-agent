// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	metricsUpdated = telemetry.NewCounterWithOpts("", "external_metrics_updated",
		[]string{"metric", le.JoinLeaderLabel}, "increased by 1 everytime a metric value is updated",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	metricsDelay = telemetry.NewGaugeWithOpts("", "external_metrics_delay_seconds",
		[]string{"metric", le.JoinLeaderLabel}, "freshness of the metric evaluated from querying Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsRemaining = telemetry.NewGaugeWithOpts("", "rate_limit_queries_remaining",
		[]string{"endpoint", le.JoinLeaderLabel}, "number of queries remaining before next reset",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsRemainingMin = telemetry.NewGaugeWithOpts("", "rate_limit_queries_remaining_min",
		[]string{"endpoint", le.JoinLeaderLabel}, "minimum number of queries remaining before next reset observed during an expiration interval of 2*refresh period",
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

var (
	minRemainingRequestsTracker *minTracker
	once                        sync.Once
)

func getMinRemainingRequestsTracker() *minTracker {
	once.Do(func() {
		refreshPeriod := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.refresh_period")
		expiryDuration := 2 * refreshPeriod
		minRemainingRequestsTracker = newMinTracker(time.Duration(time.Duration(expiryDuration) * time.Second))
	})

	return minRemainingRequestsTracker
}

// queryDatadogExternal converts the metric name and labels from the Ref format into a Datadog metric.
// It should ALWAYS return either: (nil, err) or (map[string]Point, nil)
// The former is used to signal a global error with the query, the latter is used to signal a successful query with potentially some errors per query.
func (p *Processor) queryDatadogExternal(currentTime time.Time, ddQueries []string, timeWindow time.Duration) (map[string]Point, error) {
	ddQueriesLen := len(ddQueries)
	if ddQueriesLen == 0 {
		log.Tracef("No query in input - nothing to do")
		return nil, nil
	}

	batchedQuery := strings.Join(ddQueries, ",")
	currentTimeUnix := currentTime.Unix()
	seriesSlice, err := p.datadogClient.QueryMetrics(currentTime.Add(-timeWindow).Unix(), currentTimeUnix, batchedQuery)
	if err != nil {
		apiErr := NewAPIError(err)
		switch {
		case apiErr.Code == RateLimitExceededAPIError:
			ddRequests.Inc("rate_limit_error", le.JoinLeaderValue)
		case apiErr.Code == UnprocessableEntityAPIError:
			ddRequests.Inc("unprocessable_entity_error", le.JoinLeaderValue)
		case apiErr.Code == DatadogAPIError:
			ddRequests.Inc("response_error", le.JoinLeaderValue)
			log.Debugf("Error while executing queries %v, err: %v", ddQueries, err)
		case apiErr.Code == OtherHTTPStatusCodeAPIError:
			ddRequests.Inc("other_http_error", le.JoinLeaderValue)
			log.Debugf("Error while executing queries %v, err: %v", ddQueries, err)
		default:
			ddRequests.Inc("unknown_error", le.JoinLeaderValue)
			log.Debugf("Error while executing queries %v, err: %v", ddQueries, err)
		}
		return nil, apiErr
	}
	ddRequests.Inc("success", le.JoinLeaderValue)

	processedMetrics := make(map[string]Point, ddQueriesLen)
	for _, serie := range seriesSlice {
		// Perform matching between query and reply, using query order and `QueryIndex` from API reply (QueryIndex is 0-based)
		var matchedQuery string
		if ddQueriesLen > 1 {
			if serie.QueryIndex != nil && *serie.QueryIndex < ddQueriesLen {
				matchedQuery = ddQueries[*serie.QueryIndex]
			} else {
				log.Errorf("Received Serie without QueryIndex or invalid QueryIndex while we sent multiple queries. Full query: %s / Serie expression: %v / QueryIndex: %v", batchedQuery, serie.Expression, serie.QueryIndex)
				continue
			}
		} else {
			matchedQuery = ddQueries[0]
		}

		// Result point, by default it's invalid
		resultPoint := Point{
			Timestamp: currentTimeUnix,
			Valid:     false,
		}

		// Check if we already have a Serie result for this query. We expect query to result in a single Serie
		// Otherwise we are not able to determine which value we should take for Autoscaling
		if _, found := processedMetrics[matchedQuery]; found {
			resultPoint.Error = NewProcessingError("multiple series found. Please change your query to return a single serie")
			processedMetrics[matchedQuery] = resultPoint
			continue
		}

		// As we batch queries to Datadog API, all returned series return the same number of points, aligned to the smallest rollup value.
		// This means that a lot of points can be `nil`.
		// What we want is to find the two most recent points that are not `nil` and use the penultimate one if present, otherwise the last one.
		var matchedPoint *datadog.DataPoint
		for i := len(serie.Points) - 1; i >= 0; i-- {
			if serie.Points[i][value] == nil {
				continue
			}

			if matchedPoint == nil {
				matchedPoint = &serie.Points[i]
			} else {
				// Penuultimate point found, we can stop here.
				matchedPoint = &serie.Points[i]
				break
			}
		}

		// No point found, we can't do anything with this serie.
		if matchedPoint == nil {
			errString := fmt.Sprintf("only null values found in API response (%d points), check data is available in the last %.0f seconds", len(serie.Points), timeWindow.Seconds())
			if serie.Interval != nil {
				errString += fmt.Sprintf(" (interval was %d)", *serie.Interval)
			}

			resultPoint.Error = NewProcessingError(errString)
			processedMetrics[matchedQuery] = resultPoint
			continue
		}

		resultPoint.Timestamp = int64(*matchedPoint[timestamp] / 1000)
		resultPoint.Value = *matchedPoint[value]
		resultPoint.Valid = true
		processedMetrics[matchedQuery] = resultPoint

		// Prometheus submissions on the processed external metrics
		metricTag := fmt.Sprintf("%s{%s}", *serie.Metric, *serie.Scope)
		metricsEval.Set(resultPoint.Value, metricTag, le.JoinLeaderValue)
		metricsUpdated.Inc(metricTag, le.JoinLeaderValue)
		delay := currentTimeUnix - resultPoint.Timestamp
		metricsDelay.Set(float64(delay), metricTag, le.JoinLeaderValue)
	}

	// If the returned Series is empty for one or more processedMetrics, add it as invalid
	for _, ddQuery := range ddQueries {
		if _, found := processedMetrics[ddQuery]; !found {
			processedMetrics[ddQuery] = Point{
				Timestamp: currentTimeUnix,
				Error:     NewProcessingError("no serie was found for this query in API Response, check Cluster Agent logs for QueryIndex errors"),
			}
		}
	}

	// Update rateLimitsRemainingMin metric
	updateMap := p.datadogClient.GetRateLimitStats()
	queryLimits := updateMap[queryEndpoint]
	newVal, err := strconv.Atoi(queryLimits.Remaining)
	if err == nil {
		getMinRemainingRequestsTracker().update(newVal)
		rateLimitsRemainingMin.Set(float64(minRemainingRequestsTracker.get()), queryEndpoint, le.JoinLeaderLabel)
	}

	return processedMetrics, nil
}

// setTelemetryMetric is a helper to submit telemetry metrics
func setTelemetryMetric(val string, metric telemetry.Gauge) {
	valFloat, err := strconv.Atoi(val)
	if err == nil {
		metric.Set(float64(valFloat), queryEndpoint, le.JoinLeaderValue)
	}
}

func (p *Processor) updateRateLimitingMetrics() {
	updateMap := p.datadogClient.GetRateLimitStats()
	queryLimits := updateMap[queryEndpoint]

	setTelemetryMetric(queryLimits.Limit, rateLimitsLimit)
	setTelemetryMetric(queryLimits.Remaining, rateLimitsRemaining)
	setTelemetryMetric(queryLimits.Period, rateLimitsPeriod)
	setTelemetryMetric(queryLimits.Reset, rateLimitsReset)
}
