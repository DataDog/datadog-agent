package filter

import (
	"regexp"
	"strings"
)

type TagFilter struct {
	filters []*regexp.Regexp
}

func NewTagFilter(regexes []string) (*TagFilter, error) {
	filters := make([]*regexp.Regexp, len(regexes))
	for i, r := range regexes {
		if regex, err := regexp.Compile(r); err != nil {
			return nil, err
		} else {
			filters[i] = regex
		}
	}

	return &TagFilter{
		filters: filters,
	}, nil
}

func (tf *TagFilter) Filter(tags []string) []string {
	var i int
	for _, t := range tags {
		if ok, result := tf.filter(t); ok {
			tags[i] = result
			i++
		}
	}

	// Prevent memory leak by erasing truncated values
	for j := i; j < len(tags); j++ {
		tags[j] = ""
	}

	return tags[:i]
}

func (tf *TagFilter) filter(tag string) (bool, string) {
	OUTER:
	for _, regex := range tf.filters {
		if m := regex.FindStringSubmatch(tag); m != nil {
			for i, name := range regex.SubexpNames() {
				if name == "Keep" {
					if strings.Contains(m[i], ":") {
						tag = m[i]
					}
					continue OUTER
				}
			}

			return false, ""
		}
	}

	return true, tag
}
