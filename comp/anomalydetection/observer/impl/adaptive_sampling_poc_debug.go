// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"sync/atomic"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	adaptiveSamplingPOCDetectorScanLogCount         atomic.Uint64
	adaptiveSamplingPOCAnomalyCreatedLogCount       atomic.Uint64
	adaptiveSamplingPOCLogPatternAnomalyLogCount    atomic.Uint64
	adaptiveSamplingPOCBoostableCandidateLogCount   atomic.Uint64
	adaptiveSamplingPOCNonBoostableDecisionLogCount atomic.Uint64
)

func logDetectorScanForSamplingPOC(detectorName string, upTo int64, anomalies []observerdef.Anomaly) {
	count := adaptiveSamplingPOCDetectorScanLogCount.Add(1)
	if len(anomalies) == 0 && !shouldLogSamplingPOCDebug(count, 20, 60) {
		return
	}
	logPatternAnomalies := 0
	for _, anomaly := range anomalies {
		if isLogPatternCountAnomalySource(anomaly.Source.Namespace, anomaly.Source.Name) {
			logPatternAnomalies++
		}
	}
	pkglog.Infof("%s observer detector scan count=%d detector=%q up_to=%d anomaly_count=%d log_pattern_anomaly_count=%d",
		adaptivesampling.DebugLogPrefix,
		count,
		detectorName,
		upTo,
		len(anomalies),
		logPatternAnomalies)
}

func logAnomalyCreatedForSamplingPOC(anomaly observerdef.Anomaly) {
	logPattern := isLogPatternCountAnomalySource(anomaly.Source.Namespace, anomaly.Source.Name)
	var count uint64
	if logPattern {
		count = adaptiveSamplingPOCLogPatternAnomalyLogCount.Add(1)
		if !shouldLogSamplingPOCDebug(count, 50, 100) {
			return
		}
	} else {
		count = adaptiveSamplingPOCAnomalyCreatedLogCount.Add(1)
		if !shouldLogSamplingPOCDebug(count, 10, 1000) {
			return
		}
	}

	var contextSource, containerID, patternHash, pattern, example string
	if anomaly.Context != nil {
		contextSource = anomaly.Context.Source
		containerID = anomaly.Context.ContainerID
		patternHash = anomaly.Context.PatternHash
		pattern = anomaly.Context.Pattern
		example = anomaly.Context.Example
	}

	pkglog.Infof("%s observer anomaly created count=%d log_pattern=%t namespace=%q metric=%q aggregate=%q detector=%q timestamp=%d score=%s context_source=%q container_id=%q pattern_hash=%q pattern=%q example=%q title=%q",
		adaptivesampling.DebugLogPrefix,
		count,
		logPattern,
		anomaly.Source.Namespace,
		anomaly.Source.Name,
		observerdef.AggregateString(anomaly.Source.Aggregate),
		anomaly.DetectorName,
		anomaly.Timestamp,
		scoreForDebug(anomaly.Score),
		contextSource,
		containerID,
		patternHash,
		adaptivesampling.TruncateDebugString(pattern, 180),
		adaptivesampling.TruncateDebugString(example, 180),
		adaptivesampling.TruncateDebugString(anomaly.Title, 180))
}

func shouldLogSamplingPOCDebug(count, first, every uint64) bool {
	return count <= first || (every > 0 && count%every == 0)
}
