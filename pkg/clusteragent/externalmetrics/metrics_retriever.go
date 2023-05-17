// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	invalidMetricErrorMessage         string = "%v, query was: %s"
	invalidMetricOutdatedErrorMessage string = "Query returned outdated result, check MaxAge setting, query: %s"
	invalidMetricNotFoundErrorMessage string = "Unexpected error, query data not found in result, query: %s"
	invalidMetricGlobalErrorMessage   string = "Global error (all queries) from backend, invalid syntax in query? Check Cluster Agent leader logs for details"
	metricRetrieverStoreID            string = "mr"
)

// MetricsRetriever is responsible for querying and storing external metrics
type MetricsRetriever struct {
	refreshPeriod int64
	metricsMaxAge int64
	processor     autoscalers.ProcessorInterface
	store         *DatadogMetricsInternalStore
	isLeader      func() bool
}

// NewMetricsRetriever returns a new MetricsRetriever
func NewMetricsRetriever(refreshPeriod, metricsMaxAge int64, processor autoscalers.ProcessorInterface, isLeader func() bool, store *DatadogMetricsInternalStore) (*MetricsRetriever, error) {
	return &MetricsRetriever{
		refreshPeriod: refreshPeriod,
		metricsMaxAge: metricsMaxAge,
		processor:     processor,
		store:         store,
		isLeader:      isLeader,
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
				mr.retrieveMetricsValues()
			}
		case <-stopCh:
			log.Infof("Stopping MetricsRetriever")
			return
		}
	}
}

func (mr *MetricsRetriever) retrieveMetricsValues() {
	// We only update active DatadogMetrics
	datadogMetrics := mr.store.GetFiltered(func(datadogMetric model.DatadogMetricInternal) bool { return datadogMetric.Active })
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
		timeWindow := MaybeAdjustTimeWindowForQuery(datadogMetric.GetTimeWindow())
		results := resultsByTimeWindow[timeWindow]

		if queryResult, found := results[query]; found {
			log.Debugf("QueryResult from DD for %q: %v", query, queryResult)

			if queryResult.Valid {
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
				datadogMetricFromStore.Error = fmt.Errorf(invalidMetricErrorMessage, queryResult.Error, query)
			}
		} else {
			datadogMetricFromStore.Valid = false
			if globalError {
				datadogMetricFromStore.Error = fmt.Errorf(invalidMetricGlobalErrorMessage)
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

func MaybeAdjustTimeWindowForQuery(timeWindow time.Duration) time.Duration {
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
		timeWindow := MaybeAdjustTimeWindowForQuery(datadogMetric.GetTimeWindow())

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
