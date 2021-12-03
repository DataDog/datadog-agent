package report

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
)

func Test_getScalarValueFromSymbol(t *testing.T) {
	mockValues := &valuestore.ResultValueStore{
		ScalarValues: map[string]valuestore.ResultValue{
			"1.2.3.4": {Value: "value1"},
		},
	}

	tests := []struct {
		name          string
		values        *valuestore.ResultValueStore
		symbol        checkconfig.SymbolConfig
		expectedValue valuestore.ResultValue
		expectedError string
	}{
		{
			name:   "OK oid value case",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{OID: "1.2.3.4", Name: "mySymbol"},
			expectedValue: valuestore.ResultValue{
				Value: "value1",
			},
			expectedError: "",
		},
		{
			name:          "not found",
			values:        mockValues,
			symbol:        checkconfig.SymbolConfig{OID: "1.2.3.99", Name: "mySymbol"},
			expectedValue: valuestore.ResultValue{},
			expectedError: "value for Scalar OID `1.2.3.99` not found in results",
		},
		{
			name:   "extract value pattern error",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				ExtractValue:         "abc",
				ExtractValueCompiled: regexp.MustCompile("abc"),
			},
			expectedValue: valuestore.ResultValue{},
			expectedError: "extract value extractValuePattern does not match (extractValuePattern=abc, srcValue=value1)",
		},
		{
			name:   "OK match pattern without replace",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				MatchPatternCompiled: regexp.MustCompile("value\\d"),
				MatchValue:           "matched-value-with-digit",
			},
			expectedValue: valuestore.ResultValue{
				Value: "matched-value-with-digit",
			},
			expectedError: "",
		},
		{
			name:   "Error match pattern does not match",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				MatchPattern:         "doesNotMatch",
				MatchPatternCompiled: regexp.MustCompile("doesNotMatch"),
				MatchValue:           "noMatch",
			},
			expectedValue: valuestore.ResultValue{},
			expectedError: "match pattern `doesNotMatch` does not match string `value1`",
		},
		{
			name:   "Error match pattern template does not match",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				MatchPattern:         "value(\\d)",
				MatchPatternCompiled: regexp.MustCompile("value(\\d)"),
				MatchValue:           "$2",
			},
			expectedValue: valuestore.ResultValue{},
			expectedError: "the pattern `value(\\d)` matched value `value1`, but template `$2` is not compatible",
		},
		{
			name:   "OK Extract value case",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				ExtractValue:         "[a-z]+(\\d)",
				ExtractValueCompiled: regexp.MustCompile("[a-z]+(\\d)"),
			},
			expectedValue: valuestore.ResultValue{
				Value: "1",
			},
			expectedError: "",
		},
		{
			name:   "Error extract value pattern des not contain any matching group",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				ExtractValue:         "[a-z]+\\d",
				ExtractValueCompiled: regexp.MustCompile("[a-z]+\\d"),
			},
			expectedValue: valuestore.ResultValue{},
			expectedError: "extract value pattern des not contain any matching group (extractValuePattern=[a-z]+\\d, srcValue=value1)",
		},
		{
			name:   "Error extract value extractValuePattern does not match",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				ExtractValue:         "[a-z]+(\\d)",
				ExtractValueCompiled: regexp.MustCompile("doesNotMatch"),
			},
			expectedValue: valuestore.ResultValue{},
			expectedError: "extract value extractValuePattern does not match (extractValuePattern=doesNotMatch, srcValue=value1)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualValues, err := getScalarValueFromSymbol(tt.values, tt.symbol)
			if err != nil || tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			}
			assert.Equal(t, tt.expectedValue, actualValues)
		})
	}
}

func Test_getColumnValueFromSymbol(t *testing.T) {
	mockValues := &valuestore.ResultValueStore{
		ColumnValues: map[string]map[string]valuestore.ResultValue{
			"1.2.3.4": {
				"1": valuestore.ResultValue{Value: "value1"},
				"2": valuestore.ResultValue{Value: "value2"},
			},
		},
	}

	tests := []struct {
		name           string
		values         *valuestore.ResultValueStore
		symbol         checkconfig.SymbolConfig
		expectedValues map[string]valuestore.ResultValue
		expectedError  string
	}{
		{
			name:   "valid case",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{OID: "1.2.3.4", Name: "mySymbol"},
			expectedValues: map[string]valuestore.ResultValue{
				"1": {Value: "value1"},
				"2": {Value: "value2"},
			},
			expectedError: "",
		},
		{
			name:           "value not found",
			values:         mockValues,
			symbol:         checkconfig.SymbolConfig{OID: "1.2.3.99", Name: "mySymbol"},
			expectedValues: nil,
			expectedError:  "value for Column OID `1.2.3.99` not found in results",
		},
		{
			name:   "invalid extract value pattern",
			values: mockValues,
			symbol: checkconfig.SymbolConfig{
				OID:                  "1.2.3.4",
				Name:                 "mySymbol",
				ExtractValue:         "abc",
				ExtractValueCompiled: regexp.MustCompile("abc"),
			},
			expectedValues: make(map[string]valuestore.ResultValue),
			expectedError:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualValues, err := getColumnValueFromSymbol(tt.values, tt.symbol)
			if err != nil || tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			}
			assert.Equal(t, tt.expectedValues, actualValues)
		})
	}
}
