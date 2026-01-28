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
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
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

	cfg := make(map[string]interface{})
	cfg["metric_filterlist"] = []string{"local.metric.1", "local.metric.2"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

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

	// metric filterlist has been reset to default
	metricNames = configComponent.GetStringSlice("metric_filterlist")
	require.ElementsMatch([]string{"local.metric.1", "local.metric.2"}, metricNames)

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

// TestMergeMetricTagListEntry_SameActionInclude tests merging tags when both entries have Include action
func TestMergeMetricTagListEntry_SameActionInclude(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	// Setup: current entry with Include action
	currentHashed := hashedMetricTagList{
		action: Include,
		tags:   hashTags([]string{"env", "host"}),
	}
	currentEntry := MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "include",
		Tags:       []string{"env", "host"},
	}

	// New entry also with Include action
	newMetric := tagEntry{
		Name:       "test.metric",
		ExcludeTag: false,
		Tags:       []string{"pod", "cluster"},
	}

	// Execute merge
	hashedResult, entryResult := filterList.mergeMetricTagListEntry(newMetric, currentHashed, currentEntry)

	require.Equal(hashedResult, hashedMetricTagList{
		action: Include,
		tags:   hashTags([]string{"env", "host", "pod", "cluster"}),
	})

	require.Equal(entryResult, MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "include",
		Tags:       []string{"env", "host", "pod", "cluster"},
	})
}

// TestMergeMetricTagListEntry_SameActionExclude tests merging tags when both entries have Exclude action
func TestMergeMetricTagListEntry_SameActionExclude(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	// Setup: current entry with Exclude action
	currentHashed := hashedMetricTagList{
		action: Exclude,
		tags:   hashTags([]string{"env", "host"}),
	}
	currentEntry := MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "exclude",
		Tags:       []string{"env", "host"},
	}

	// New entry also with Exclude action
	newMetric := tagEntry{
		Name:       "test.metric",
		ExcludeTag: true,
		Tags:       []string{"pod", "cluster"},
	}

	// Execute merge
	hashedResult, entryResult := filterList.mergeMetricTagListEntry(newMetric, currentHashed, currentEntry)

	require.Equal(hashedResult, hashedMetricTagList{
		action: Exclude,
		tags:   hashTags([]string{"env", "host", "pod", "cluster"}),
	})

	require.Equal(entryResult, MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "exclude",
		Tags:       []string{"env", "host", "pod", "cluster"},
	})
}

// TestMergeMetricTagListEntry_IncludeOverriddenByExclude tests that Exclude overwrites Include
func TestMergeMetricTagListEntry_IncludeOverriddenByExclude(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	// Setup: current entry with Include action
	currentHashed := hashedMetricTagList{
		action: Include,
		tags:   hashTags([]string{"env", "host"}),
	}
	currentEntry := MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "include",
		Tags:       []string{"env", "host"},
	}

	// New entry with Exclude action (should overwrite)
	newMetric := tagEntry{
		Name:       "test.metric",
		ExcludeTag: true,
		Tags:       []string{"pod"},
	}

	// Execute merge
	hashedResult, entryResult := filterList.mergeMetricTagListEntry(newMetric, currentHashed, currentEntry)

	require.Equal(hashedResult, hashedMetricTagList{
		action: Exclude,
		tags:   hashTags([]string{"pod"}),
	})

	require.Equal(entryResult, MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "exclude",
		Tags:       []string{"pod"},
	})
}

// TestMergeMetricTagListEntry_ExcludeIgnoresInclude tests that Exclude entry ignores Include updates
func TestMergeMetricTagListEntry_ExcludeIgnoresInclude(t *testing.T) {
	require := require.New(t)

	filterList, _ := newFilterList(t)

	// Setup: current entry with Exclude action
	currentHashed := hashedMetricTagList{
		action: Exclude,
		tags:   hashTags([]string{"env", "host"}),
	}
	currentEntry := MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "exclude",
		Tags:       []string{"env", "host"},
	}

	// New entry with Include action (should be ignored)
	newMetric := tagEntry{
		Name:       "test.metric",
		ExcludeTag: false,
		Tags:       []string{"pod"},
	}

	// Execute merge
	hashedResult, entryResult := filterList.mergeMetricTagListEntry(newMetric, currentHashed, currentEntry)

	// Results should be the original exclude
	require.Equal(hashedResult, hashedMetricTagList{
		action: Exclude,
		tags:   hashTags([]string{"env", "host"}),
	})

	require.Equal(entryResult, MetricTagListEntry{
		MetricName: "test.metric",
		Action:     "exclude",
		Tags:       []string{"env", "host"},
	})
}
