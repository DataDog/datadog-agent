// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"encoding/json"
	"regexp"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultMatchThreshold = 0.75

// UserSample represents a user-defined sample for auto multi-line detection.
type UserSample struct {
	config.AutoMultilineSample

	// Parse fields
	tokens         []tokens.Token
	matchThreshold float64
	label          Label
	compiledRegex  *regexp.Regexp
}

// UserSamples is a heuristic that represents a collection of user-defined samples for auto multi-line aggreagtion.
type UserSamples struct {
	samples []*UserSample
}

// NewUserSamples creates a new UserSamples instance.
func NewUserSamples(cfgRdr model.Reader, sourceSamples []*config.AutoMultilineSample) *UserSamples {
	tokenizer := NewTokenizer(0)
	s := make([]*UserSample, 0)
	var err error

	if sourceSamples != nil {
		for _, sample := range sourceSamples {
			log.Debugf("Adding source user sample: %+v", sample)
			s = append(s, &UserSample{
				AutoMultilineSample: *sample,
			})
		}
	} else {
		rawMainSamples := cfgRdr.Get("logs_config.auto_multi_line_detection_custom_samples")
		if rawMainSamples != nil {
			if str, ok := rawMainSamples.(string); ok && str != "" {
				err = json.Unmarshal([]byte(str), &s)
			} else {
				var rawUserSamples []config.AutoMultilineSample
				err = structure.UnmarshalKey(cfgRdr, "logs_config.auto_multi_line_detection_custom_samples", &rawUserSamples)
				for _, rawSample := range rawUserSamples {
					s = append(s, &UserSample{
						AutoMultilineSample: rawSample,
					})
				}
			}

			if err != nil {
				log.Error("Failed to unmarshal main config custom samples: ", err)
				s = make([]*UserSample, 0)
			}
		}

		legacyAdditionalPatterns := cfgRdr.GetStringSlice("logs_config.auto_multi_line_extra_patterns")
		if len(legacyAdditionalPatterns) > 0 {
			log.Warn("Found deprecated logs_config.auto_multi_line_extra_patterns converting to logs_config.auto_multi_line_detection_custom_samples")
			for _, pattern := range legacyAdditionalPatterns {
				s = append(s, &UserSample{
					AutoMultilineSample: config.AutoMultilineSample{
						Regex: pattern,
					},
				})
			}
		}
	}

	parsedSamples := make([]*UserSample, 0, len(s))
	for _, sample := range s {
		if sample.Sample != "" {
			sample.tokens, _ = tokenizer.tokenize([]byte(sample.Sample))

			if sample.MatchThreshold != nil {
				if *sample.MatchThreshold <= 0 || *sample.MatchThreshold > 1 {
					log.Warnf("Invalid match threshold %f, skipping sample", *sample.MatchThreshold)
					continue
				}
				sample.matchThreshold = *sample.MatchThreshold
			} else {
				sample.matchThreshold = defaultMatchThreshold
			}
		} else if sample.Regex != "" {
			compiled, err := regexp.Compile("^" + sample.Regex)
			if err != nil {
				log.Warn(sample.Regex, " is not a valid regular expression - skipping")
				continue
			}
			sample.compiledRegex = compiled
		} else {
			log.Warn("Sample and regex was empty, skipping")
			continue
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

// ProcessAndContinue applies a user sample to a log message. If it matches, a label is assigned.
// This implements the Heuristic interface - so we should stop processing if we detect a user pattern by returning false.
func (j *UserSamples) ProcessAndContinue(context *messageContext) bool {
	if context.tokens == nil {
		log.Error("Tokens are required to process user samples")
		return true
	}

	for _, sample := range j.samples {
		if sample.compiledRegex != nil {
			if sample.compiledRegex.Match(context.rawMessage) {
				context.label = sample.label
				context.labelAssignedBy = "user_sample"
				return false
			}
		} else if isMatch(sample.tokens, context.tokens, sample.matchThreshold) {
			context.label = sample.label
			context.labelAssignedBy = "user_sample"
			return false
		}
	}
	return true
}
