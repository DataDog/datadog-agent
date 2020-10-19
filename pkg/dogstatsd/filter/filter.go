package filter

import (
	"regexp"
	"strings"
)

const tagSeparator = ":"

// TagFilter filters a slice of tags against a list of regular expressions
type TagFilter struct {
	filters []*regexp.Regexp
}

// New compiles a list of regular expressions and returns a TagFilter or an
// error if one of the expressions fails not compile
func New(regexes []string) (*TagFilter, error) {
	filters := make([]*regexp.Regexp, len(regexes))
	for i, r := range regexes {
		regex, err := regexp.Compile(r)
		if err != nil {
			return nil, err
		}

		filters[i] = regex
	}
	return &TagFilter{
		filters: filters,
	}, nil
}

// Filter a slice of tags against a pre-compiled list of regular expressions
// and return a list of tags. Undesirable tags that match an expression are
// removed. Any expression that contains a match group named "Keep" will return
// only the matching substring if and only if that substring contains the
// required ':' tag separator.
func (tf *TagFilter) Filter(tags []string) []string {
	var i int
	for _, t := range tags {
		if keep, result := tf.check(t); keep {
			tags[i] = result
			i++
		}
	}

	// Optimization: erase truncated values in the backing array
	for j := i; j < len(tags); j++ {
		tags[j] = ""
	}

	return tags[:i]
}

// check returns true if the tag should be kept or replaced with the second
// string and false when the tag should be filtered.
func (tf *TagFilter) check(tag string) (bool, string) {
OUTER:
	for _, regex := range tf.filters {
		if m := regex.FindStringSubmatch(tag); m != nil {
			for i, name := range regex.SubexpNames() {
				if strings.ToLower(name) == "check" {
					if strings.Contains(m[i], tagSeparator) {
						// overwrite tag as the match group
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
