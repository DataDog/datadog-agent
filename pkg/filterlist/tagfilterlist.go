package filterlist

import "slices"

type MetricTagList struct {
	Tags []string `yaml:"tags"`
	// If Negated == false, we keep only the provided tags (allow list).
	// If Negated == true, we strip the provided tags (deny list).
	Negated bool `yaml:"tags_negated"`
}

type TagMatcher struct {
	Metrics map[string]MetricTagList
}

func NewTagMatcher(metrics map[string]MetricTagList) *TagMatcher {
	return &TagMatcher{
		Metrics: metrics,
	}
}

// ShouldStripTags returns true if it has been configured to strip tags
// from the given metric name.
func (m *TagMatcher) ShouldStripTags(metricName string) (MetricTagList, bool) {
	tags, ok := m.Metrics[metricName]
	return tags, ok
}

// KeepTag will return true if the given tagname should be kept.
func (tm MetricTagList) KeepTag(tag string) bool {
	return slices.Contains(tm.Tags, tag) != tm.Negated
}
