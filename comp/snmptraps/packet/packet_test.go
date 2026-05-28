// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package packet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTags(t *testing.T) {
	packet := CreateTestPacket(NetSNMPExampleHeartbeatNotification)
	assert.Equal(t, packet.GetTags(), []string{
		"snmp_version:2",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsSNMPV1(t *testing.T) {
	packet := CreateTestV1GenericPacket()
	assert.Equal(t, packet.GetTags(), []string{
		"snmp_version:1",
		"device_namespace:the_baron",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := CreateTestPacket(NetSNMPExampleHeartbeatNotification)
	packet.Content.Version = 12
	assert.Equal(t, packet.GetTags(), []string{
		"snmp_version:unknown",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	})
}

// TestGetTagsWithCustomTags asserts that user-supplied tags are appended
// after the three built-in tags, in their declared order.
func TestGetTagsWithCustomTags(t *testing.T) {
	packet := CreateTestPacket(NetSNMPExampleHeartbeatNotification)
	packet.Tags = []string{"application:my-app", "team:netops"}
	assert.Equal(t, []string{
		"snmp_version:2",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
		"application:my-app",
		"team:netops",
	}, packet.GetTags())
}

// TestGetTagsWithCustomTagsSNMPV1 mirrors TestGetTagsWithCustomTags for v1
// packets — both formatV1Trap and formatTrap pull through GetTags().
func TestGetTagsWithCustomTagsSNMPV1(t *testing.T) {
	packet := CreateTestV1GenericPacket()
	packet.Tags = []string{"application:my-app"}
	assert.Equal(t, []string{
		"snmp_version:1",
		"device_namespace:the_baron",
		"snmp_device:127.0.0.1",
		"application:my-app",
	}, packet.GetTags())
}

// TestGetTagsWithEmptyCustomTags asserts that an empty/nil Tags slice does not
// alter the existing three-tag output (success criterion #4 in the PRD).
func TestGetTagsWithEmptyCustomTags(t *testing.T) {
	packet := CreateTestPacket(NetSNMPExampleHeartbeatNotification)
	packet.Tags = []string{}
	assert.Equal(t, []string{
		"snmp_version:2",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	}, packet.GetTags())

	packet.Tags = nil
	assert.Equal(t, []string{
		"snmp_version:2",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	}, packet.GetTags())
}
