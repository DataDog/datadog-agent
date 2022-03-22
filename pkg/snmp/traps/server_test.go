// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartFailure(t *testing.T) {
	/*
		Start two servers with the same config to trigger an "address already in use" error.
	*/
	port := serverPort

	config := Config{Port: port, CommunityStrings: []string{"public"}}
	Configure(t, config)

	mockSender, _ := getMockSender(t)
	sucessServer, err := NewTrapServer("dummy_hostname", &DummyFormatter{}, mockSender)
	require.NoError(t, err)
	require.NotNil(t, sucessServer)
	defer sucessServer.Stop()

	mockSender, _ = getMockSender(t)
	failedServer, err := NewTrapServer("dummy_hostname", &DummyFormatter{}, mockSender)
	require.Nil(t, failedServer)
	require.Error(t, err)
}
