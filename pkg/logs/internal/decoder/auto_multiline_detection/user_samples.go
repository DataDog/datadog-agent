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
	configSamples := make([]config.AutoMultilineSample, 0)
	var err error

	if sourceSamples != nil {
		for _, sample := range sourceSamples {
			log.Debugf("Adding source user sample: %+v", sample)
			configSamples = append(configSamples, *sample)
		}
	} else {
		rawMainSamples := cfgRdr.Get("logs_config.auto_multi_line_detection_custom_samples")
		if rawMainSamples != nil {
			if str, ok := rawMainSamples.(string); ok && str != "" {
				err = json.Unmarshal([]byte(str), &configSamples)
			} else {
				err = structure.UnmarshalKey(cfgRdr, "logs_config.auto_multi_line_detection_custom_samples", &configSamples)
			}

			if err != nil {
				log.Error("Failed to unmarshal main config custom samples: ", err)
				configSamples = make([]config.AutoMultilineSample, 0)
			}
		}

		legacyAdditionalPatterns := cfgRdr.GetStringSlice("logs_config.auto_multi_line_extra_patterns")
		if len(legacyAdditionalPatterns) > 0 {
			log.Warn("Found deprecated logs_config.auto_multi_line_extra_patterns converting to logs_config.auto_multi_line_detection_custom_samples")
			for _, pattern := range legacyAdditionalPatterns {
				configSamples = append(configSamples, config.AutoMultilineSample{
					Regex: pattern,
				})
			}
		}
	}

	parsedSamples := make([]*UserSample, 0, len(configSamples))
	for _, configSample := range configSamples {
		parsedSample := &UserSample{}
		if configSample.Sample != "" {
			parsedSample.tokens, _ = tokenizer.tokenize([]byte(configSample.Sample))

			if configSample.MatchThreshold != nil {
				if *configSample.MatchThreshold <= 0 || *configSample.MatchThreshold > 1 {
					log.Warnf("Invalid match threshold %f, skipping sample", *configSample.MatchThreshold)
					continue
				}
				parsedSample.matchThreshold = *configSample.MatchThreshold
			} else {
				parsedSample.matchThreshold = defaultMatchThreshold
			}
		} else if configSample.Regex != "" {
			compiled, err := regexp.Compile("^" + configSample.Regex)
			if err != nil {
				log.Warn(configSample.Regex, " is not a valid regular expression - skipping")
				continue
			}
			parsedSample.compiledRegex = compiled
		} else {
			log.Warn("Sample and regex was empty, skipping")
			continue
		}

		if configSample.Label != nil {
			switch *configSample.Label {
			case "start_group":
				parsedSample.label = startGroup
			case "no_aggregate":
				parsedSample.label = noAggregate
			case "aggregate":
				parsedSample.label = aggregate
			default:
				log.Warnf("Unknown label %s, skipping sample", *configSample.Label)
				continue
			}
		} else {
			parsedSample.label = startGroup
		}

		parsedSamples = append(parsedSamples, parsedSample)
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
