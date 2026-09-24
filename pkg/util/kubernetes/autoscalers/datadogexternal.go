// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	ddRequests = telemetryimpl.GetCompatComponent().NewCounterWithOpts("", "datadog_requests",
		[]string{"status", le.JoinLeaderLabel}, "Counter of requests made to Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	metricsEval = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "external_metrics_processed_value",
		[]string{"metric", le.JoinLeaderLabel}, "value processed from querying Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	metricsUpdated = telemetryimpl.GetCompatComponent().NewCounterWithOpts("", "external_metrics_updated",
		[]string{"metric", le.JoinLeaderLabel}, "increased by 1 everytime a metric value is updated",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	metricsDelay = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "external_metrics_delay_seconds",
		[]string{"metric", le.JoinLeaderLabel}, "freshness of the metric evaluated from querying Datadog",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsRemaining = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "rate_limit_queries_remaining",
		[]string{"endpoint", le.JoinLeaderLabel}, "number of queries remaining before next reset",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsRemainingMin = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "rate_limit_queries_remaining_min",
		[]string{"endpoint", le.JoinLeaderLabel}, "minimum number of queries remaining before next reset observed during an expiration interval of 2*refresh period",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsReset = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "rate_limit_queries_reset",
		[]string{"endpoint", le.JoinLeaderLabel}, "number of seconds before next reset",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsPeriod = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "rate_limit_queries_period",
		[]string{"endpoint", le.JoinLeaderLabel}, "period of rate limiting",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rateLimitsLimit = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("", "rate_limit_queries_limit",
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
	for _, query := range ddQueries {
		if err := validateDatadogExternalQuery(query); err != nil {
			return nil, NewProcessingError(fmt.Sprintf("invalid query %q: %v", query, err))
		}
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
	matchedQueries := make([]string, len(seriesSlice))
	for i, serie := range seriesSlice {
		matchedQuery, err := matchDatadogExternalSeries(serie, ddQueries)
		if err != nil {
			log.Errorf("Received Serie that does not match the submitted query batch. Full query: %s / Serie expression: %v / QueryIndex: %v / Error: %v", batchedQuery, serie.Expression, serie.QueryIndex, err)
			return invalidQueryResults(ddQueries, currentTimeUnix, "Datadog API response did not match the submitted query batch"), nil
		}
		matchedQueries[i] = matchedQuery
	}

	for i, serie := range seriesSlice {
		matchedQuery := matchedQueries[i]

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

func invalidQueryResults(queries []string, timestamp int64, message string) map[string]Point {
	results := make(map[string]Point, len(queries))
	for _, query := range queries {
		results[query] = Point{
			Timestamp: timestamp,
			Error:     NewProcessingError(message),
		}
	}
	return results
}

func matchDatadogExternalSeries(serie datadog.Series, queries []string) (string, error) {
	queryIndex := 0
	if serie.QueryIndex == nil {
		if len(queries) != 1 {
			return "", fmt.Errorf("missing QueryIndex for a batch of %d queries", len(queries))
		}
	} else {
		queryIndex = *serie.QueryIndex
		if queryIndex < 0 || queryIndex >= len(queries) {
			return "", fmt.Errorf("QueryIndex %d is outside the batch of %d queries", queryIndex, len(queries))
		}
	}

	return queries[queryIndex], nil
}

// validateDatadogExternalQuery ensures a tenant-provided query cannot escape
// its position in a comma-delimited request batch. Commas nested in functions,
// scopes, or quoted strings remain valid.
func validateDatadogExternalQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("query is empty")
	}

	var delimiters []rune
	var quote rune
	escaped := false
	for _, char := range query {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}

		switch char {
		case '\'', '"':
			quote = char
		case '(', '{', '[':
			delimiters = append(delimiters, char)
		case ')', '}', ']':
			if len(delimiters) == 0 || !matchingDelimiters(delimiters[len(delimiters)-1], char) {
				return fmt.Errorf("unbalanced delimiter %q", char)
			}
			delimiters = delimiters[:len(delimiters)-1]
		case ',':
			if len(delimiters) == 0 {
				return errors.New("query contains a top-level comma")
			}
		}
	}

	if quote != 0 {
		return errors.New("query contains an unterminated quoted string")
	}
	if len(delimiters) != 0 {
		return fmt.Errorf("query contains an unclosed delimiter %q", delimiters[len(delimiters)-1])
	}
	return nil
}

func matchingDelimiters(open, close rune) bool {
	return open == '(' && close == ')' || open == '{' && close == '}' || open == '[' && close == ']'
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
