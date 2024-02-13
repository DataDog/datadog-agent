package aws

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseAWSRegion(t *testing.T) {
	tests := []struct {
		name             string
		availabilityZone string
		expectedRegion   string
		expectedErr      error
	}{
		{
			name:             "Valid availability zone",
			availabilityZone: "us-east-1a",
			expectedRegion:   "us-east-1",
			expectedErr:      nil,
		},
		{
			name:             "Invalid availability zone",
			availabilityZone: "invalid-zone",
			expectedRegion:   "",
			expectedErr:      errors.New("unable to parse AWS region from availability zone: invalid-zone"),
		},
		{
			name:             "Empty availability zone",
			availabilityZone: "",
			expectedRegion:   "",
			expectedErr:      errors.New("unable to parse AWS region from availability zone: "),
		},
		{
			name:             "Invalid availability zone format no number",
			availabilityZone: "us-west-b",
			expectedRegion:   "",
			expectedErr:      errors.New("unable to parse AWS region from availability zone: us-west-b"),
		},
		{
			name:             "Invalid availability zone format multiple letters",
			availabilityZone: "us-west-2bbb",
			expectedRegion:   "",
			expectedErr:      errors.New("unable to parse AWS region from availability zone: us-west-2bbb"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualRegion, err := parseAWSRegion(tt.availabilityZone)
			if tt.expectedErr != nil {
				require.EqualError(t, err, tt.expectedErr.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedRegion, actualRegion)
		})
	}
}
