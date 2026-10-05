// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

// Package recorderimpl wraps observer handles to record forwarded observations.
package recorderimpl

import (
	"slices"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

type recorderImpl struct {
	metrics recorder.MetricWriter
	logs    recorder.LogWriter
}

// NewComponent wraps the supplied writers with recorder middleware.
// The caller owns their lifecycle.
func NewComponent(metrics recorder.MetricWriter, logs recorder.LogWriter) recorder.Component {
	return &recorderImpl{metrics: metrics, logs: logs}
}

func (r *recorderImpl) GetHandle(inner observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		return &recordingHandle{inner: inner(name), name: name, recorder: r}
	}
}

type metricDropObserver interface {
	ObserveMetricAndReportDrop(observer.MetricView, uint64) bool
}

type recordingHandle struct {
	inner    observer.Handle
	name     string
	recorder *recorderImpl
}

func (h *recordingHandle) ObserveMetric(sample observer.MetricView, contextKey uint64) {
	dropped := false
	if reporting, ok := h.inner.(metricDropObserver); ok {
		dropped = reporting.ObserveMetricAndReportDrop(sample, contextKey)
	} else {
		h.inner.ObserveMetric(sample, contextKey)
	}

	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	tagsView := sample.GetTags()
	tags := make([]string, 0, tagsView.Len()+1)
	tagsView.ForEach(func(tag string) { tags = append(tags, tag) })
	if host := sample.GetHost(); host != "" && !slices.Contains(tags, "host:"+host) {
		tags = append(tags, "host:"+host)
	}
	metricType := sample.GetMetricType().String()
	if metricType == "" {
		metricType = "Unknown"
	}
	h.recorder.metrics.WriteMetric(recorder.MetricData{
		Source: h.name, Name: sample.GetName(), MetricType: metricType, Value: sample.GetValue(),
		Timestamp: timestamp, Tags: tags, Dropped: dropped,
	})
}

func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)
	timestampMs := msg.GetTimestampUnixMilli()
	if timestampMs == 0 {
		timestampMs = time.Now().UnixMilli()
	}
	h.recorder.logs.WriteLog(recorder.LogData{
		Source: h.name, TimestampMs: timestampMs,
		Content: []byte(msg.GetContent()), Status: msg.GetStatus(),
		Hostname: msg.GetHostname(), Tags: slices.Clone(msg.Tags()),
	})
}
