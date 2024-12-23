// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
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

// TestConnecitivityDiagnoseNoBlock ensures the connectivity diagnose doesn't block
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
