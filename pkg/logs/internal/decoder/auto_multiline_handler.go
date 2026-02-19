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

// autoMultilineHandler is a thin adapter satisfying the LineHandler interface.
// It delegates all processing to a Pipeline that runs stages in the correct order:
// JSON aggregation → tokenization → labeling → aggregation.
type autoMultilineHandler struct {
	pipeline *preprocessor.Pipeline
}

// NewAutoMultilineHandler creates a new auto multiline handler.
// The aggregator parameter determines whether logs are combined (combiningAggregator) or just tagged (detectingAggregator).
// enableJSONAggregation controls whether split JSON objects should be combined before processing.
func NewAutoMultilineHandler(aggregator preprocessor.Aggregator, tokenizer *preprocessor.Tokenizer, maxContentSize int, flushTimeout time.Duration, tailerInfo *status.InfoRegistry, sourceSettings *config.SourceAutoMultiLineOptions, sourceSamples []*config.AutoMultilineSample, enableJSONAggregation bool) LineHandler {
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
	analyticsHeuristics := []preprocessor.Heuristic{preprocessor.NewPatternTable(
		patternTableMaxSize,
		patternTableMatchThreshold,
		tailerInfo),
	}

	labeler := preprocessor.NewLabeler(heuristics, analyticsHeuristics)
	jsonAggregator := preprocessor.NewJSONAggregator(pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.tag_aggregated_json"), maxContentSize)
	pipeline := preprocessor.NewPipeline(aggregator, tokenizer, labeler, jsonAggregator, enableJSONAggregation, flushTimeout)

	return &autoMultilineHandler{pipeline: pipeline}
}

func (a *autoMultilineHandler) process(msg *message.Message) {
	a.pipeline.Process(msg)
}

func (a *autoMultilineHandler) flushChan() <-chan time.Time {
	return a.pipeline.FlushChan()
}

func (a *autoMultilineHandler) flush() {
	a.pipeline.Flush()
}
