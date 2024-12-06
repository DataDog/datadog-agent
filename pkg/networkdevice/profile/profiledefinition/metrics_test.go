// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
)

func TestCloneSymbolConfig(t *testing.T) {
	s := SymbolConfig{
		OID:              "1.2.3.4",
		Name:             "foo",
		ExtractValue:     regexp.MustCompile(".*"),
		MatchPattern:     regexp.MustCompile(".*"),
		MatchValue:       "$1",
		ScaleFactor:      100,
		Format:           "mac_address",
		ConstantValueOne: true,
		MetricType:       ProfileMetricTypeCounter,
	}
	s2 := s.Clone()
	assert.Equal(t, s, s2)
	// confirm they aren't the same object
	s2.ExtractValue.Longest()
	assert.NotEqual(t, s, s2)
}

func TestCloneSymbolConfigCompat(t *testing.T) {
	s := SymbolConfigCompat{
		OID:              "1.2.3.4",
		Name:             "foo",
		ExtractValue:     regexp.MustCompile(".*"),
		MatchPattern:     regexp.MustCompile(".*"),
		MatchValue:       "$1",
		ScaleFactor:      100,
		Format:           "mac_address",
		ConstantValueOne: true,
		MetricType:       ProfileMetricTypeCounter,
	}
	s2 := s.Clone()
	assert.Equal(t, s, s2)
	// confirm they aren't the same object
	s2.ExtractValue.Longest()
	assert.NotEqual(t, s, s2)

}

func TestCloneMetricTagConfig(t *testing.T) {
	c := MetricTagConfig{
		Tag:   "foo",
		Index: 10,
		Column: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: regexp.MustCompile(".*"),
		},
		OID:    "2.4",
		Symbol: SymbolConfigCompat{},
		IndexTransform: []MetricIndexTransform{
			{
				Start: 0,
				End:   1,
			},
		},
		Mapping: map[string]string{
			"1": "bar",
			"2": "baz",
		},
		Match: regexp.MustCompile(".*"),
		Tags: map[string]string{
			"foo": "$1",
		},
		SymbolTag: "baz",
	}
	c2 := c.Clone()
	assert.Equal(t, c, c2)
	c2.Tags["bar"] = "$2"
	c2.IndexTransform = append(c2.IndexTransform, MetricIndexTransform{1, 3})
	c2.Mapping["3"] = "foo"
	c2.Tag = "bar"
	assert.NotEqual(t, c, c2)
	// Validate that c has not changed
	assert.Equal(t, c, MetricTagConfig{
		Tag:   "foo",
		Index: 10,
		Column: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: regexp.MustCompile(".*"),
		},
		OID:    "2.4",
		Symbol: SymbolConfigCompat{},
		IndexTransform: []MetricIndexTransform{
			{
				Start: 0,
				End:   1,
			},
		},
		Mapping: map[string]string{
			"1": "bar",
			"2": "baz",
		},
		Match: regexp.MustCompile(".*"),
		Tags: map[string]string{
			"foo": "$1",
		},
		SymbolTag: "baz",
	})
}

func TestCloneMetricsConfig(t *testing.T) {
	conf := &MetricsConfig{
		MIB: "FOO-MIB",
		Table: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: regexp.MustCompile(".*"),
		},
		Symbol: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: regexp.MustCompile(".*"),
		},
		OID:  "1.2.3.4",
		Name: "foo",
		Symbols: []SymbolConfig{
			{
				OID:          "1.2.3.4",
				ExtractValue: regexp.MustCompile(".*"),
			},
		},
		StaticTags: []string{
			"foo",
			"bar",
		},
		MetricTags: []MetricTagConfig{
			{
				IndexTransform: make([]MetricIndexTransform, 0),
			},
		},
		ForcedType: ProfileMetricTypeCounter,
		MetricType: ProfileMetricTypeGauge,
		Options: MetricsConfigOption{
			Placement:    1,
			MetricSuffix: ".foo",
		},
	}
	unchanged := &MetricsConfig{
		MIB: "FOO-MIB",
		Table: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: regexp.MustCompile(".*"),
		},
		Symbol: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: regexp.MustCompile(".*"),
		},
		OID:  "1.2.3.4",
		Name: "foo",
		Symbols: []SymbolConfig{
			{
				OID:          "1.2.3.4",
				ExtractValue: regexp.MustCompile(".*"),
			},
		},
		StaticTags: []string{
			"foo",
			"bar",
		},
		MetricTags: []MetricTagConfig{
			{
				IndexTransform: make([]MetricIndexTransform, 0),
			},
		},
		ForcedType: ProfileMetricTypeCounter,
		MetricType: ProfileMetricTypeGauge,
		Options: MetricsConfigOption{
			Placement:    1,
			MetricSuffix: ".foo",
		},
	}
	re2 := regexp.MustCompile(".*")
	re2.Longest()
	expected := &MetricsConfig{
		MIB: "FOO-MIB",
		Table: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: re2,
		},
		Symbol: SymbolConfig{
			OID:          "1.2.3.4",
			ExtractValue: re2,
		},
		OID:  "1.2.3.4",
		Name: "foo",
		Symbols: []SymbolConfig{
			{
				OID:          "1.2.3.4",
				ExtractValue: re2,
			},
		},
		StaticTags: []string{
			"baz",
			"bar",
		},
		MetricTags: []MetricTagConfig{
			{
				IndexTransform: []MetricIndexTransform{{
					Start: 5,
					End:   7,
				}},
			},
		},
		ForcedType: ProfileMetricTypeCounter,
		MetricType: ProfileMetricTypeGauge,
		Options: MetricsConfigOption{
			Placement:    2,
			MetricSuffix: ".bar",
		},
	}
	conf2 := conf.Clone()
	assert.Equal(t, conf, conf2)
	conf2.Table.ExtractValue.Longest()
	conf2.Symbol.ExtractValue.Longest()
	conf2.Symbols[0].ExtractValue.Longest()
	conf2.StaticTags[0] = "baz"
	conf2.MetricTags[0].IndexTransform = []MetricIndexTransform{{5, 7}}
	conf2.Options.Placement = 2
	conf2.Options.MetricSuffix = ".bar"
	assert.Equal(t, unchanged, conf)
	assert.Equal(t, expected, conf2)
}
