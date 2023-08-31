// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

var freePort = getFreePort()

func TestStartFailure(t *testing.T) {
	/*
		Start two servers with the same config to trigger an "address already in use" error.
	*/

	config := Config{Port: freePort, CommunityStrings: []string{"public"}}

	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()

	sucessServer, err := NewTrapServer(config, &DummyFormatter{}, mockSender)
	require.NoError(t, err)
	require.NotNil(t, sucessServer)
	defer sucessServer.Stop()

	failedServer, err := NewTrapServer(config, &DummyFormatter{}, mockSender)
	require.Nil(t, failedServer)
	require.Error(t, err)
}
