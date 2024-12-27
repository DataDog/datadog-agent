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
	endpoint := config.NewEndpoint("00000000", "host", 0, true)
	done := make(chan struct{})

	go func() {
		CheckConnectivityDiagnose(endpoint)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Error("TCP diagnosis check blocked for too long.")
	}
}

func StartTestTCPServer(t *testing.T) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.Nil(t, err)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if _, ok := err.(net.Error); ok {
					continue
				}
				break // Exit the loop if the listener is closed
			}

			defer conn.Close()
		}
	}()

	// Return the server address and a cleanup function
	return listener.Addr().String(), func() {
		_ = listener.Close()
	}
}

// TestConnectivityDiagnoseFails ensures the connectivity diagnosis operates
// correctly
func TestConnectivityDiagnoseOperation(t *testing.T) {
	// Start the test TCP server
	serverAddr, cleanup := StartTestTCPServer(t)
	defer cleanup()

	// Simulate a client connecting to the server
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to connect to test TCP server: %v", err)
	}
	defer conn.Close()

	testFailEndpoint := config.NewEndpoint("api-key", "failhost", 1234, true)
	connManager := NewConnectionManager(testFailEndpoint, statusinterface.NewNoopStatusProvider())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = connManager.NewConnection(ctx)
	assert.NotNil(t, err)

	host, port, err := net.SplitHostPort(serverAddr)
	assert.Nil(t, err)
	portInt, err := strconv.Atoi(port)
	assert.Nil(t, err)
	testSuccessEndpoint := config.NewEndpoint("api-key", host, portInt, false)
	connManager = NewConnectionManager(testSuccessEndpoint, statusinterface.NewNoopStatusProvider())
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = connManager.NewConnection(ctx)
	assert.Nil(t, err)
}
