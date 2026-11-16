// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func TestTestConnection(t *testing.T) {
	rack := "us-east-1a"
	client := &fakeAdminClient{metadata: kadm.Metadata{
		Cluster:    "cluster-id",
		Controller: 1,
		Brokers: kadm.BrokerDetails{
			{NodeID: 1, Host: "broker-1", Port: 9093, Rack: &rack},
			{NodeID: 2, Host: "broker-2", Port: 9093},
		},
	}}
	handler := &TestConnectionHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("testConnection", nil), nil)
	require.NoError(t, err)
	assert.Equal(t, &TestConnectionOutputs{
		ConfigurationValid: true,
		ConnectionValid:    true,
		ClusterInfo: &ClusterInfo{
			ClusterID:    "cluster-id",
			ControllerID: 1,
			Brokers: []Broker{
				{NodeID: 1, Host: "broker-1", Port: 9093, Rack: "us-east-1a"},
				{NodeID: 2, Host: "broker-2", Port: 9093},
			},
		},
	}, output)
	assert.True(t, client.closed)
}

func TestTestConnectionReportsUnreachableBrokers(t *testing.T) {
	client := &fakeAdminClient{metadataErr: errors.New("unable to dial: connection refused")}
	handler := &TestConnectionHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("testConnection", nil), nil)
	require.NoError(t, err)
	assert.Equal(t, &TestConnectionOutputs{
		ConfigurationValid: true,
		ConnectionValid:    false,
		Errors:             []string{"Unable to connect to Kafka: unable to dial: connection refused"},
	}, output)
}

func TestTestConnectionReportsInvalidConfiguration(t *testing.T) {
	handler := NewTestConnectionHandler()
	credential := &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: bootstrapServersToken, Value: "broker-1:9093"},
			{Name: saslMechanismToken, Value: "OAUTHBEARER"},
			{Name: usernameToken, Value: "user"},
			{Name: passwordToken, Value: "secret"},
		},
	}

	output, err := handler.Run(context.Background(), newTestTask("testConnection", nil), credential)
	require.NoError(t, err)
	outputs := output.(*TestConnectionOutputs)
	assert.False(t, outputs.ConfigurationValid)
	assert.False(t, outputs.ConnectionValid)
	require.Len(t, outputs.Errors, 1)
	assert.Contains(t, outputs.Errors[0], `invalid saslMechanism "OAUTHBEARER"`)
}
