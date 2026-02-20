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

// newPipelineHandler is the single constructor for all pipeline-based line handlers.
// The caller picks the Combiner that determines the combining strategy; everything else
// (tokenization, JSON aggregation, sampling) is wired identically.
func newPipelineHandler(combiner preprocessor.Combiner, tok *preprocessor.Tokenizer, sampler preprocessor.Sampler, jsonAggregator *preprocessor.JSONAggregator, enableJSONAggregation bool, flushTimeout time.Duration) LineHandler {
	pipeline := preprocessor.NewPipeline(combiner, tok, sampler, jsonAggregator, enableJSONAggregation, flushTimeout)
	return &pipelineLineHandler{pipeline: pipeline}
}

// pipelineLineHandler is a thin adapter that satisfies the LineHandler interface
// by delegating all processing to a preprocessor.Pipeline.
type pipelineLineHandler struct {
	pipeline *preprocessor.Pipeline
}

func (h *pipelineLineHandler) process(msg *message.Message) {
	h.pipeline.Process(msg)
}

func (h *pipelineLineHandler) flushChan() <-chan time.Time {
	return h.pipeline.FlushChan()
}

func (h *pipelineLineHandler) flush() {
	h.pipeline.Flush()
}

// buildAutoMultilineLabeler constructs a Labeler configured from global settings and any
// per-source overrides. It is shared by both aggregating and detecting pipeline modes.
func buildAutoMultilineLabeler(sourceSettings *config.SourceAutoMultiLineOptions, sourceSamples []*config.AutoMultilineSample, tailerInfo *status.InfoRegistry) *preprocessor.Labeler {
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
