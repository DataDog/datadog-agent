package report

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_formatValue(t *testing.T) {
	tests := []struct {
		name          string
		value         valuestore.ResultValue
		format        string
		expectedValue valuestore.ResultValue
	}{
		{
			name: "format mac address",
			value: valuestore.ResultValue{
				Value: []byte{0x82, 0xa5, 0x6e, 0xa5, 0xc8, 0x01},
			},
			format: "mac_address",
			expectedValue: valuestore.ResultValue{
				Value: "82:a5:6e:a5:c8:01",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedValue, formatValue(tt.value, tt.format))
		})
	}
}
