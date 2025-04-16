// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestDestinationHA(t *testing.T) {
	variants := []bool{true, false}
	for _, variant := range variants {
		endpoint := config.Endpoint{
			IsMRF: variant,
		}
		isEndpointMRF := endpoint.IsMRF

		dest := NewDestination(endpoint, false, client.NewDestinationsContext(), false, statusinterface.NewStatusProviderMock())
		isDestMRF := dest.IsMRF()

		assert.Equal(t, isEndpointMRF, isDestMRF)
	}
}

// TestConnecitivityDiagnoseNoBlock ensures the connectivity diagnose doesn't
// block
func TestConnecitivityDiagnoseNoBlock(t *testing.T) {
	endpoint := config.NewEndpoint("00000000", "", "host", 0, true)
	done := make(chan struct{})

	go func() {
		CheckConnectivityDiagnose(endpoint, 1)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Error("TCP diagnosis check blocked for too long.")
	}
}

// TestConnectivityDiagnoseFails ensures the connectivity diagnosis connects
// successfully
func TestConnectivityDiagnoseOperationSuccess(t *testing.T) {
	// Start the test TCP server
	intake := mock.NewMockLogsIntake(t)
	serverAddr := intake.Addr().String()

	// Simulate a client connecting to the server
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to connect to test TCP server: %v", err)
	}
	defer conn.Close()

	host, port, err := net.SplitHostPort(serverAddr)
	assert.Nil(t, err)
	portInt, err := strconv.Atoi(port)
	assert.Nil(t, err)

	testSuccessEndpoint := config.NewEndpoint("api-key", "", host, portInt, false)
	connManager := NewConnectionManager(testSuccessEndpoint, statusinterface.NewNoopStatusProvider())
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = connManager.NewConnection(ctx)
	assert.Nil(t, err)
}

// TestConnectivityDiagnoseOperationFail ensure the connectivity diagnosis fails
// when provided with incorrect information
func TestConnectivityDiagnoseOperationFail(t *testing.T) {
	// Start the test TCP server
	intake := mock.NewMockLogsIntake(t)
	serverAddr := intake.Addr().String()

	// Simulate a client connecting to the server
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to connect to test TCP server: %v", err)
	}
	defer conn.Close()

	host, port, err := net.SplitHostPort(serverAddr)
	assert.Nil(t, err)
	portInt, err := strconv.Atoi(port)
	assert.Nil(t, err)

	testFailEndpointWrongAddress := config.NewEndpoint("api-key", "", "failhost", portInt, false)
	connManager := NewConnectionManager(testFailEndpointWrongAddress, statusinterface.NewNoopStatusProvider())
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = connManager.NewConnection(ctx)
	assert.NotNil(t, err)

	testFailEndpointWrongPort := config.NewEndpoint("api-key", "", host, portInt+1, false)
	connManager = NewConnectionManager(testFailEndpointWrongPort, statusinterface.NewNoopStatusProvider())
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = connManager.NewConnection(ctx)
	assert.NotNil(t, err)
}
