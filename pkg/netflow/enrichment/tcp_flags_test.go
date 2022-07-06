package enrichment

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFormatFCPFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         uint32
		expectedFlags []string
	}{
		{
			name:          "no flag",
			flags:         uint32(0),
			expectedFlags: nil,
		},
		{
			name:          "FIN",
			flags:         uint32(1),
			expectedFlags: []string{"FIN"},
		},
		{
			name:          "SYN",
			flags:         uint32(2),
			expectedFlags: []string{"SYN"},
		},
		{
			name:          "RST",
			flags:         uint32(4),
			expectedFlags: []string{"RST"},
		},
		{
			name:          "PSH",
			flags:         uint32(8),
			expectedFlags: []string{"PSH"},
		},
		{
			name:          "ACK",
			flags:         uint32(16),
			expectedFlags: []string{"ACK"},
		},
		{
			name:          "URG",
			flags:         uint32(32),
			expectedFlags: []string{"URG"},
		},
		{
			name:          "FIN SYN ACK",
			flags:         uint32(19),
			expectedFlags: []string{"FIN", "SYN", "ACK"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualFlags := FormatFCPFlags(tt.flags)
			assert.Equal(t, tt.expectedFlags, actualFlags)
		})
	}
}
