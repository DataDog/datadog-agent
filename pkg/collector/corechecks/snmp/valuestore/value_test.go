package valuestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFloat64FromString(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: "gauge",
		Value:          "255.745",
	}
	value, err := snmpValue.ToFloat64()
	assert.NoError(t, err)
	assert.Equal(t, float64(255.745), value)
}

func TestToFloat64FromFloat(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: "gauge",
		Value:          float64(255.745),
	}
	value, err := snmpValue.ToFloat64()
	assert.NoError(t, err)
	assert.Equal(t, float64(255.745), value)
}

func TestToFloat64FromInvalidType(t *testing.T) {
	snmpValue := &ResultValue{
		SubmissionType: "gauge",
		Value:          int64(255),
	}
	_, err := snmpValue.ToFloat64()
	assert.NotNil(t, err)
}
