package checkconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_buildDeviceID(t *testing.T) {
	tests := []struct {
		name             string
		tags             []string
		expectedDeviceID string
		expectedIDTags   []string
	}{
		{
			name:             "one tag with only ip address",
			tags:             []string{"snmp_device:1.2.3.4"},
			expectedDeviceID: "74f22f3320d2d692",
			expectedIDTags:   []string{"snmp_device:1.2.3.4"},
		},
		{
			name:             "one tag with different ip address",
			tags:             []string{"snmp_device:1.2.3.5"},
			expectedDeviceID: "74f22f3320d2d693",
			expectedIDTags:   []string{"snmp_device:1.2.3.5"},
		},
		{
			name:             "one tag with another different ip address",
			tags:             []string{"snmp_device:2.2.3.4"},
			expectedDeviceID: "2ac7c48f271d6905",
			expectedIDTags:   []string{"snmp_device:2.2.3.4"},
		},
		{
			name:             "many tags",
			tags:             []string{"zoo", "snmp_device:1.2.3.4", "foo"},
			expectedDeviceID: "e413dc66fbaf23f4",
			expectedIDTags:   []string{"foo", "snmp_device:1.2.3.4", "zoo"}, // sorted tags
		},
		{
			name:             "many tags with duplicate",
			tags:             []string{"zoo", "snmp_device:1.2.3.4", "zoo", "foo"},
			expectedDeviceID: "e413dc66fbaf23f4",
			expectedIDTags:   []string{"foo", "snmp_device:1.2.3.4", "zoo"}, // sorted tags
		},
		{
			name:             "ignore autodiscovery_subnet prefix",
			tags:             []string{"zoo", "autodiscovery_subnet:127.0.0.0/29", "snmp_device:1.2.3.4", "zoo", "foo"},
			expectedDeviceID: "e413dc66fbaf23f4",
			expectedIDTags:   []string{"foo", "snmp_device:1.2.3.4", "zoo"}, // sorted tags
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceID, deviceIDTags := buildDeviceID(tt.tags)
			assert.Equal(t, deviceID, tt.expectedDeviceID)
			assert.Equal(t, deviceIDTags, tt.expectedIDTags)
		})
	}
}
