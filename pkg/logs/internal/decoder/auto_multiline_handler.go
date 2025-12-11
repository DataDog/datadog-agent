// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package decoder provides log line decoding and parsing functionality
package decoder

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// AutoMultilineHandler aggregates multiline logs or tags them for detection-only mode.
type AutoMultilineHandler struct {
	labeler               *automultilinedetection.Labeler
	aggregator            *automultilinedetection.DefaultAggregator
	jsonAggregator        *automultilinedetection.JSONAggregator
	flushTimeout          time.Duration
	flushTimer            *time.Timer
	enableJSONAggregation bool
	detectionOnly         bool // if true, tag instead of aggregate
}

// NewAutoMultilineHandler creates a new auto multiline handler.
// If detectionOnly is true, logs are tagged with their label but not aggregated.
func NewAutoMultilineHandler(outputFn func(m []*message.Message), maxContentSize int, flushTimeout time.Duration, tailerInfo *status.InfoRegistry, sourceSettings *config.SourceAutoMultiLineOptions, sourceSamples []*config.AutoMultilineSample, detectionOnly bool) *AutoMultilineHandler {

	// Order is important
	heuristics := []automultilinedetection.Heuristic{}
	sourceHasSettings := sourceSettings != nil

	tokenizerMaxInputBytes := pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes")
	if sourceHasSettings && sourceSettings.TokenizerMaxInputBytes != nil {
		tokenizerMaxInputBytes = *sourceSettings.TokenizerMaxInputBytes
	}
	heuristics = append(heuristics, automultilinedetection.NewTokenizer(tokenizerMaxInputBytes))
	heuristics = append(heuristics, automultilinedetection.NewUserSamples(pkgconfigsetup.Datadog(), sourceSamples))

	enableJSONAggregation := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_aggregation")
	if sourceHasSettings && sourceSettings.EnableJSONAggregation != nil {
		enableJSONAggregation = *sourceSettings.EnableJSONAggregation
	}

	enableJSONDetection := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_detection")
	if sourceHasSettings && sourceSettings.EnableJSONDetection != nil {
		enableJSONDetection = *sourceSettings.EnableJSONDetection
	}
	if enableJSONDetection {
		heuristics = append(heuristics, automultilinedetection.NewJSONDetector())
	}

	enableDatetimeDetection := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_datetime_detection")
	if sourceHasSettings && sourceSettings.EnableDatetimeDetection != nil {
		enableDatetimeDetection = *sourceSettings.EnableDatetimeDetection
	}
	if enableDatetimeDetection {
		timestampDetectorMatchThreshold := pkgconfigsetup.Datadog().GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold")
		if sourceHasSettings && sourceSettings.TimestampDetectorMatchThreshold != nil {
			timestampDetectorMatchThreshold = *sourceSettings.TimestampDetectorMatchThreshold
		}
		heuristics = append(heuristics, automultilinedetection.NewTimestampDetector(timestampDetectorMatchThreshold))
	}

	patternTableMaxSize := pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.pattern_table_max_size")
	if sourceHasSettings && sourceSettings.PatternTableMaxSize != nil {
		patternTableMaxSize = *sourceSettings.PatternTableMaxSize
	}
	patternTableMatchThreshold := pkgconfigsetup.Datadog().GetFloat64("logs_config.auto_multi_line.pattern_table_match_threshold")
	if sourceHasSettings && sourceSettings.PatternTableMatchThreshold != nil {
		patternTableMatchThreshold = *sourceSettings.PatternTableMatchThreshold
	}
	analyticsHeuristics := []automultilinedetection.Heuristic{automultilinedetection.NewPatternTable(
		patternTableMaxSize,
		patternTableMatchThreshold,
		tailerInfo),
	}

	handler := &AutoMultilineHandler{
		labeler:               automultilinedetection.NewLabeler(heuristics, analyticsHeuristics),
		jsonAggregator:        automultilinedetection.NewJSONAggregator(pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.tag_aggregated_json"), maxContentSize),
		flushTimeout:          flushTimeout,
		enableJSONAggregation: enableJSONAggregation,
		detectionOnly:         detectionOnly,
	}

	// Only create aggregator if not in detection-only mode
	if !detectionOnly {
		handler.aggregator = automultilinedetection.NewAggregator(
			outputFn,
			maxContentSize,
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs"),
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs"),
			tailerInfo)
	}

	return handler
}

func (a *AutoMultilineHandler) process(msg *message.Message) {
	if !a.detectionOnly {
		a.stopFlushTimerIfNeeded()
		defer a.startFlushTimerIfNeeded()
	}

	if a.enableJSONAggregation {
		msgs := a.jsonAggregator.Process(msg)
		for _, m := range msgs {
			label := a.labeler.Label(m.GetContent())
			if a.detectionOnly {
				a.tagWithLabel(m, label)
				a.outputFn(m)
			} else {
				a.aggregator.Aggregate(m, label)
			}
		}
	} else {
		label := a.labeler.Label(msg.GetContent())
		if a.detectionOnly {
			a.tagWithLabel(msg, label)
			a.outputFn(msg)
		} else {
			a.aggregator.Aggregate(msg, label)
		}
	}
}

func (a *AutoMultilineHandler) tagWithLabel(msg *message.Message, label automultilinedetection.Label) {
	labelStr := automultilinedetection.LabelToString(label)
	msg.ProcessingTags = append(msg.ProcessingTags, "auto_multiline_label:"+labelStr)
}

func (a *AutoMultilineHandler) flushChan() <-chan time.Time {
	if a.detectionOnly {
		return nil // No buffering in detection-only mode
	}
	if a.flushTimer != nil {
		return a.flushTimer.C
	}
	return nil
}

func (a *AutoMultilineHandler) isEmpty() bool {
	if a.detectionOnly {
		return a.jsonAggregator.IsEmpty() // Only JSON aggregator buffer in detection-only mode
	}
	return a.aggregator.IsEmpty() && a.jsonAggregator.IsEmpty()
}

func (a *AutoMultilineHandler) flush() {
	if a.enableJSONAggregation {
		msgs := a.jsonAggregator.Flush()
		for _, m := range msgs {
			label := a.labeler.Label(m.GetContent())
			if a.detectionOnly {
				a.tagWithLabel(m, label)
				a.outputFn(m)
			} else {
				a.aggregator.Aggregate(m, label)
			}
		}
	}
	if !a.detectionOnly {
		a.aggregator.Flush()
		a.stopFlushTimerIfNeeded()
	}
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
