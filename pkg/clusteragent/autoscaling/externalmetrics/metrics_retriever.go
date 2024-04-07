// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	invalidMetricErrorMessage                  string = "%v, query was: %s"
	invalidMetricErrorWithRetriesMessage       string = "%v, query was: %s, will retry after %s."
	invalidMetricOutdatedErrorMessage          string = "Query returned outdated result, check MaxAge setting, query: %s"
	invalidMetricNotFoundErrorMessage          string = "Unexpected error, query data not found in result, query: %s"
	invalidMetricGlobalErrorMessage            string = "Global error (all queries) from backend, invalid syntax in query? Check Cluster Agent leader logs for details"
	invalidMetricGlobalErrorWithRetriesMessage string = "Global error (all queries, batch size %d) from backend, invalid syntax in query? Check Cluster Agent leader logs for details. Will retry after %s."
	metricRetrieverStoreID                     string = "mr"
)

// Backoff range for number of retries R:
// For R < 6 random(2^(R-1) * 30, 2^R * 30)
// Otherwise 1800sec
var backoffPolicy backoff.Policy = backoff.NewExpBackoffPolicy(2, 30, 1800, 2, false)

// MetricsRetriever is responsible for querying and storing external metrics
type MetricsRetriever struct {
	refreshPeriod             int64
	metricsMaxAge             int64
	splitBatchBackoffOnErrors bool
	processor                 autoscalers.ProcessorInterface
	store                     *DatadogMetricsInternalStore
	isLeader                  func() bool
}

// NewMetricsRetriever returns a new MetricsRetriever
func NewMetricsRetriever(refreshPeriod, metricsMaxAge int64, processor autoscalers.ProcessorInterface, isLeader func() bool, store *DatadogMetricsInternalStore, splitBatchBackoffOnErrors bool) (*MetricsRetriever, error) {
	return &MetricsRetriever{
		refreshPeriod:             refreshPeriod,
		metricsMaxAge:             metricsMaxAge,
		processor:                 processor,
		store:                     store,
		isLeader:                  isLeader,
		splitBatchBackoffOnErrors: splitBatchBackoffOnErrors,
	}, nil
}

// Run starts retrieving external metrics
func (mr *MetricsRetriever) Run(stopCh <-chan struct{}) {
	log.Infof("Starting MetricsRetriever")
	tickerRefreshProcess := time.NewTicker(time.Duration(mr.refreshPeriod) * time.Second)
	for {
		select {
		case <-tickerRefreshProcess.C:
			if mr.isLeader() {
				startTime := time.Now()
				mr.retrieveMetricsValues()
				retrieverElapsed.Observe(time.Since(startTime).Seconds())
			}
		case <-stopCh:
			log.Infof("Stopping MetricsRetriever")
			return
		}
	}
}

func (mr *MetricsRetriever) retrieveMetricsValues() {
	if mr.splitBatchBackoffOnErrors {
		// We only update active DatadogMetrics
		// We split metrics in two slices, those with errors and those without.
		// Query first slice one by one, other as batch.
		// TODO: consider implementing one-pass splitting in the store
		datadogMetrics := mr.store.GetFiltered(func(datadogMetric model.DatadogMetricInternal) bool {
			return datadogMetric.Active && datadogMetric.Error == nil
		})

		// Do all errors warrant separate query? probably no, but we run them separately because:
		// Backoff should be applied to each metrics separately.
		// Only way to differentiate error from a global error is via comparing error strings.
		datadogMetricsErr := mr.store.GetFiltered(func(datadogMetric model.DatadogMetricInternal) bool {
			return datadogMetric.Active && datadogMetric.Error != nil
		})

		// First split then query because store state is shared and query mutates it
		mr.retrieveMetricsValuesSlice(datadogMetrics)

		// Now test each metric query separately respecting its backoff retry duration elapse value.
		for _, metrics := range datadogMetricsErr {
			if time.Now().After(metrics.RetryAfter) {
				singleton := []model.DatadogMetricInternal{metrics}
				mr.retrieveMetricsValuesSlice(singleton)
			}
		}
	} else {
		// We only update active DatadogMetrics
		datadogMetrics := mr.store.GetFiltered(func(datadogMetric model.DatadogMetricInternal) bool { return datadogMetric.Active })
		mr.retrieveMetricsValuesSlice(datadogMetrics)
	}
}

func (mr *MetricsRetriever) retrieveMetricsValuesSlice(datadogMetrics []model.DatadogMetricInternal) {
	if len(datadogMetrics) == 0 {
		log.Debugf("No active DatadogMetric, nothing to refresh")
		return
	}

	queriesByTimeWindow := getBatchedQueriesByTimeWindow(datadogMetrics)
	resultsByTimeWindow := make(map[time.Duration]map[string]autoscalers.Point)
	globalError := false

	for timeWindow, queries := range queriesByTimeWindow {
		log.Debugf("Starting refreshing external metrics with: %d queries (window: %d)", len(queries), timeWindow)

		results, err := mr.processor.QueryExternalMetric(queries, timeWindow)
		// Check for global failure
		if len(results) == 0 && err != nil {
			globalError = true
			log.Errorf("Unable to fetch external metrics: %v", err)
		}

		resultsByTimeWindow[timeWindow] = results
	}

	// Update store with current results
	currentTime := time.Now().UTC()
	for _, datadogMetric := range datadogMetrics {
		datadogMetricFromStore := mr.store.LockRead(datadogMetric.ID, false)
		if datadogMetricFromStore == nil {
			// This metric is not in the store anymore, discard it
			log.Debugf("Discarding results for DatadogMetric: %s as not present in store anymore", datadogMetric.ID)
			continue
		}

		query := datadogMetric.Query()
		timeWindow := maybeAdjustTimeWindowForQuery(datadogMetric.GetTimeWindow())
		results := resultsByTimeWindow[timeWindow]

		if queryResult, found := results[query]; found {
			log.Debugf("QueryResult from DD for %q: %v", query, queryResult)

			if queryResult.Valid {
				if mr.splitBatchBackoffOnErrors {
					datadogMetricFromStore.Retries = 0
				}
				datadogMetricFromStore.Value = queryResult.Value
				datadogMetricFromStore.DataTime = time.Unix(queryResult.Timestamp, 0).UTC()

				// If we get a valid but old metric, flag it as invalid
				maxAge := datadogMetric.MaxAge
				if maxAge == 0 {
					maxAge = time.Duration(mr.metricsMaxAge) * time.Second
				}

				if time.Duration(currentTime.Unix()-queryResult.Timestamp)*time.Second <= maxAge {
					datadogMetricFromStore.Valid = true
					datadogMetricFromStore.Error = nil
				} else {
					datadogMetricFromStore.Valid = false
					datadogMetricFromStore.Error = fmt.Errorf(invalidMetricOutdatedErrorMessage, query)
				}
			} else {
				datadogMetricFromStore.Valid = false
				if mr.splitBatchBackoffOnErrors {
					incrementRetries(datadogMetricFromStore)
					datadogMetricFromStore.Error = fmt.Errorf(invalidMetricErrorWithRetriesMessage,
						queryResult.Error, query, datadogMetricFromStore.RetryAfter.Format(time.RFC3339))
				} else {
					datadogMetricFromStore.Error = fmt.Errorf(invalidMetricErrorMessage, queryResult.Error, query)
				}
			}
		} else {
			datadogMetricFromStore.Valid = false
			if globalError {
				if mr.splitBatchBackoffOnErrors {
					incrementRetries(datadogMetricFromStore)
					datadogMetricFromStore.Error = fmt.Errorf(invalidMetricGlobalErrorWithRetriesMessage, len(datadogMetrics), datadogMetricFromStore.RetryAfter.Format(time.RFC3339))
				} else {
					datadogMetricFromStore.Error = fmt.Errorf(invalidMetricGlobalErrorMessage)
				}
			} else {
				// This should never happen as `QueryExternalMetric` is filling all missing series
				// if no global error.
				datadogMetricFromStore.Error = log.Errorf(invalidMetricNotFoundErrorMessage, query)
			}
		}
		datadogMetricFromStore.UpdateTime = currentTime

		mr.store.UnlockSet(datadogMetric.ID, *datadogMetricFromStore, metricRetrieverStoreID)
	}
}

func incrementRetries(metricsInternal *model.DatadogMetricInternal) {
	metricsInternal.Retries++
	timeNow := time.Now().UTC()
	backoffDuration := backoffPolicy.GetBackoffDuration(metricsInternal.Retries)
	metricsInternal.RetryAfter = timeNow.Add(backoffDuration)
}

func maybeAdjustTimeWindowForQuery(timeWindow time.Duration) time.Duration {
	configMaxAge := autoscalers.GetDefaultMaxAge()
	if configMaxAge > timeWindow {
		timeWindow = configMaxAge
	}

	// Adjust the time window to the default time window if even if we have a smaller one to get more
	// opportunities to batch queries.
	configTimeWindow := autoscalers.GetDefaultTimeWindow()
	if configTimeWindow > timeWindow {
		timeWindow = configTimeWindow
	}

	// Safeguard against large time window
	configMaxTimeWindow := autoscalers.GetDefaultMaxTimeWindow()
	if timeWindow > configMaxTimeWindow {
		log.Warnf("Querying external metrics with a time window larger than: %v is not allowed, ceiling value", configMaxTimeWindow)
		timeWindow = configMaxTimeWindow
	}

	return timeWindow
}

func getBatchedQueriesByTimeWindow(datadogMetrics []model.DatadogMetricInternal) map[time.Duration][]string {
	// Now we create a map of timeWindow to list of queries. All these queries will be executed with
	// this time window.
	queriesByTimeWindow := make(map[time.Duration][]string)
	unique := make(map[string]struct{}, len(datadogMetrics))
	for _, datadogMetric := range datadogMetrics {
		query := datadogMetric.Query()
		timeWindow := maybeAdjustTimeWindowForQuery(datadogMetric.GetTimeWindow())

		key := query + "-" + timeWindow.String()
		if _, found := unique[key]; !found {
			unique[key] = struct{}{}

			if _, found := queriesByTimeWindow[timeWindow]; !found {
				queriesByTimeWindow[timeWindow] = make([]string, 0)
			}
			queriesByTimeWindow[timeWindow] = append(queriesByTimeWindow[timeWindow], query)
		}
	}

	return queriesByTimeWindow
}
