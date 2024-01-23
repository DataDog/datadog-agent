// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/config"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/formatter"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"
)

func TestStartFailure(t *testing.T) {
	/*
		Start two servers with the same config to trigger an "address already in use" error.
	*/
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())

	freePort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: freePort, CommunityStrings: []string{"public"}}

	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()
	status := status.NewMock()

	sucessServer, err := NewTrapServer(config, &formatter.DummyFormatter{}, mockSender, logger, status)
	require.NoError(t, err)
	require.NotNil(t, sucessServer)
	defer sucessServer.Stop()

	failedServer, err := NewTrapServer(config, &formatter.DummyFormatter{}, mockSender, logger, status)
	require.Nil(t, failedServer)
	require.Error(t, err)
}
