// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
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
