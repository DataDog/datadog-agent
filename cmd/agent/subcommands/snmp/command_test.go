// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	// this command has _lots_ of options, so the test just exercises a few
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"snmp", "walk", "1.2.3.4", "10.9.8.7", "-v", "3", "-r", "10"},
		snmpwalk,
		func(cliParams *cliParams) {
			require.Equal(t, []string{"1.2.3.4", "10.9.8.7"}, cliParams.args)
			require.Equal(t, "3", cliParams.snmpVersion)
			require.Equal(t, 10, cliParams.retries)
			require.False(t, cliParams.unconnectedUDPSocket)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"snmp", "walk", "1.2.3.4", "10.9.8.7", "--use-unconnected-udp-socket"},
		snmpwalk,
		func(cliParams *cliParams) {
			require.Equal(t, []string{"1.2.3.4", "10.9.8.7"}, cliParams.args)
			require.True(t, cliParams.unconnectedUDPSocket)
		})
}
