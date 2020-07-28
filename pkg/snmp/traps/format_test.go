// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatV2(t *testing.T) {
	c := Config{Port: GetPort(t), CommunityStrings: []string{"public"}}
	configure(t, c)
	err := StartServer()
	require.NoError(t, err)
	defer StopServer()

	params := sendTestV2Trap(t, c, "public")
	clientPort := parsePort(t, params.Conn.LocalAddr().String())

	p := receivePacket(t)

	data, err := FormatJSON(p)
	require.NoError(t, err)

	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", data["oid"])
	assert.NotNil(t, data["uptime"])

	vars, ok := data["variables"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, len(vars), 2)

	heartBeatRate := vars[0]
	assert.Equal(t, heartBeatRate["oid"], "1.3.6.1.4.1.8072.2.3.2.1")
	assert.Equal(t, heartBeatRate["type"], "integer")
	assert.Equal(t, heartBeatRate["value"], 1024)

	heartBeatName := vars[1]
	assert.Equal(t, heartBeatName["oid"], "1.3.6.1.4.1.8072.2.3.2.2")
	assert.Equal(t, heartBeatName["type"], "string")
	assert.Equal(t, heartBeatName["value"], "test")

	tags := GetTags(p)
	assert.Equal(t, tags, []string{
		"snmp_version:2",
		"community:public",
		"device_ip:127.0.0.1",
		fmt.Sprintf("device_port:%d", clientPort),
	})
}
