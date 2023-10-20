// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_mergeProfileDefinition(t *testing.T) {
	okBaseDefinition := profiledefinition.ProfileDefinition{
		Metrics: []profiledefinition.MetricsConfig{
			{Symbol: profiledefinition.SymbolConfig{OID: "1.1", Name: "metric1"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
		},
		MetricTags: []profiledefinition.MetricTagConfig{
			{
				Tag:    "tag1",
				Symbol: profiledefinition.SymbolConfigCompat{OID: "2.1", Name: "tagName1"},
			},
		},
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"vendor": {
						Value: "f5",
					},
					"description": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.1.0",
							Name: "sysDescr",
						},
					},
				},
			},
			"interface": {
				Fields: map[string]profiledefinition.MetadataField{
					"admin_status": {
						Symbol: profiledefinition.SymbolConfig{

							OID:  "1.3.6.1.2.1.2.2.1.7",
							Name: "ifAdminStatus",
						},
					},
				},
				IDTags: profiledefinition.MetricTagConfigList{
					{
						Tag: "alias",
						Symbol: profiledefinition.SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifAlias",
						},
					},
				},
			},
		},
	}
	emptyBaseDefinition := profiledefinition.ProfileDefinition{}
	okTargetDefinition := profiledefinition.ProfileDefinition{
		Metrics: []profiledefinition.MetricsConfig{
			{Symbol: profiledefinition.SymbolConfig{OID: "1.2", Name: "metric2"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
		},
		MetricTags: []profiledefinition.MetricTagConfig{
			{
				Tag:    "tag2",
				Symbol: profiledefinition.SymbolConfigCompat{OID: "2.2", Name: "tagName2"},
			},
		},
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.5.0",
							Name: "sysName",
						},
					},
				},
			},
			"interface": {
				Fields: map[string]profiledefinition.MetadataField{
					"oper_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.8",
							Name: "ifOperStatus",
						},
					},
				},
				IDTags: profiledefinition.MetricTagConfigList{
					{
						Tag: "interface",
						Symbol: profiledefinition.SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifName",
						},
					},
				},
			},
		},
	}
	tests := []struct {
		name               string
		targetDefinition   profiledefinition.ProfileDefinition
		baseDefinition     profiledefinition.ProfileDefinition
		expectedDefinition profiledefinition.ProfileDefinition
	}{
		{
			name:             "merge case",
			baseDefinition:   CopyProfileDefinition(okBaseDefinition),
			targetDefinition: CopyProfileDefinition(okTargetDefinition),
			expectedDefinition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2", Name: "metric2"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
					{Symbol: profiledefinition.SymbolConfig{OID: "1.1", Name: "metric1"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Tag:    "tag2",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.2", Name: "tagName2"},
					},
					{
						Tag:    "tag1",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.1", Name: "tagName1"},
					},
				},
				Metadata: profiledefinition.MetadataConfig{
					"device": {
						Fields: map[string]profiledefinition.MetadataField{
							"vendor": {
								Value: "f5",
							},
							"name": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
							"description": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]profiledefinition.MetadataField{
							"oper_status": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
							"admin_status": {
								Symbol: profiledefinition.SymbolConfig{

									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
						},
						IDTags: profiledefinition.MetricTagConfigList{
							{
								Tag: "interface",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
							{
								Tag: "alias",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifAlias",
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "empty base definition",
			baseDefinition:   CopyProfileDefinition(emptyBaseDefinition),
			targetDefinition: CopyProfileDefinition(okTargetDefinition),
			expectedDefinition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2", Name: "metric2"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Tag:    "tag2",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.2", Name: "tagName2"},
					},
				},
				Metadata: profiledefinition.MetadataConfig{
					"device": {
						Fields: map[string]profiledefinition.MetadataField{
							"name": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]profiledefinition.MetadataField{
							"oper_status": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
						},
						IDTags: profiledefinition.MetricTagConfigList{
							{
								Tag: "interface",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "empty taget definition",
			baseDefinition:   CopyProfileDefinition(okBaseDefinition),
			targetDefinition: CopyProfileDefinition(emptyBaseDefinition),
			expectedDefinition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.1", Name: "metric1"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Tag:    "tag1",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.1", Name: "tagName1"},
					},
				},
				Metadata: profiledefinition.MetadataConfig{
					"device": {
						Fields: map[string]profiledefinition.MetadataField{
							"vendor": {
								Value: "f5",
							},
							"description": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]profiledefinition.MetadataField{
							"admin_status": {
								Symbol: profiledefinition.SymbolConfig{

									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
						},
						IDTags: profiledefinition.MetricTagConfigList{
							{
								Tag: "alias",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifAlias",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeProfileDefinition(&tt.targetDefinition, &tt.baseDefinition)
			assert.Equal(t, tt.expectedDefinition.Metrics, tt.targetDefinition.Metrics)
			assert.Equal(t, tt.expectedDefinition.MetricTags, tt.targetDefinition.MetricTags)
			assert.Equal(t, tt.expectedDefinition.Metadata, tt.targetDefinition.Metadata)
		})
	}
}
