// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"sync"
	"time"

	kutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"

	"k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

// DataSource provides cached access to kubelet API data. It ensures that
// /stats/summary and /metrics/cadvisor are each fetched at most once per
// check run, shared between the generic metrics collector and the kubelet
// check providers.
type DataSource struct {
	mu sync.Mutex

	kubeletClient kutil.KubeUtilInterface
	timeout       time.Duration

	statsSummary    *v1alpha1.Summary
	statsSummaryErr error

	cadvisorRaw     []byte
	cadvisorStatus  int
	cadvisorErr     error
	cadvisorMetrics []prometheus.MetricFamily

	refreshed bool
}

// NewDataSource creates a new DataSource using the given kubelet client.
func NewDataSource(client kutil.KubeUtilInterface, timeout time.Duration) *DataSource {
	return &DataSource{
		kubeletClient: client,
		timeout:       timeout,
	}
}

// Reset clears cached data. Should be called at the start of each check run.
func (ds *DataSource) Reset() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.statsSummary = nil
	ds.statsSummaryErr = nil
	ds.cadvisorRaw = nil
	ds.cadvisorStatus = 0
	ds.cadvisorErr = nil
	ds.cadvisorMetrics = nil
	ds.refreshed = false
}

// GetStatsSummary returns the cached /stats/summary response, fetching it if
// not yet cached in this check run.
func (ds *DataSource) GetStatsSummary() (*v1alpha1.Summary, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.statsSummary != nil || ds.statsSummaryErr != nil {
		return ds.statsSummary, ds.statsSummaryErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), ds.timeout)
	ds.statsSummary, ds.statsSummaryErr = ds.kubeletClient.GetLocalStatsSummary(ctx)
	cancel()

	if ds.statsSummaryErr != nil {
		log.Debugf("Unable to get stats summary from kubelet: %v", ds.statsSummaryErr)
	}

	return ds.statsSummary, ds.statsSummaryErr
}

// QueryCadvisor returns the cached /metrics/cadvisor raw response, fetching it if
// not yet cached in this check run.
func (ds *DataSource) QueryCadvisor() ([]byte, int, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.cadvisorRaw != nil || ds.cadvisorErr != nil {
		return ds.cadvisorRaw, ds.cadvisorStatus, ds.cadvisorErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), ds.timeout)
	ds.cadvisorRaw, ds.cadvisorStatus, ds.cadvisorErr = ds.kubeletClient.QueryKubelet(ctx, "/metrics/cadvisor")
	cancel()

	if ds.cadvisorErr != nil {
		log.Debugf("Unable to query cadvisor metrics from kubelet: %v", ds.cadvisorErr)
	}

	return ds.cadvisorRaw, ds.cadvisorStatus, ds.cadvisorErr
}

// GetCadvisorMetrics returns the parsed cadvisor prometheus metrics, fetching
// and parsing them if not yet cached in this check run.
func (ds *DataSource) GetCadvisorMetrics(textFilterBlacklist []string) ([]prometheus.MetricFamily, error) {
	// Ensure raw data is fetched first
	ds.QueryCadvisor()

	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.cadvisorMetrics != nil {
		return ds.cadvisorMetrics, nil
	}

	if ds.cadvisorErr != nil {
		return nil, ds.cadvisorErr
	}

	var err error
	ds.cadvisorMetrics, err = prometheus.ParseMetricsWithFilter(ds.cadvisorRaw, textFilterBlacklist)
	return ds.cadvisorMetrics, err
}
