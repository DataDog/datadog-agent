// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func TestToFloat64FromString(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: profiledefinition.ProfileMetricTypeGauge,
		Value:          "255.745",
	}
	value, err := snmpValue.ToFloat64()
	assert.NoError(t, err)
	assert.Equal(t, float64(255.745), value)
}

func TestToFloat64FromFloat(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: profiledefinition.ProfileMetricTypeGauge,
		Value:          float64(255.745),
	}
	value, err := snmpValue.ToFloat64()
	assert.NoError(t, err)
	assert.Equal(t, float64(255.745), value)
}

func TestToFloat64FromInvalidType(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: profiledefinition.ProfileMetricTypeGauge,
		Value:          int64(255),
	}
	_, err := snmpValue.ToFloat64()
	assert.NotNil(t, err)
}

func TestToFloat64FromBytes(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: profiledefinition.ProfileMetricTypeGauge,
		Value:          []byte("123.456"),
	}
	value, err := snmpValue.ToFloat64()
	assert.NoError(t, err)
	assert.Equal(t, float64(123.456), value)
}

func TestToFloat64FromUnparseableString(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: profiledefinition.ProfileMetricTypeGauge,
		Value:          "not-a-number",
	}
	_, err := snmpValue.ToFloat64()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestExtractStringValue(t *testing.T) {
	t.Run("matching pattern", func(t *testing.T) {
		sv := ResultValue{
			SubmissionType: profiledefinition.ProfileMetricTypeGauge,
			Value:          "version: 1.2.3-beta",
		}
		pattern := regexp.MustCompile(`version: (\S+)`)
		result, err := sv.ExtractStringValue(pattern)
		assert.NoError(t, err)
		assert.Equal(t, "1.2.3-beta", result.Value)
		assert.Equal(t, profiledefinition.ProfileMetricTypeGauge, result.SubmissionType)
	})

	t.Run("no match", func(t *testing.T) {
		sv := ResultValue{Value: "no match here"}
		pattern := regexp.MustCompile(`(xyz)`)
		_, err := sv.ExtractStringValue(pattern)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not match")
	})

	t.Run("no capture group", func(t *testing.T) {
		sv := ResultValue{Value: "test123"}
		pattern := regexp.MustCompile(`test123`)
		_, err := sv.ExtractStringValue(pattern)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "matching group")
	})

	t.Run("non-string type passes through", func(t *testing.T) {
		sv := ResultValue{Value: float64(42)}
		pattern := regexp.MustCompile(`(.+)`)
		result, err := sv.ExtractStringValue(pattern)
		assert.NoError(t, err)
		assert.Equal(t, float64(42), result.Value)
	})

	t.Run("bytes value", func(t *testing.T) {
		sv := ResultValue{Value: []byte("firmware v2.5")}
		pattern := regexp.MustCompile(`v(\d+\.\d+)`)
		result, err := sv.ExtractStringValue(pattern)
		assert.NoError(t, err)
		assert.Equal(t, "2.5", result.Value)
	})
}

func TestResultValue_ToString(t *testing.T) {
	tests := []struct {
		name          string
		resultValue   ResultValue
		expectedStr   string
		expectedError string
	}{
		{
			name: "hexify",
			resultValue: ResultValue{
				Value: []byte{0xff, 0xaa, 0x00},
			},
			expectedStr:   "0xffaa00",
			expectedError: "",
		},
		{
			name: "do not hexify newline and tabs",
			resultValue: ResultValue{
				Value: []byte(`m\ny\rV\ta\n\r\tl`),
			},
			expectedStr:   "m\\ny\\rV\\ta\\n\\r\\tl",
			expectedError: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strValue, err := tt.resultValue.ToString()
			if tt.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), tt.expectedError)
			}
			assert.Equal(t, tt.expectedStr, strValue)
		})
	}
}
