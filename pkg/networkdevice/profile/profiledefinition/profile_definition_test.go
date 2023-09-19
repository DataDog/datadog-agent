package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDeviceProfileRcConfig_NormalizeInplaceForRc(t *testing.T) {
	profile := DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Mapping: map[string]string{
								"1": "aa",
								"2": "bb",
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							Tags: map[string]string{
								"tag1": "$1",
								"tag2": "$2",
							},
						},
					},
				},
			},
		},
	}
	expectedProfile := DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							MappingList: []KeyValue{
								{
									Key:   "1",
									Value: "aa",
								},
								{
									Key:   "2",
									Value: "bb",
								},
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							TagsList: []KeyValue{
								{
									Key:   "tag1",
									Value: "$1",
								},
								{
									Key:   "tag2",
									Value: "$2",
								},
							},
						},
					},
				},
			},
		},
	}
	profile.NormalizeInplaceForRc()
	assert.Equal(t, expectedProfile, profile)
}

func TestDeviceProfileRcConfig_NormalizeInplaceFromRc(t *testing.T) {
	profile := DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							MappingList: []KeyValue{
								{
									Key:   "1",
									Value: "aa",
								},
								{
									Key:   "2",
									Value: "bb",
								},
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							TagsList: []KeyValue{
								{
									Key:   "tag1",
									Value: "$1",
								},
								{
									Key:   "tag2",
									Value: "$2",
								},
							},
						},
					},
				},
			},
		},
	}
	expectedProfile := DeviceProfileRcConfig{

		Profile: ProfileDefinition{
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Mapping: map[string]string{
								"1": "aa",
								"2": "bb",
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							Tags: map[string]string{
								"tag1": "$1",
								"tag2": "$2",
							},
						},
					},
				},
			},
		},
	}
	profile.NormalizeInplaceFromRc()
	assert.Equal(t, expectedProfile, profile)
}
