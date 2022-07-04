package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetAllFlowTypes(t *testing.T) {
	expectedFlowTypes := []FlowType{
		"ipfix",
		"sflow5",
		"netflow5",
		"netflow9",
	}
	assert.ElementsMatch(t, expectedFlowTypes, GetAllFlowTypes())
}

func TestGetFlowTypeByName(t *testing.T) {
	tests := []struct {
		flowTypeName           FlowType
		expectedFlowTypeDetail FlowTypeDetail
		expectedError          string
	}{
		{
			flowTypeName: TypeIPFIX,
			expectedFlowTypeDetail: FlowTypeDetail{
				name:        TypeIPFIX,
				defaultPort: uint16(4739),
			},
		},
		{
			flowTypeName: TypeSFlow5,
			expectedFlowTypeDetail: FlowTypeDetail{
				name:        TypeSFlow5,
				defaultPort: uint16(6343),
			},
		},
		{
			flowTypeName: TypeNetFlow5,
			expectedFlowTypeDetail: FlowTypeDetail{
				name:        TypeNetFlow5,
				defaultPort: uint16(2055),
			},
		},
		{
			flowTypeName: TypeNetFlow9,
			expectedFlowTypeDetail: FlowTypeDetail{
				name:        TypeNetFlow9,
				defaultPort: uint16(2055),
			},
		},
		{
			flowTypeName:           "invalidFlowType",
			expectedFlowTypeDetail: FlowTypeDetail{},
			expectedError:          "flow type `invalidFlowType` is not valid",
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.flowTypeName), func(t *testing.T) {
			detail, err := GetFlowTypeByName(tt.flowTypeName)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedFlowTypeDetail, detail)

		})
	}
}
