// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	invalidMetricBackendErrorMessage  string = "Invalid metric (from backend), query: %s"
	invalidMetricOutdatedErrorMessage string = "Outdated result from backend, query: %s"
	invalidMetricNoDataErrorMessage   string = "No data from backend, query: %s"
	invalidMetricGlobalErrorMessage   string = "Global error (all queries) from backend"
	metricRetrieverStoreID            string = "mr"
)

type MetricsRetriever struct {
	refreshPeriod int64
	metricsMaxAge int64
	processor     autoscalers.ProcessorInterface
	store         *DatadogMetricsInternalStore
	isLeader      func() bool
}

func NewMetricsRetriever(refreshPeriod, metricsMaxAge int64, processor autoscalers.ProcessorInterface, isLeader func() bool, store *DatadogMetricsInternalStore) (*MetricsRetriever, error) {
	return &MetricsRetriever{
		refreshPeriod: refreshPeriod,
		metricsMaxAge: metricsMaxAge,
		processor:     processor,
		store:         store,
		isLeader:      isLeader,
	}, nil
}

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

	queries := getUniqueQueries(datadogMetrics)
	log.Debugf("Starting refreshing external metrics with: %d queries", len(queries))

	results, err := mr.processor.QueryExternalMetric(queries)
	globalError := false
	// Check for global failure
	if len(results) == 0 && err != nil {
		globalError = true
		log.Errorf("Unable to fetch external metrics: %v", err)
	}

	// Update store with current results
	currentTime := time.Now().UTC()
	for _, datadogMetric := range datadogMetrics {
		datadogMetricFromStore := mr.store.LockRead(datadogMetric.ID, false)
		if datadogMetricFromStore == nil {
			// This metric is not in the store anymore, discard it
			log.Infof("Discarding results for DatadogMetric: %s as not present in store anymore", datadogMetric.ID)
			continue
		}

		if queryResult, found := results[datadogMetric.Query]; found {
			log.Debug("QueryResult from DD: %v", queryResult)

			if queryResult.Valid {
				datadogMetricFromStore.Value = queryResult.Value

				// If we get a valid but old metric, flag it as invalid
				if currentTime.Unix()-queryResult.Timestamp <= mr.metricsMaxAge {
					datadogMetricFromStore.Valid = true
					datadogMetricFromStore.Error = nil
					datadogMetricFromStore.UpdateTime = time.Unix(queryResult.Timestamp, 0).UTC()
				} else {
					datadogMetricFromStore.Valid = false
					datadogMetricFromStore.Error = fmt.Errorf(invalidMetricOutdatedErrorMessage, datadogMetric.Query)
					datadogMetricFromStore.UpdateTime = currentTime
				}
			} else {
				datadogMetricFromStore.Valid = false
				datadogMetricFromStore.Error = fmt.Errorf(invalidMetricBackendErrorMessage, datadogMetric.Query)
				datadogMetricFromStore.UpdateTime = currentTime
			}
		} else {
			datadogMetricFromStore.Valid = false
			if globalError {
				datadogMetricFromStore.Error = fmt.Errorf(invalidMetricGlobalErrorMessage)
			} else {
				datadogMetricFromStore.Error = fmt.Errorf(invalidMetricNoDataErrorMessage, datadogMetric.Query)
			}
			datadogMetricFromStore.UpdateTime = currentTime
		}

		mr.store.UnlockSet(datadogMetric.ID, *datadogMetricFromStore, metricRetrieverStoreID)
	}
}

func getUniqueQueries(datadogMetrics []model.DatadogMetricInternal) []string {
	queries := make([]string, 0, len(datadogMetrics))
	unique := make(map[string]struct{}, len(queries))
	for _, datadogMetric := range datadogMetrics {
		if _, found := unique[datadogMetric.Query]; !found {
			unique[datadogMetric.Query] = struct{}{}
			queries = append(queries, datadogMetric.Query)
		}
	}

	return queries
}
