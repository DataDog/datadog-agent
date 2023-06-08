package traps

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetTags(t *testing.T) {
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	assert.Equal(t, packet.getTags(), []string{
		"snmp_version:2",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsSNMPV1(t *testing.T) {
	packet := createTestV1GenericPacket()
	assert.Equal(t, packet.getTags(), []string{
		"snmp_version:1",
		"device_namespace:the_baron",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	packet.Content.Version = 12
	assert.Equal(t, packet.getTags(), []string{
		"snmp_version:unknown",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	})
}
