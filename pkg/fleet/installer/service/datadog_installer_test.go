package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	insertedEnvs = "DD_APM_RECEIVER_SOCKET=/var/run/datadog/apm.socket\nDD_DOGSTATSD_SOCKET=/var/run/datadog/dsd.socket\n"
)

func TestSetEnvs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file doesn't exist",
			input:    "",
			expected: insertedEnvs,
		},
		{
			name:     "keep previous - missing newline",
			input:    "banana=true",
			expected: "banana=true\n" + insertedEnvs,
		},
		{
			name:     "keep previous - with newline",
			input:    "apple=false\nat=home\n",
			expected: "apple=false\nat=home\n" + insertedEnvs,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := setSocketEnvs([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(res))
		})
	}
}
