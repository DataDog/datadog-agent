// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// AutoMultilineHandler aggregates multiline logs.
type AutoMultilineHandler struct {
	labeler               *automultilinedetection.Labeler
	aggregator            *automultilinedetection.Aggregator
	jsonAggregator        *automultilinedetection.JSONAggregator
	flushTimeout          time.Duration
	flushTimer            *time.Timer
	enableJSONAggregation bool
}

// NewAutoMultilineHandler creates a new auto multiline handler.
func NewAutoMultilineHandler(outputFn func(m *message.Message), maxContentSize int, flushTimeout time.Duration, tailerInfo *status.InfoRegistry) *AutoMultilineHandler {

	// Order is important
	heuristics := []automultilinedetection.Heuristic{}

	heuristics = append(heuristics, automultilinedetection.NewTokenizer(pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes")))
	heuristics = append(heuristics, automultilinedetection.NewUserSamples(pkgconfigsetup.Datadog()))

	enableJSONAggregation := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_aggregation")

	if pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_detection") {
		heuristics = append(heuristics, automultilinedetection.NewJSONDetector())
	}

	if pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_datetime_detection") {
		heuristics = append(heuristics, automultilinedetection.NewTimestampDetector(
			pkgconfigsetup.Datadog().GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold")))
	}

	analyticsHeuristics := []automultilinedetection.Heuristic{automultilinedetection.NewPatternTable(
		pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.pattern_table_max_size"),
		pkgconfigsetup.Datadog().GetFloat64("logs_config.auto_multi_line.pattern_table_match_threshold"),
		tailerInfo),
	}

	return &AutoMultilineHandler{
		labeler: automultilinedetection.NewLabeler(heuristics, analyticsHeuristics),
		aggregator: automultilinedetection.NewAggregator(
			outputFn,
			maxContentSize,
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs"),
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs"),
			tailerInfo),
		jsonAggregator:        automultilinedetection.NewJSONAggregator(pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.tag_aggregated_json"), maxContentSize),
		flushTimeout:          flushTimeout,
		enableJSONAggregation: enableJSONAggregation,
	}
}

func (a *AutoMultilineHandler) process(msg *message.Message) {
	a.stopFlushTimerIfNeeded()
	defer a.startFlushTimerIfNeeded()

	if a.enableJSONAggregation {
		msgs := a.jsonAggregator.Process(msg)
		for _, m := range msgs {
			label := a.labeler.Label(m.GetContent())
			a.aggregator.Aggregate(m, label)
		}
	} else {
		label := a.labeler.Label(msg.GetContent())
		a.aggregator.Aggregate(msg, label)
	}
}

func (a *AutoMultilineHandler) flushChan() <-chan time.Time {
	if a.flushTimer != nil {
		return a.flushTimer.C
	}
	return nil
}

func (a *AutoMultilineHandler) isEmpty() bool {
	return a.aggregator.IsEmpty() && a.jsonAggregator.IsEmpty()
}

func (a *AutoMultilineHandler) flush() {
	if a.enableJSONAggregation {
		msgs := a.jsonAggregator.Flush()
		for _, m := range msgs {
			label := a.labeler.Label(m.GetContent())
			a.aggregator.Aggregate(m, label)
		}
	}
	a.aggregator.Flush()
	a.stopFlushTimerIfNeeded()
}

func (a *AutoMultilineHandler) stopFlushTimerIfNeeded() {
	if a.flushTimer == nil || a.isEmpty() {
		return
	}
	// stop the flush timer, as we now have data
	if !a.flushTimer.Stop() {
		<-a.flushTimer.C
	}
}

func (a *AutoMultilineHandler) startFlushTimerIfNeeded() {
	if a.isEmpty() {
		return
	}
	// since there's buffered data, start the flush timer to flush it
	if a.flushTimer == nil {
		a.flushTimer = time.NewTimer(a.flushTimeout)
	} else {
		a.flushTimer.Reset(a.flushTimeout)
	}
}
