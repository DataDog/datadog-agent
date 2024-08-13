// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultMatchThreshold = 0.75

// UserSample represents a user-defined sample for auto multi-line detection.
type UserSample struct {
	// Sample is a raw log message sample used to aggregate logs.
	Sample string `mapstructure:"sample"`
	// MatchThreshold is the ratio of tokens that must match between the sample and the log message to consider it a match.
	// From a user perspective, this is how similar the log has to be to the sample to be considered a match.
	// Optional - Default value is 0.75.
	MatchThreshold *float64 `mapstructure:"match_threshold"`
	// Label is the label to apply to the log message if it matches the sample.
	// Optional - Default value is "start_group".
	Label *string `mapstructure:"label,omitempty"`

	// Parse fields
	tokens         []Token
	matchThreshold float64
	label          Label
}

// UserSamples is a heuristic that represents a collection of user-defined samples for auto multi-line aggreagtion.
type UserSamples struct {
	samples []*UserSample
}

// NewUserSamples creates a new UserSamples instance.
func NewUserSamples(config config.Reader) *UserSamples {
	tokenizer := NewTokenizer(0)
	s := make([]*UserSample, 0)
	err := config.UnmarshalKey("logs_config.auto_multi_line_detection_custom_samples", &s)

	if err != nil {
		log.Error("Failed to unmarshal custom samples: ", err)
		return &UserSamples{
			samples: []*UserSample{},
		}
	}

	parsedSamples := make([]*UserSample, 0, len(s))
	for _, sample := range s {
		sample.tokens = tokenizer.tokenize([]byte(sample.Sample))
		if sample.MatchThreshold != nil {
			if *sample.MatchThreshold <= 0 || *sample.MatchThreshold > 1 {
				log.Warnf("Invalid match threshold %f, skipping sample", *sample.MatchThreshold)
				continue
			}
			sample.matchThreshold = *sample.MatchThreshold
		} else {
			sample.matchThreshold = defaultMatchThreshold
		}

		if sample.Label != nil {
			switch *sample.Label {
			case "start_group":
				sample.label = startGroup
			case "no_aggregate":
				sample.label = noAggregate
			case "aggregate":
				sample.label = aggregate
			default:
				log.Warnf("Unknown label %s, skipping sample", *sample.Label)
				continue
			}
		} else {
			sample.label = startGroup
		}

		parsedSamples = append(parsedSamples, sample)
	}

	return &UserSamples{
		samples: parsedSamples,
	}
}

// Process applies a user sample to a log message. If it matches, a label is assigned.
func (j *UserSamples) Process(context *messageContext) bool {
	if context.tokens == nil {
		log.Error("Tokens are required to process user samples")
		return true
	}

	for _, sample := range j.samples {
		if isMatch(sample.tokens, context.tokens, sample.matchThreshold) {
			context.label = sample.label
			return true
		}
	}
	return false
}
