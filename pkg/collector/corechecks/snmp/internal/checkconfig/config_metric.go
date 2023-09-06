package checkconfig

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/cprofstruct"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"regexp"
)

// GetTags returns tags based on MetricTagConfig and a value
func GetTags(mtc *cprofstruct.MetricTagConfig, value string) []string {
	var tags []string
	if mtc.Tag != "" {
		if len(mtc.Mapping) > 0 {
			mappedValue, err := GetMappedValue(value, mtc.Mapping)
			if err != nil {
				log.Debugf("error getting tags. mapping for `%s` does not exist. mapping=`%v`", value, mtc.Mapping)
			} else {
				tags = append(tags, mtc.Tag+":"+mappedValue)
			}
		} else {
			tags = append(tags, mtc.Tag+":"+value)
		}
	} else if mtc.Match != "" {
		if mtc.Pattern == nil {
			log.Warnf("match Pattern must be present: match=%s", mtc.Match)
			return tags
		}
		if mtc.Pattern.MatchString(value) {
			for key, val := range mtc.Tags {
				normalizedTemplate := normalizeRegexReplaceValue(val)
				replacedVal := RegexReplaceValue(value, mtc.Pattern, normalizedTemplate)
				if replacedVal == "" {
					log.Debugf("Pattern `%v` failed to match `%v` with template `%v`", mtc.Pattern, value, normalizedTemplate)
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
	re := regexp.MustCompile("\\\\(\\d+)")
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
