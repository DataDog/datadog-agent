package snmpintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func Test_PingConfig_UnmarshalYAML(t *testing.T) {
	trueVal := true
	falseVal := false
	twoVal := 2
	threeVal := 3
	fourVal := 4
	tests := []struct {
		name          string
		data          []byte
		result        PackedPingConfig
		expectedError string
	}{
		{
			name: "empty ping config",
			data: []byte(`
""
`),
			result: PackedPingConfig{},
		},
		{
			name: "ping config as yaml struct",
			data: []byte(`
enabled: true
linux:
  use_raw_socket: true
interval: 2
timeout: 3
count: 4
`),
			result: PackedPingConfig{
				Enabled: &trueVal,
				Linux: PingLinuxConfig{
					UseRawSocket: &trueVal,
				},
				Interval: &twoVal,
				Timeout:  &threeVal,
				Count:    &fourVal,
			},
		},
		{
			name: "ping config as json string all null",
			data: []byte(`
'{"linux":{"use_raw_socket":null},"enabled":null,"interval":null,"timeout":null,"count":null}'
`),
			result: PackedPingConfig{},
		},
		{
			name: "ping config as json string",
			data: []byte(`
'{"linux":{"use_raw_socket":false},"enabled":true,"interval":4,"timeout":2,"count":3}'
`),
			result: PackedPingConfig{
				Linux: PingLinuxConfig{
					UseRawSocket: &falseVal,
				},
				Enabled:  &trueVal,
				Interval: &fourVal,
				Timeout:  &twoVal,
				Count:    &threeVal,
			},
		},
		{
			name: "invalid json",
			data: []byte(`
'{'
`),
			result:        PackedPingConfig{},
			expectedError: "cannot unmarshal json to snmpintegration.PingConfig: unexpected end of JSON input",
		},
		{
			name: "invalid overall yaml",
			data: []byte(`
{
`),
			result:        PackedPingConfig{},
			expectedError: "yaml: line 2: did not find expected node content",
		},
		{
			name: "invalid ping yaml",
			data: []byte(`
[]
`),
			result:        PackedPingConfig{},
			expectedError: "cannot unmarshal to string: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!seq into string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			myStruct := PackedPingConfig{}
			err := yaml.Unmarshal(tt.data, &myStruct)
			assert.Equal(t, tt.result, myStruct)
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
