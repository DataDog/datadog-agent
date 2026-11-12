// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package filterlistimpl

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// to validate the outcome of running the config update,
// the callback will update this object
type updateRes map[state.ApplyState][]string

type resultTester struct {
	Acked    int
	Unacked  int
	Errors   int
	Unknowns int
}

func newFilterList(t *testing.T) (*FilterList, config.Component) {
	cfg := make(map[string]interface{})
	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	return NewFilterList(logComponent, configComponent, telemetryComponent), configComponent
}

// Validating that the Agent isn't crashing on malformed updates.
func TestMalformedFilterListUpdate(t *testing.T) {
	require := require.New(t)
	test := func(results updateRes, tester resultTester) {
		require.Len(results[state.ApplyStateAcknowledged], tester.Acked, "wrong amount of acked")
		require.Len(results[state.ApplyStateUnacknowledged], tester.Unacked, "wrong amount of unacked")
		require.Len(results[state.ApplyStateError], tester.Errors, "wrong amount of errors")
		require.Len(results[state.ApplyStateUnknown], tester.Unknowns, "wrong amount of unknowns")
	}
	reset := func() updateRes { return updateRes{} }

	filterList, _ := newFilterList(t)

	results := reset()

	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// this call should fail because the content is malformed
	updates := map[string]state.RawConfig{
		"first": {Config: []byte(`malformedjson":{}}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()

	// both of these should fail
	updates = map[string]state.RawConfig{
		"first":  {Config: []byte(`malformedjson":{}}`)},
		"second": {Config: []byte(`malformedjson":{}}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   2,
		Unknowns: 0,
	})
	results = reset()

	// one is incorrect json, the other is an unknown struct we don't know
	// how to interpret, but that'll still be processed without errors (acked)
	updates = map[string]state.RawConfig{
		"first":  {Config: []byte(`malformedjson":{}}`)},
		"second": {Config: []byte(`{"random":"json","field":[]}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    1,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()

	// two correct ones, with one metric
	updates = map[string]state.RawConfig{
		"first":  {Config: []byte(`{"blocking_metrics":{"by_name":["hello","world"]}`)},
		"second": {Config: []byte(`{"random":"json","field":[]}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    1,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()

	// nothing
	updates = map[string]state.RawConfig{}
	filterList.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   0,
		Unknowns: 0,
	})
	results = reset()

	// one config but empty, it should be unparseable
	updates = map[string]state.RawConfig{
		"first": {Config: []byte("")},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()
}

// TestFilterListUpdateWithValidMetrics tests the callback with valid metric filter list updates
func TestFilterListUpdateWithValidMetrics(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Valid metric filter list update
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [
						{"metric_name": "test.metric.1"},
						{"metric_name": "test.metric.2"},
						{"metric_name": "test.metric.1"}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	// Verify metrics were set in config
	metricNames := configComponent.GetStringSlice("metric_filterlist")
	require.ElementsMatch([]string{"test.metric.1", "test.metric.2"}, metricNames)
}

// blocked_metrics entries from RC stay exact; `*` is not a prefix marker.
func TestFilterListUpdateBlockedMetricsStarIsLiteral(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [
						{"metric_name": "test.exactfoo*"},
						{"metric_name": "test.exact"}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	metricNames := configComponent.GetStringSlice("metric_filterlist")
	require.ElementsMatch([]string{"test.exactfoo", "test.exact"}, metricNames)

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("test.exactfoo"))
	require.False(matcher.Test("test.exactfooX"))
	require.True(matcher.Test("test.exact"))
	require.False(matcher.Test("test.exact.suffix"))
	require.False(matcher.Test("test.other"))
}

// RC prefix rules are converted to the local config shape.
func TestFilterListUpdateWithMetricPrefixRules(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"metric_filterlist_prefix": {
				"by_prefix": {
					"values": [
						{
							"prefix": "test.prefixed.",
							"except_prefix": [{"prefix": "test.prefixed.locks."}],
							"except_exact": [{"name": "test.prefixed.keep"}]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	var prefixEntries []MetricPrefixListEntry
	err := structure.UnmarshalKey(configComponent, "metric_filterlist_prefix", &prefixEntries)
	require.NoError(err)
	require.Len(prefixEntries, 1)
	require.Equal(MetricPrefixListEntry{
		Prefix:       "test.prefixed.",
		ExceptPrefix: []string{"test.prefixed.locks."},
		ExceptExact:  []string{"test.prefixed.keep"},
	}, prefixEntries[0])

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("test.prefixed.anything"))
	require.False(matcher.Test("test.prefixed.keep"), "except_exact entry should be kept")
	require.False(matcher.Test("test.prefixed.locks.waiting"), "except_prefix entry should be kept")
	require.False(matcher.Test("test.other"))

	// Prefix rules also apply to histogram aggregates.
	histo := filterList.GetHistoFilterList()
	require.True(histo.Test("test.prefixed.histo.avg"))
	require.False(histo.Test("test.prefixed.keep"))
}

func TestFilterListUpdateWithDuplicateMetricPrefixRulesPreservesBoth(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"metric_filterlist_prefix": {
				"by_prefix": {
					"values": [
						{
							"prefix": "test.dup.",
							"except_exact": [{"name": "test.dup.keep.one"}, {"name": "test.dup.keep.both"}]
						},
						{
							"prefix": "test.dup.",
							"except_exact": [{"name": "test.dup.keep.two"}, {"name": "test.dup.keep.both"}]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	var prefixEntries []MetricPrefixListEntry
	err := structure.UnmarshalKey(configComponent, "metric_filterlist_prefix", &prefixEntries)
	require.NoError(err)
	require.Len(prefixEntries, 2)

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("test.dup.anything"))
	require.False(matcher.Test("test.dup.keep.one"), "exceptions apply across duplicate rules")
	require.False(matcher.Test("test.dup.keep.two"), "exceptions apply across duplicate rules")
	require.False(matcher.Test("test.dup.keep.both"), "excepted by both duplicate rules")
}

// RC configs with only prefix rules are not empty.
func TestFilterListUpdateWithOnlyPrefixRulesIsNotSkipped(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"metric_filterlist_prefix": {
				"by_prefix": {
					"values": [{"prefix": "test.prefixed."}]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("test.prefixed.anything"), "a config carrying only prefix rules must not be skipped")
}

// RC exact and prefix-rule fields fall back independently.
func TestFilterListUpdateMetricNamesAndPrefixRulesAreIndependent(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"metric_filterlist": []string{"local.exact"},
		"metric_filterlist_prefix": []map[string]interface{}{
			{"prefix": "local.prefixed."},
		},
	}
	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Only prefix rules come from RC.
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"metric_filterlist_prefix": {
				"by_prefix": {
					"values": [{"prefix": "rc.prefixed."}]
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("local.exact"), "metric_filterlist should fall back to local config")
	require.True(matcher.Test("rc.prefixed.anything"), "metric_filterlist_prefix should come from RC")
	require.False(matcher.Test("local.prefixed.anything"), "the local prefix rule should have been replaced by RC's")

	// Now only exact names come from RC.
	results = updateRes{}
	updates = map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [{"metric_name": "rc.exact"}]
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	matcher = filterList.GetMetricFilterList()
	require.True(matcher.Test("rc.exact"), "metric_filterlist should come from RC")
	require.False(matcher.Test("local.exact"), "the local exact metric should have been replaced by RC's")
	require.True(matcher.Test("local.prefixed.anything"), "metric_filterlist_prefix should fall back to local config")
}

func TestFilterListUpdateRestoresDeprecatedStatsdMetricBlocklistFromFile(t *testing.T) {
	require := require.New(t)

	logComponent := logmock.New(t)
	configComponent := config.NewMockFromYAML(t, `
statsd_metric_blocklist:
  - local.legacy
statsd_metric_blocklist_match_prefix: true
`)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	require.ElementsMatch([]string{"local.legacy"}, configComponent.GetStringSlice("metric_filterlist"))
	require.Equal(model.SourceFile, configComponent.GetSource("metric_filterlist"))
	require.True(configComponent.GetBool("metric_filterlist_match_prefix"))
	require.Equal(model.SourceFile, configComponent.GetSource("metric_filterlist_match_prefix"))
	require.False(configComponent.IsConfigured("statsd_metric_blocklist"))
	require.False(configComponent.IsConfigured("statsd_metric_blocklist_match_prefix"))

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [{"metric_name": "rc.exact"}]
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	require.ElementsMatch([]string{"rc.exact"}, configComponent.GetStringSlice("metric_filterlist"))
	require.Equal(model.SourceRC, configComponent.GetSource("metric_filterlist"))
	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("rc.exact"), "metric_filterlist should come from RC")
	require.False(matcher.Test("local.legacy"), "the deprecated local metric should not be applied while RC is active")

	results = updateRes{}
	emptyUpdates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": []
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(emptyUpdates, callback)
	require.Len(results[state.ApplyStateError], 0)

	require.ElementsMatch([]string{"local.legacy"}, configComponent.GetStringSlice("metric_filterlist"))
	require.Equal(model.SourceFile, configComponent.GetSource("metric_filterlist"))
	matcher = filterList.GetMetricFilterList()
	require.True(matcher.Test("local.legacy.anything"), "empty RC update should restore the deprecated file config after migration")
	require.False(matcher.Test("rc.exact"), "empty RC update should remove the RC metric_filterlist")
}

// Empty RC prefix rules restore the local prefix rules.
func TestFilterListUpdatePrefixRulesEmptyRestoresLocal(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"metric_filterlist_prefix": []map[string]interface{}{
			{"prefix": "local.prefixed."},
		},
	}
	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"metric_filterlist_prefix": {
				"by_prefix": {
					"values": [{"prefix": "rc.prefixed."}]
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("rc.prefixed.anything"))
	require.False(matcher.Test("local.prefixed.anything"))

	// No prefix rules in RC: fall back to local config.
	results = updateRes{}
	emptyUpdates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [{"metric_name": "still.here"}]
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(emptyUpdates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	matcher = filterList.GetMetricFilterList()
	require.False(matcher.Test("rc.prefixed.anything"), "RC's prefix rule should be gone")
	require.True(matcher.Test("local.prefixed.anything"), "the local prefix rule should be restored")
	require.True(matcher.Test("still.here"), "the exact metric from this same update should still apply")
}

// TestFilterListUpdateWithValidTags tests the callback with valid tag filter list updates
func TestFilterListUpdateWithValidTags(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Valid tag filter list update with exclude mode
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839205,
							"metric_name": "test.distribution",
							"exclude_tags_mode": true,
							"tags": ["env", "host"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	// Verify tag filter list was set in config with unhashed tag names
	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal(MetricTagListEntry{
		MetricName: "test.distribution",
		Action:     "exclude",
		Tags:       []string{"env", "host"},
	}, tagEntries[0])
}

// TestFilterListIgnoresUnknownSections tests a config file with unknown keys is handled gracefully
// and the unknown keys are ignored.
func TestFilterListIgnoresUnknownSections(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Valid tag filter list update with exclude mode
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839205,
							"metric_name": "test.distribution",
							"exclude_tags_mode": true,
							"tags": ["env", "host"]
						}
					]
				},
				"by_name_pattern": {
					"values": [
						{
                                                        "created_at": 1768839205,
							"metric_name_include": "test.distribution.*",
							"metric_name_exclude": ["test.distribution.thing.*"],
							"exclude_tags_mode": true,
							"tags": ["env", "host"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	// Verify tag filter list was set in config with unhashed tag names
	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal(MetricTagListEntry{
		MetricName: "test.distribution",
		Action:     "exclude",
		Tags:       []string{"env", "host"},
	}, tagEntries[0])

}

// TestFilterListUpdateWithMergedTags tests tag merging logic
func TestFilterListUpdateWithMergedTags(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Multiple updates with same metric name and same action - should merge tags
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839203,
							"metric_name": "test.metric",
							"exclude_tags_mode": true,
							"tags": ["env", "host"]
						}
					]
				}
			}
		}`)},
		"config2": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839205,
							"metric_name": "test.metric",
							"exclude_tags_mode": true,
							"tags": ["pod", "cluster"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 2)
	require.Len(results[state.ApplyStateError], 0)

	// Verify tags were merged
	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	slices.Sort(tagEntries[0].Tags)
	require.Equal(
		MetricTagListEntry{
			MetricName: "test.metric",
			Action:     "exclude",
			Tags:       []string{"cluster", "env", "host", "pod"},
		}, tagEntries[0])

}

// TestFilterListUpdateWithConflictingActions tests that exclude takes precedence over include
func TestFilterListUpdateWithConflictingActions(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// First include, then exclude - exclude should win
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839205,
							"metric_name": "test.metric",
							"exclude_tags_mode": false,
							"tags": ["env", "host"]
						}
					]
				}
			}
		}`)},
		"config2": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839205,
							"metric_name": "test.metric",
							"exclude_tags_mode": true,
							"tags": ["pod"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 2)

	// Verify exclude action was kept with new tags
	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal(
		MetricTagListEntry{
			MetricName: "test.metric",
			Action:     "exclude",
			Tags:       []string{"pod"},
		},
		tagEntries[0],
	)

}

// TestFilterListUpdateWithCombinedMetricsAndTags tests both metric and tag filter list updates
func TestFilterListUpdateWithCombinedMetricsAndTags(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Update with both blocked metrics and tag filterlist
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [
						{"metric_name": "blocked.metric.1"},
						{"metric_name": "blocked.metric.2"}
					]
				}
			},
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839203,
							"metric_name": "dist.metric",
							"exclude_tags_mode": true,
							"tags": ["env", "version"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)
	require.Len(results[state.ApplyStateError], 0)

	// Verify both were set
	metricNames := configComponent.GetStringSlice("metric_filterlist")
	require.ElementsMatch([]string{"blocked.metric.1", "blocked.metric.2"}, metricNames)

	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal("dist.metric", tagEntries[0].MetricName)
}

// TestFilterListUpdateEmptyRestoresLocal tests that empty updates restore local config
func TestFilterListUpdateEmptyRestoresLocal(t *testing.T) {
	require := require.New(t)

	logComponent := logmock.New(t)
	configComponent := config.NewMockFromYAML(t, `
metric_filterlist:
  - local.metric.1
  - local.metric.2
`)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	require.Equal(model.SourceFile, configComponent.GetSource("metric_filterlist"))

	// First, set RC config
	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": [{"metric_name": "rc.metric"}]
				}
			},
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839203,
							"metric_name": "rc.distribution",
							"exclude_tags_mode": true,
							"tags": ["rc_tag"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	// Verify RC config was set
	metricNames := configComponent.GetStringSlice("metric_filterlist")
	require.ElementsMatch([]string{"rc.metric"}, metricNames)

	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal(
		MetricTagListEntry{
			MetricName: "rc.distribution",
			Action:     "exclude",
			Tags:       []string{"rc_tag"},
		},
		tagEntries[0],
	)

	// Now send empty update - should trigger restoration logic
	results = updateRes{}
	emptyUpdates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"blocked_metrics": {
				"by_name": {
					"values": []
				}
			},
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839203,
							"metric_name": "rc.distribution",
							"exclude_tags_mode": true,
							"tags": ["rc_tag"]
						}
					]
				}
			}
		}`)},
	}
	filterList.onFilterListUpdateCallback(emptyUpdates, callback)
	require.Len(results[state.ApplyStateError], 0)

	// metric filterlist has been reset to local file config
	metricNames = configComponent.GetStringSlice("metric_filterlist")
	require.ElementsMatch([]string{"local.metric.1", "local.metric.2"}, metricNames)
	require.Equal(model.SourceFile, configComponent.GetSource("metric_filterlist"))

	// tag entries should be as configured through RC
	err = structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal(
		MetricTagListEntry{
			MetricName: "rc.distribution",
			Action:     "exclude",
			Tags:       []string{"rc_tag"},
		},
		tagEntries[0],
	)
}

// TestFilterListUpdateWithIncludeMode tests include mode for tag filtering
func TestFilterListUpdateWithIncludeMode(t *testing.T) {
	require := require.New(t)

	filterList, configComponent := newFilterList(t)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Update with include mode
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839203,
							"metric_name": "test.metric",
							"exclude_tags_mode": false,
							"tags": ["important_tag"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	// Verify include action was set
	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 1)
	require.Equal("include", tagEntries[0].Action)
}

// TestFilterListUpdateMultipleMetrics tests handling of multiple different metrics
func TestFilterListUpdateMultipleMetrics(t *testing.T) {
	require := require.New(t)

	cfg := make(map[string]interface{})

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	results := updateRes{}
	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// Update with multiple different metrics in tag filterlist
	updates := map[string]state.RawConfig{
		"config1": {Config: []byte(`{
			"tag_filterlist": {
				"by_name": {
					"values": [
						{
                                                        "created_at": 1768839203,
							"metric_name": "metric.one",
							"exclude_tags_mode": true,
							"tags": ["env"]
						},
						{
                                                        "created_at": 1768839204,
							"metric_name": "metric.two",
							"exclude_tags_mode": false,
							"tags": ["host"]
						},
						{
                                                        "created_at": 1768839205,
							"metric_name": "metric.three",
							"exclude_tags_mode": true,
							"tags": ["pod", "cluster"]
						}
					]
				}
			}
		}`)},
	}

	filterList.onFilterListUpdateCallback(updates, callback)
	require.Len(results[state.ApplyStateAcknowledged], 1)

	// Verify all metrics were stored
	var tagEntries []MetricTagListEntry
	err := structure.UnmarshalKey(configComponent, "metric_tag_filterlist", &tagEntries)
	require.NoError(err)
	require.Len(tagEntries, 3)

	// Verify each metric
	metricMap := make(map[string]MetricTagListEntry)
	for _, entry := range tagEntries {
		metricMap[entry.MetricName] = entry
	}

	require.Contains(metricMap, "metric.one")
	require.Equal(
		MetricTagListEntry{
			MetricName: "metric.one",
			Action:     "exclude",
			Tags:       []string{"env"},
		},
		metricMap["metric.one"],
	)

	require.Contains(metricMap, "metric.two")
	require.Equal(
		MetricTagListEntry{
			MetricName: "metric.two",
			Action:     "include",
			Tags:       []string{"host"},
		},
		metricMap["metric.two"],
	)

	require.Contains(metricMap, "metric.three")
	require.Equal(
		MetricTagListEntry{
			MetricName: "metric.three",
			Action:     "exclude",
			Tags:       []string{"pod", "cluster"},
		},
		metricMap["metric.three"],
	)
}

// TestNewHashedMetricTagListTagsAreSorted verifies that newHashedMetricTagList always returns
// a hashedMetricTagList with tags in sorted order, regardless of input order.
func TestNewHashedMetricTagListTagsAreSorted(t *testing.T) {
	require := require.New(t)

	// Use tags whose hash values are unlikely to already be in sorted order.
	tags := []string{"zzz", "aaa", "mmm", "bbb", "env", "host"}
	hashedTags := hashTags(tags)
	result := newHashedMetricTagList(exclude, hashedTags)
	require.True(slices.IsSorted(result.tags), "tags in hashedMetricTagList must always be sorted")

	// Also verify with include action.
	result = newHashedMetricTagList(include, hashedTags)
	require.True(slices.IsSorted(result.tags), "tags in hashedMetricTagList must always be sorted")
}

// TestBuildTagFilterListConfigTagsAreSorted verifies that the matcher compiled from
// buildTagFilterListConfig always has its tags in sorted order.
func TestBuildTagFilterListConfigTagsAreSorted(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	updates := []filteredTags{
		{
			ByName: tagByName{
				Metrics: []tagEntry{
					{
						Name:       "test.metric",
						ExcludeTag: true,
						Tags:       []string{"zzz", "aaa", "mmm", "bbb"},
					},
					{
						Name:       "test.metric",
						ExcludeTag: true,
						Tags:       []string{"ccc", "yyy"},
					},
				},
			},
		},
	}

	matcher := compileTagFilterList(filterList.buildTagFilterListConfig(updates), filterList.log)
	require.True(slices.IsSorted(matcher.MetricTags["test.metric"].tags), "compiled tags must be sorted")
	require.Len(matcher.MetricTags["test.metric"].tags, 6)
}

// TestBuildTagFilterListConfigReportsEntriesAsReceived verifies that RC entries
// are reported in the configuration as received, and only normalized in the
// compiled matcher.
func TestBuildTagFilterListConfigReportsEntriesAsReceived(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	updates := []filteredTags{{ByName: tagByName{Metrics: []tagEntry{
		{Name: "my-metric", ExcludeTag: true, Tags: []string{"Pod Name"}},
	}}}}

	entries := filterList.buildTagFilterListConfig(updates)
	require.Equal([]MetricTagListEntry{{MetricName: "my-metric", Action: "exclude", Tags: []string{"Pod Name"}}}, entries)

	matcher := compileTagFilterList(entries, filterList.log)
	require.Equal(newHashedMetricTagList(exclude, hashTags([]string{"pod_name"})), matcher.MetricTags["my_metric"])
}

// TestMergeMetricTagListEntry tests the merging of RC entries for the same metric.
func TestMergeMetricTagListEntry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current MetricTagListEntry
		metric  tagEntry
		want    MetricTagListEntry
	}{
		{
			name:    "same action include merges tags",
			current: MetricTagListEntry{MetricName: "test.metric", Action: "include", Tags: []string{"env", "host"}},
			metric:  tagEntry{Name: "test.metric", ExcludeTag: false, Tags: []string{"pod", "cluster"}},
			want:    MetricTagListEntry{MetricName: "test.metric", Action: "include", Tags: []string{"env", "host", "pod", "cluster"}},
		},
		{
			name:    "same action exclude merges tags",
			current: MetricTagListEntry{MetricName: "test.metric", Action: "exclude", Tags: []string{"env", "host"}},
			metric:  tagEntry{Name: "test.metric", ExcludeTag: true, Tags: []string{"pod", "cluster"}},
			want:    MetricTagListEntry{MetricName: "test.metric", Action: "exclude", Tags: []string{"env", "host", "pod", "cluster"}},
		},
		{
			name:    "include overridden by exclude",
			current: MetricTagListEntry{MetricName: "test.metric", Action: "include", Tags: []string{"env", "host"}},
			metric:  tagEntry{Name: "test.metric", ExcludeTag: true, Tags: []string{"pod"}},
			want:    MetricTagListEntry{MetricName: "test.metric", Action: "exclude", Tags: []string{"pod"}},
		},
		{
			name:    "exclude ignores include",
			current: MetricTagListEntry{MetricName: "test.metric", Action: "exclude", Tags: []string{"env", "host"}},
			metric:  tagEntry{Name: "test.metric", ExcludeTag: false, Tags: []string{"pod"}},
			want:    MetricTagListEntry{MetricName: "test.metric", Action: "exclude", Tags: []string{"env", "host"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filterList, _ := newFilterList(t)
			// The current entry's tags must not be modified in place: they may
			// alias the RC payload.
			original := slices.Clone(tc.current.Tags)
			current := tc.current
			current.Tags = slices.Clip(current.Tags)

			require.Equal(t, tc.want, filterList.mergeMetricTagListEntry(tc.metric, current))
			require.Equal(t, original, current.Tags)
		})
	}
}
