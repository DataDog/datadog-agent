// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"testing"

	"go.yaml.in/yaml/v2"

	"github.com/stretchr/testify/assert"
)

type MyStringArray struct {
	SomeIDs StringArray `yaml:"my_field"`
}

type MySymbolStruct struct {
	SymbolField SymbolConfigCompat `yaml:"my_symbol_field"`
}

type MyMetrics struct {
	SomeMetrics []MetricsConfig `yaml:"metrics"`
}

func Test_metricTagConfig_UnmarshalYAML(t *testing.T) {
	myStruct := MetricsConfig{}
	expected := MetricsConfig{MetricTags: []MetricTagConfig{{Index: 3}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- index: 3
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func Test_metricTagConfig_onlyTags(t *testing.T) {
	myStruct := MetricsConfig{}
	expected := MetricsConfig{MetricTags: []MetricTagConfig{{SymbolTag: "aaa"}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestStringArray_UnmarshalYAML_array(t *testing.T) {
	myStruct := MyStringArray{}
	expected := MyStringArray{SomeIDs: StringArray{"aaa", "bbb"}}

	yaml.Unmarshal([]byte(`
my_field:
 - aaa
 - bbb
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestStringArray_UnmarshalYAML_string(t *testing.T) {
	myStruct := MyStringArray{}
	expected := MyStringArray{SomeIDs: StringArray{"aaa"}}

	yaml.Unmarshal([]byte(`
my_field: aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestSymbolConfig_UnmarshalYAML_symbolObject(t *testing.T) {
	myStruct := MySymbolStruct{}
	expected := MySymbolStruct{SymbolField: SymbolConfigCompat{OID: "1.2.3", Name: "aSymbol"}}

	yaml.Unmarshal([]byte(`
my_symbol_field:
  name: aSymbol
  OID: 1.2.3
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestSymbolConfig_UnmarshalYAML_symbolString(t *testing.T) {
	myStruct := MySymbolStruct{}
	expected := MySymbolStruct{SymbolField: SymbolConfigCompat{Name: "aSymbol"}}

	yaml.Unmarshal([]byte(`
my_symbol_field: aSymbol
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func Test_MetricsConfigs_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		result         MyMetrics
		expectedErrors []string
	}{
		{
			name: "ok unmarshal",
			data: []byte(`
metrics:
  - MIB: FOO-MIB
    OID: 1.2.3.4
    name: fooName

  - MIB: FOO-MIB
    symbol:
      OID: 1.2.3.4
      name: fooName

  - MIB: FOO-MIB
    table:
      OID: 1.2.3.4
      name: fooTable
    symbols:
      - OID: 1.2.3.4.1
        name: fooName1
      - OID: 1.2.3.4.2
        name: fooName2
`),
			result: MyMetrics{[]MetricsConfig{
				{
					MIB:  "FOO-MIB",
					OID:  "1.2.3.4",
					Name: "fooName",
				},
				{
					MIB: "FOO-MIB",
					Symbol: SymbolConfig{
						OID:  "1.2.3.4",
						Name: "fooName",
					},
				},
				{
					MIB: "FOO-MIB",
					Table: SymbolConfig{
						OID:  "1.2.3.4",
						Name: "fooTable",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3.4.1",
							Name: "fooName1",
						},
						{
							OID:  "1.2.3.4.2",
							Name: "fooName2",
						},
					},
				},
			}},
		},
		{
			name: "symbol declared in the legacy way with MIB specified",
			data: []byte(`
metrics:
  - MIB: FOO-MIB
    OID: 1.2.3.4
    symbol: fooName
`),
			expectedErrors: []string{
				"line 5: cannot unmarshal !!str `fooName` into profiledefinition.SymbolConfig",
				"legacy symbol type 'string' is not supported with the Core loader",
			},
		},
		{
			name: "symbol declared in the legacy way without MIB specified",
			data: []byte(`
metrics:
  - OID: 1.2.3.4
    symbol: fooName
`),
			expectedErrors: []string{
				"line 4: cannot unmarshal !!str `fooName` into profiledefinition.SymbolConfig",
			},
		},
		{
			name: "symbol declared in the legacy way with MIB empty",
			data: []byte(`
metrics:
  - MIB:
    OID: 1.2.3.4
    symbol: fooName
`),
			expectedErrors: []string{
				"line 5: cannot unmarshal !!str `fooName` into profiledefinition.SymbolConfig",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			myStruct := MyMetrics{}
			err := yaml.Unmarshal(tt.data, &myStruct)
			if len(tt.expectedErrors) > 0 {
				for _, expectedError := range tt.expectedErrors {
					assert.ErrorContains(t, err, expectedError)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.result, myStruct)
			}
		})
	}
}
