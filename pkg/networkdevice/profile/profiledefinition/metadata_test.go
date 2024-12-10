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

func TestCloneMetadata(t *testing.T) {
	// This is not actually a valid config, since e.g. it has ExtractValue and
	// MatchPattern both set; this is just to check that every field gets copied
	// properly.
	metadata := MetadataConfig{
		"device": MetadataResourceConfig{
			Fields: map[string]MetadataField{
				"name": {
					Value: "hey",
					Symbol: SymbolConfig{
						OID:              "1.2.3",
						Name:             "someSymbol",
						ExtractValue:     regexp.MustCompile(".*"),
						MatchPattern:     regexp.MustCompile(".*"),
						MatchValue:       "$1",
						ScaleFactor:      100,
						Format:           "mac_address",
						ConstantValueOne: true,
						MetricType:       "gauge",
					},
				},
			},
			IDTags: []MetricTagConfig{
				{
					Tag:   "foo",
					Index: 1,
					Column: SymbolConfig{
						Name:         "bar",
						OID:          "1.2.3",
						ExtractValue: regexp.MustCompile(".*"),
					},
					OID: "2.3.4",
					Symbol: SymbolConfigCompat{
						OID:          "1.2.3",
						Name:         "someSymbol",
						ExtractValue: regexp.MustCompile(".*"),
					},
					IndexTransform: []MetricIndexTransform{
						{
							Start: 1,
							End:   5,
						},
					},
					Mapping: map[string]string{
						"1": "on",
						"2": "off",
					},
					Match: regexp.MustCompile(".*"),
					Tags: map[string]string{
						"foo": "bar",
					},
					SymbolTag: "ok",
				},
			},
		},
	}

	// identical regexp, except with longest=true
	re2 := regexp.MustCompile(".*")
	re2.Longest()
	expected := MetadataConfig{
		"interface": MetadataResourceConfig{},
		"device": MetadataResourceConfig{
			Fields: map[string]MetadataField{
				"name": {
					Value: "hey",
					Symbol: SymbolConfig{
						OID:              "1.2.3",
						Name:             "someSymbol",
						ExtractValue:     re2,
						MatchPattern:     re2,
						MatchValue:       "$1",
						ScaleFactor:      100,
						Format:           "mac_address",
						ConstantValueOne: true,
						MetricType:       "gauge",
					},
				},
			},
			IDTags: []MetricTagConfig{
				{
					Tag:   "foo",
					Index: 1,
					Column: SymbolConfig{
						Name:         "bar",
						OID:          "1.2.3",
						ExtractValue: re2,
					},
					OID: "2.3.4",
					Symbol: SymbolConfigCompat{
						OID:          "1.2.3",
						Name:         "someSymbol",
						ExtractValue: re2,
					},
					IndexTransform: []MetricIndexTransform{
						{
							Start: 1,
							End:   5,
						},
					},
					Mapping: map[string]string{
						"1": "on",
						"2": "off",
					},
					Match: re2,
					Tags: map[string]string{
						"foo": "bar",
					},
					SymbolTag: "ok",
				},
			},
		},
	}

	metaCopy := metadata.Clone()
	assert.Equal(t, metadata, metaCopy)
	// Modify the copy in place
	metaCopy["interface"] = MetadataResourceConfig{}
	metaCopy["device"].Fields["name"].Symbol.ExtractValue.Longest()
	metaCopy["device"].Fields["name"].Symbol.MatchPattern.Longest()
	metaCopy["device"].IDTags[0].Column.ExtractValue.Longest()
	metaCopy["device"].IDTags[0].Symbol.ExtractValue.Longest()
	metaCopy["device"].IDTags[0].Match.Longest()
	// metaCopy should now match expected, and metadata should not have been affected.
	assert.Equal(t, expected, metaCopy)
	assert.NotEqual(t, metadata, metaCopy)
}
