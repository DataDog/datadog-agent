package dogstatsd

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	allowedGlobMatchPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_*.]+$`)
)

const (
	matchTypeGlob  = "glob"
	matchTypeRegex = "regex"
)

type metricMapper struct {
	Mappings []metricMapping
	cache    *mapperCache
}

type metricMapping struct {
	Match     string            `mapstructure:"match"`
	MatchType string            `mapstructure:"match_type"`
	Name      string            `mapstructure:"name"`
	Tags      map[string]string `mapstructure:"tags"`
	regex     *regexp.Regexp
}

// newMetricMapper creates a new metricMapper
func newMetricMapper(configMappings []metricMapping, cacheSize int) (metricMapper, error) {
	var mappings []metricMapping
	for i := range configMappings {
		currentMapping := configMappings[i]
		if currentMapping.MatchType == "" {
			currentMapping.MatchType = matchTypeGlob
		}
		if currentMapping.MatchType != matchTypeGlob && currentMapping.MatchType != matchTypeRegex {
			return metricMapper{}, fmt.Errorf("mapping num %d: invalid match type, must be `glob` or `regex`", i)
		}
		if currentMapping.Name == "" {
			return metricMapper{}, fmt.Errorf("mapping num %d: name is required", i)
		}
		if currentMapping.Match == "" {
			return metricMapper{}, fmt.Errorf("mapping num %d: match is required", i)
		}
		err := currentMapping.prepare()
		if err != nil {
			return metricMapper{}, err
		}
		mappings = append(mappings, currentMapping)
	}
	cache, err := newMapperCache(cacheSize)
	if err != nil {
		return metricMapper{}, err
	}
	return metricMapper{Mappings: mappings, cache: cache}, nil
}

// prepare compiles the match patterns into regexes
func (m *metricMapping) prepare() error {
	metricRe := m.Match
	if m.MatchType == matchTypeGlob {
		if !allowedGlobMatchPattern.MatchString(m.Match) {
			return fmt.Errorf("invalid glob match pattern `%s`, it does not match allowed match regex `%s`", m.Match, allowedGlobMatchPattern)
		}
		if strings.Contains(m.Match, "**") {
			return fmt.Errorf("invalid glob match pattern `%s`, it should not contain consecutive `*`", m.Match)
		}
		metricRe = strings.Replace(metricRe, ".", "\\.", -1)
		metricRe = strings.Replace(metricRe, "*", "([^.]*)", -1)
	}
	regex, err := regexp.Compile("^" + metricRe + "$")
	if err != nil {
		return fmt.Errorf("invalid match `%s`. cannot compile regex: %v", m.Match, err)
	}
	m.regex = regex
	return nil
}

// getMapping returns:
// - name: the mapped expanded name
// - tags: the tags extracted from the metric name and expanded
// - matched: weather we found a match or not
func (m *metricMapper) getMapping(metricName string) (string, []string, bool) {
	result, cached := m.cache.get(metricName)
	if cached {
		return result.Name, result.Tags, result.Matched
	}
	for _, mapping := range m.Mappings {
		matches := mapping.regex.FindStringSubmatchIndex(metricName)
		if len(matches) == 0 {
			continue
		}

		name := string(mapping.regex.ExpandString(
			[]byte{},
			mapping.Name,
			metricName,
			matches,
		))

		var tags []string
		for tagKey, tagValueExpr := range mapping.Tags {
			tagValue := string(mapping.regex.ExpandString([]byte{}, tagValueExpr, metricName, matches))
			tags = append(tags, tagKey+":"+tagValue)
		}

		m.cache.addMatch(metricName, name, tags)
		return name, tags, true
	}
	m.cache.addMiss(metricName)
	return "", nil, false
}
