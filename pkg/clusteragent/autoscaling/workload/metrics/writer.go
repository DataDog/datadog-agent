// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SenderMetricsWriter writes metrics from a store to a Datadog Sender
type SenderMetricsWriter struct {
	store    *PodAutoscalerMetricsStore
	sender   sender.Sender
	isLeader func() bool
}

// NewSenderMetricsWriter creates a new metrics writer
func NewSenderMetricsWriter(store *PodAutoscalerMetricsStore, senderInstance sender.Sender, isLeaderFunc func() bool) *SenderMetricsWriter {
	return &SenderMetricsWriter{
		store:    store,
		sender:   senderInstance,
		isLeader: isLeaderFunc,
	}
}

// WriteAll sends all metrics in the store via Sender
func (w *SenderMetricsWriter) WriteAll() error {
	// Only write metrics if this instance is the leader
	if !w.isLeader() {
		log.Tracef("Skipping metrics submission - not leader")
		return nil
	}

	w.store.metrics.Range(func(key, value interface{}) bool {
		metrics, ok := value.(StructuredMetrics)
		if !ok {
			log.Warnf("Invalid metrics type in store for key %v", key)
			return true
		}

		for _, metric := range metrics {
			switch metric.Type {
			case MetricTypeGauge:
				w.sender.Gauge(metric.Name, metric.Value, "", metric.Tags)
			case MetricTypeCount:
				w.sender.Count(metric.Name, metric.Value, "", metric.Tags)
			default:
				log.Warnf("Unknown metric type %v for metric %s", metric.Type, metric.Name)
			}
		}
		return true
	})

	w.sender.Commit()
	return nil
}

// WriteAllPeriodically runs WriteAll on a ticker at the specified interval
func (w *SenderMetricsWriter) WriteAllPeriodically(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Submit immediately on start
	if err := w.WriteAll(); err != nil {
		log.Errorf("Failed to write metrics: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping periodic metrics writer")
			return
		case <-ticker.C:
			if err := w.WriteAll(); err != nil {
				log.Errorf("Failed to write metrics: %v", err)
			}
		}
	}
}
