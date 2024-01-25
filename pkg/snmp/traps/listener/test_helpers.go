// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !serverless && test

package listener

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp/traps/config"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/packet"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sendTestV1GenericTrap(t *testing.T, trapConfig *config.TrapsConfig, community string) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams(nil)
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.
	params.Version = gosnmp.Version1

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	_, err = params.SendTrap(packet.LinkDownv1GenericTrap)
	require.NoError(t, err)

	return params
}

func sendTestV1SpecificTrap(t *testing.T, trapConfig *config.TrapsConfig, community string) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams(nil)
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.
	params.Version = gosnmp.Version1

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	_, err = params.SendTrap(packet.AlarmActiveStatev1SpecificTrap)
	require.NoError(t, err)

	return params
}

func sendTestV2Trap(t *testing.T, trapConfig *config.TrapsConfig, community string) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams(nil)
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	trap := packet.NetSNMPExampleHeartbeatNotification
	_, err = params.SendTrap(trap)
	require.NoError(t, err)

	return params
}

func sendTestV3Trap(t *testing.T, trapConfig *config.TrapsConfig, msgFlags gosnmp.SnmpV3MsgFlags, securityParams *gosnmp.UsmSecurityParameters) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams(nil)
	require.NoError(t, err)
	params.MsgFlags = msgFlags
	params.SecurityParameters = securityParams
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	trap := packet.NetSNMPExampleHeartbeatNotification
	_, err = params.SendTrap(trap)
	require.NoError(t, err)

	return params
}

func assertIsValidV2Packet(t *testing.T, packet *packet.SnmpPacket, trapConfig *config.TrapsConfig) {
	require.Equal(t, gosnmp.Version2c, packet.Content.Version)
	communityValid := false
	for _, community := range trapConfig.CommunityStrings {
		if packet.Content.Community == community {
			communityValid = true
		}
	}
	require.True(t, communityValid)
}

func assertVariables(t *testing.T, packet *packet.SnmpPacket) {
	variables := packet.Content.Variables
	assert.Equal(t, 4, len(variables))

	sysUptimeInstance := variables[0]
	assert.Equal(t, ".1.3.6.1.2.1.1.3.0", sysUptimeInstance.Name)
	assert.Equal(t, gosnmp.TimeTicks, sysUptimeInstance.Type)

	snmptrapOID := variables[1]
	assert.Equal(t, ".1.3.6.1.6.3.1.1.4.1.0", snmptrapOID.Name)
	assert.Equal(t, gosnmp.OctetString, snmptrapOID.Type)
	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", string(snmptrapOID.Value.([]byte)))

	heartBeatRate := variables[2]
	assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.1", heartBeatRate.Name)
	assert.Equal(t, gosnmp.Integer, heartBeatRate.Type)
	assert.Equal(t, 1024, heartBeatRate.Value.(int))

	heartBeatName := variables[3]
	assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.2", heartBeatName.Name)
	assert.Equal(t, gosnmp.OctetString, heartBeatName.Type)
	assert.Equal(t, "test", string(heartBeatName.Value.([]byte)))
}
