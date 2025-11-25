// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package strings

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type tags = []string

// Matcher test a string for match against a list of strings.
// See `NewMatcher` for details.
type Matcher struct {
	data        []string
	tags        map[string]tags
	matchPrefix bool
}

// NewMatcher creates a new strings matcher.
// Use `matchPrefix` to  create a prefixes matcher.
func NewMatcher(data []string, matchPrefix bool) Matcher {
	metricData := []string{}
	tagData := make(map[string]tags)
	for _, metric := range data {
		tags := strings.Split(metric, ":")
		if len(tags) > 1 {
			tag := tagData[tags[0]]
			tagData[tags[0]] = append(tag, tags[1:]...)
			log.Info("tag matcher", tags[0], tagData[tags[0]])
		} else {
			metricData = append(metricData, metric)
		}
	}
	sort.Strings(metricData)

	if matchPrefix && len(metricData) > 0 {
		// Make sure that elements identify unique prefixes.
		i := 0
		for j := 1; j < len(metricData); j++ {
			if strings.HasPrefix(metricData[j], metricData[i]) {
				continue
			}
			i++
			metricData[i] = metricData[j]
		}

		metricData = metricData[:i+1]
	}

	// Invariants for data:
	// For all i, j such that i < j, data[i] < data[j].
	// for all i, j such that i != j, !HasPrefix(data[i], data[j]).
	return Matcher{
		data:        metricData,
		tags:        tagData,
		matchPrefix: matchPrefix,
	}
}

// Test returns true if the given string matches one in the matcher list.
// or is matching by prefix if the matcher has been created with `matchPrefix`.
// First bool is if we should remove the metric completely, second is if we should
// remove a tag in the metric.
func (m *Matcher) Test(name string) (bool, bool) {
	if m == nil {
		return false, false
	}

	if len(m.data) == 0 && len(m.tags) == 0 {
		return false, false
	}

	i := sort.SearchStrings(m.data, name)

	// SearchStrings returns an index such that either:
	// - data[i] == name
	// - data[i-1] < name (if i > 0) && data[i] > name (if i < len(m.data))
	//
	// If for some j, data[j] is a prefix of name, then:
	//
	// - j < i, because any prefix of a string is less than string itself,
	//
	// - if j < i - 1, then strings in range [j+1, i-1] would have
	// data[j] as a prefix, which is impossible by construction of
	// data.
	//
	// Thus j must be i - 1.
	if m.matchPrefix && i > 0 && strings.HasPrefix(name, m.data[i-1]) {
		return true, false
	}
	if i < len(m.data) {
		return name == m.data[i], false
	}

	m.DebugTags("zoogzoggle")

	_, tag := m.tags[name]

	if tag {
		fmt.Println("\033[035m", "tags", name, "\033[0m")
	}

	return false, tag
}

func (m *Matcher) TestTag(metric string, tag string) bool {
	tags, ok := m.tags[metric]
	if !ok {
		return false
	}

	pos := strings.Index(tag, ":")
	return slices.Contains(tags, tag[:pos])
}

func (m *Matcher) DebugTags(ook string) {
	fmt.Println("\033[035m", ook, m.tags, "\033[0m")
	for k, v := range m.tags {
		fmt.Println("\033[035m", k, "=>", v, "\033[0m")
	}
}
