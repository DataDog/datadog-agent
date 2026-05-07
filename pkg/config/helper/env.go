// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Those are legacy env parsers that don't use the default format for env vars.
// No new usage of those should be added to the code base

func parseNameAndRate(token string) (string, float64, error) {
	parts := strings.Split(token, "=")
	if len(parts) != 2 {
		return "", 0, errors.New("Bad format")
	}
	rate, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", 0, errors.New("Unabled to parse rate")
	}
	return parts[0], rate, nil
}

// parseAnalyzedSpans parses the env string to extract a map of spans to be analyzed by service and operation.
// the format is: service_name|operation_name=rate,other_service|other_operation=rate
func parseAnalyzedSpans(env string) (map[string]interface{}, error) {
	analyzedSpans := make(map[string]interface{})
	if env == "" {
		return analyzedSpans, nil
	}
	tokens := strings.SplitSeq(env, ",")
	for token := range tokens {
		name, rate, err := parseNameAndRate(token)
		if err != nil {
			return nil, err
		}
		analyzedSpans[name] = rate
	}
	return analyzedSpans, nil
}

// ParseEnvTraceSpan is a custom helper for the 'apm_config.analyzed_spans' setting.
//
// This implements the env_parser 'traces_span' from the configuration schema
func ParseEnvTraceSpan(key string, config model.Setup) {
	config.ParseEnvAsMapStringInterface(key, func(in string) map[string]interface{} {
		out, err := parseAnalyzedSpans(in)
		if err != nil {
			log.Errorf(`Bad format for "%s" it should be of the form \"service_name|operation_name=rate,other_service|other_operation=rate\", error: %v`, key, err)
		}
		return out
	})
}

// ParseEnvCSVSplit is a custom helper for the 'apm_config.ignore_resources' setting.
//
// This implements the env_parser 'csv_coma_separated' from the from the configuration schema.
func ParseEnvCSVSplit(key string, config model.Setup) {
	config.ParseEnvAsStringSlice(key, func(in string) []string {
		r := csv.NewReader(strings.NewReader(in))
		r.TrimLeadingSpace = true
		r.LazyQuotes = true
		r.Comma = ','

		res, err := r.Read()
		if err != nil {
			log.Warnf(`"%s" can not be parsed: %v`, key, err)
			return []string{}
		}
		return res
	})
}

// ParseEnvSplitCommaAndSpace is a custom helper for the 'otelcollector.converter.features' setting.
//
// This implements the env_parser 'comma_and_space_separated'
func ParseEnvSplitCommaAndSpace(key string, config model.Setup) {
	config.ParseEnvAsStringSlice(key, func(s string) []string {
		// Support both comma and space separators
		return strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == ' '
		})
	})
}

// ParseEnvSplitCommaThenSpace is a custom helper for the 'apm_config.features' setting.
//
// This implements the env_parser 'comma_then_space_separated'
func ParseEnvSplitCommaThenSpace(key string, config model.Setup) {
	config.ParseEnvAsStringSlice(key, func(s string) []string {
		// Either commas or spaces can be used as separators.
		// Comma takes precedence as it was the only supported separator in the past.
		// Mixing separators is not supported.
		var res []string
		if strings.ContainsRune(s, ',') {
			res = strings.Split(s, ",")
		} else {
			res = strings.Split(s, " ")
		}
		for i, v := range res {
			res[i] = strings.TrimSpace(v)
		}
		return res
	})
}

func jsonOrSplitBy(key string, config model.Setup, sep string) {
	config.ParseEnvAsStringSlice(key, func(val string) []string {
		val = strings.TrimSpace(val)
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			res := []string{}
			if err := json.Unmarshal([]byte(val), &res); err != nil {
				log.Errorf("Error parsing JSON value for '%s' from env vars: %s", key, err)
				return nil
			}
			return res
		}

		return strings.Split(val, sep)
	})
}

// ParseEnvJSONOrComma is a custom helper for the following settings:
//
// - private_action_runner.restricted_shell.allowed_commands
// - private_action_runner.restricted_shell.allowed_paths
// - process_config.custom_sensitive_words
//
// This implements the env_parser 'json_list_or_comma_separated'
func ParseEnvJSONOrComma(key string, config model.Setup) {
	jsonOrSplitBy(key, config, ",")
}

// ParseEnvJSONOrSpace is a custom helper for the following settings:
//
// - apm_config.filter_tags.require
// - apm_config.filter_tags.reject
// - apm_config.filter_tags_regex.require
// - apm_config.filter_tags_regex.reject
// - apm_config.obfuscation.credit_cards.keep_values
//
// This implements the env_parser ”json_list_or_space_separated”
func ParseEnvJSONOrSpace(key string, config model.Setup) {
	jsonOrSplitBy(key, config, " ")
}
