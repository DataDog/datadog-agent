// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package checkconfig

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// BuildMetricTagsFromValue returns tags based on MetricTagConfig and a value
func BuildMetricTagsFromValue(metricTag *profiledefinition.MetricTagConfig, value string) []string {
	var tags []string
	if metricTag.Tag != "" {
		if len(metricTag.Mapping) > 0 {
			mappedValue, err := GetMappedValue(value, metricTag.Mapping)
			if err != nil {
				log.Debugf("error getting tags. mapping for `%s` does not exist. mapping=`%v`", value, metricTag.Mapping)
			} else {
				tags = append(tags, metricTag.Tag+":"+mappedValue)
			}
		} else {
			tags = append(tags, metricTag.Tag+":"+value)
		}
	} else if metricTag.Match != "" {
		if metricTag.Pattern == nil {
			log.Warnf("match Pattern must be present: match=%s", metricTag.Match)
			return tags
		}
		if metricTag.Pattern.MatchString(value) {
			for key, val := range metricTag.Tags {
				normalizedTemplate := normalizeRegexReplaceValue(val)
				replacedVal := RegexReplaceValue(value, metricTag.Pattern, normalizedTemplate)
				if replacedVal == "" {
					log.Debugf("Pattern `%v` failed to match `%v` with template `%v`", metricTag.Pattern, value, normalizedTemplate)
					continue
				}
				tags = append(tags, key+":"+replacedVal)
			}
		}
	}
	return tags
}

// RegexReplaceValue replaces a value using a regex and template
func RegexReplaceValue(value string, pattern *regexp.Regexp, normalizedTemplate string) string {
	result := []byte{}
	for _, submatches := range pattern.FindAllStringSubmatchIndex(value, 1) {
		result = pattern.ExpandString(result, normalizedTemplate, value, submatches)
	}
	return string(result)
}

// normalizeRegexReplaceValue normalize regex value to keep compatibility with Python
// Converts \1 into $1, \2 into $2, etc
func normalizeRegexReplaceValue(val string) string {
	re := regexp.MustCompile(`\\(\d+)`)
	return re.ReplaceAllString(val, "$$$1")
}

// GetMappedValue retrieves mapped value from a given mapping.
// If mapping is empty, it will return the index.
func GetMappedValue(index string, mapping map[string]string) (string, error) {
	if len(mapping) > 0 {
		mappedValue, ok := mapping[index]
		if !ok {
			return "", fmt.Errorf("mapping for `%s` does not exist. mapping=`%v`", index, mapping)
		}
		return mappedValue, nil
	}
	return index, nil
}
