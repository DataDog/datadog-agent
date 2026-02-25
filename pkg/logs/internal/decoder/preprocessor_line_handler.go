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
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// newPreprocessorHandler is the single constructor for all preprocessor-based line handlers.
// The caller picks the Aggregator that determines the combining strategy; everything else
// (tokenization, labeling, JSON aggregation, sampling) is wired identically.
// Pass nil for labeler on paths that don't use auto multiline detection (regex, pass-through).
func newPreprocessorHandler(aggregator preprocessor.Aggregator, tok *preprocessor.Tokenizer, labeler preprocessor.Labeler, sampler preprocessor.Sampler, outputChan chan *message.Message, jsonAggregator preprocessor.JSONAggregator, flushTimeout time.Duration) LineHandler {
	pp := preprocessor.NewPreprocessor(aggregator, tok, labeler, sampler, outputChan, jsonAggregator, flushTimeout)
	return &preprocessorLineHandler{preprocessor: pp}
}

// preprocessorLineHandler is a thin adapter that satisfies the LineHandler interface
// by delegating all processing to a preprocessor.Preprocessor.
type preprocessorLineHandler struct {
	preprocessor *preprocessor.Preprocessor
}

func (h *preprocessorLineHandler) process(msg *message.Message) {
	h.preprocessor.Process(msg)
}

func (h *preprocessorLineHandler) flushChan() <-chan time.Time {
	return h.preprocessor.FlushChan()
}

func (h *preprocessorLineHandler) flush() {
	h.preprocessor.Flush()
}

// buildAutoMultilineLabeler constructs a Labeler configured from global settings and any
// per-source overrides. It is shared by both aggregating and detecting preprocessor modes.
func buildAutoMultilineLabeler(sourceSettings *config.SourceAutoMultiLineOptions, sourceSamples []*config.AutoMultilineSample, tailerInfo *status.InfoRegistry) preprocessor.Labeler {
	heuristics := []preprocessor.Heuristic{}
	sourceHasSettings := sourceSettings != nil

	heuristics = append(heuristics, preprocessor.NewUserSamples(pkgconfigsetup.Datadog(), sourceSamples))

	enableJSONDetection := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_detection")
	if sourceHasSettings && sourceSettings.EnableJSONDetection != nil {
		enableJSONDetection = *sourceSettings.EnableJSONDetection
	}
	if enableJSONDetection {
		heuristics = append(heuristics, preprocessor.NewJSONDetector())
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
		heuristics = append(heuristics, preprocessor.NewTimestampDetector(timestampDetectorMatchThreshold))
	}

	patternTableMaxSize := pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.pattern_table_max_size")
	if sourceHasSettings && sourceSettings.PatternTableMaxSize != nil {
		patternTableMaxSize = *sourceSettings.PatternTableMaxSize
	}
	patternTableMatchThreshold := pkgconfigsetup.Datadog().GetFloat64("logs_config.auto_multi_line.pattern_table_match_threshold")
	if sourceHasSettings && sourceSettings.PatternTableMatchThreshold != nil {
		patternTableMatchThreshold = *sourceSettings.PatternTableMatchThreshold
	}
	analyticsHeuristics := []preprocessor.Heuristic{
		preprocessor.NewPatternTable(patternTableMaxSize, patternTableMatchThreshold, tailerInfo),
	}

	return preprocessor.NewLabeler(heuristics, analyticsHeuristics)
}
