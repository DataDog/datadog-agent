// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestWalkCommand(t *testing.T) {
	// this command has _lots_ of options, so the test just exercises a few
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"snmp", "walk", "1.2.3.4", "10.9.8.7", "-v", "3", "-r", "10"},
		snmpWalk,
		func(cliParams *snmpConnectionParams, args argsType) {
			require.Equal(t, argsType{"1.2.3.4", "10.9.8.7"}, args)
			require.Equal(t, "3", cliParams.Version)
			require.Equal(t, 10, cliParams.Retries)
			require.False(t, cliParams.UseUnconnectedUDPSocket)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"snmp", "walk", "1.2.3.4", "10.9.8.7", "--use-unconnected-udp-socket"},
		snmpWalk,
		func(cliParams *snmpConnectionParams, args argsType) {
			require.Equal(t, argsType{"1.2.3.4", "10.9.8.7"}, args)
			require.True(t, cliParams.UseUnconnectedUDPSocket)
		})
}

func TestScanCommand(t *testing.T) {
	// this command has _lots_ of options, so the test just exercises a few
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"snmp", "scan", "1.2.3.4", "-v", "3", "-r", "10"},
		scanDevice,
		func(cliParams *snmpConnectionParams, args argsType) {
			require.Equal(t, argsType{"1.2.3.4"}, args)
			require.Equal(t, "3", cliParams.Version)
			require.Equal(t, 10, cliParams.Retries)
			require.False(t, cliParams.UseUnconnectedUDPSocket)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"snmp", "scan", "1.2.3.4", "--use-unconnected-udp-socket"},
		scanDevice,
		func(cliParams *snmpConnectionParams, args argsType) {
			require.Equal(t, argsType{"1.2.3.4"}, args)
			require.True(t, cliParams.UseUnconnectedUDPSocket)
		})
}

func TestSplitIP(t *testing.T) {
	for _, tc := range []struct {
		addr    string
		host    string
		port    uint16
		hasPort bool
	}{
		{"127.0.0.1", "127.0.0.1", 0, false},
		{"127.0.0.1:60", "127.0.0.1", 60, true},
		{"::1", "::1", 0, false}, // IPv6
		{"::1:60", "::1:60", 0, false},
		{"[::1]:60", "::1", 60, true},
		{"localhost:60", "localhost", 60, true},
		{"[localhost]:60", "localhost", 60, true},
		{"[some:weird:name]:60", "some:weird:name", 60, true},
		{"not-an-ip:10", "not-an-ip", 10, true},
		{"127.0.0.1:badport", "127.0.0.1:badport", 0, false},
		{"localhost:65536", "localhost:65536", 0, false},
	} {
		host, port, hasPort := maybeSplitIP(tc.addr)
		assert.Equal(t, tc.host, host)
		assert.Equal(t, tc.port, port)
		assert.Equal(t, tc.hasPort, hasPort)
	}
}
