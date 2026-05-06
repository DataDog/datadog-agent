// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observer

// TelemetryNamespace is the storage namespace used for observer-internal debug
// metrics (e.g. testbench UI charts). Detectors must not treat it as workload data.
const TelemetryNamespace = "telemetry"

// LogPatternExtractorNamespace is the canonical storage namespace for metrics
// emitted by the log pattern extractor. Used as SeriesDescriptor.Namespace and
// as the component name in the catalog.
const LogPatternExtractorNamespace = "log_pattern_extractor"

// LogMetricsExtractorNamespace is the canonical storage namespace for metrics
// emitted by the log metrics extractor. Used as SeriesDescriptor.Namespace and
// as the component name in the catalog.
const LogMetricsExtractorNamespace = "log_metrics_extractor"

// SplitTagKeyOrder is the canonical ordered list of tag dimensions used to split
// log clusters and to render split-tag summaries (e.g. in anomaly event messages).
// When adding dimensions, update TagGroupByKey and extractTagGroupByKey in
// comp/anomalydetection/observer/impl/log_tagged_pattern_clusterer.go.
var SplitTagKeyOrder = []string{"source", "service", "env", "host"}
