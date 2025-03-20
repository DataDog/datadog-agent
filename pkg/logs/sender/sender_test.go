// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

type mockAuditor struct {
	auditor.Auditor
}

type mockDestinations struct {
	*client.Destinations
}

func TestNewSenderWorkerDistribution(t *testing.T) {
	tests := []struct {
		name            string
		workerCount     int
		expectedWorkers int
	}{
		{
			name:            "default worker count",
			workerCount:     DefaultWorkerCount,
			expectedWorkers: DefaultWorkerCount,
		},
		{
			name:            "custom worker count",
			workerCount:     3,
			expectedWorkers: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config := configmock.New(t)
			auditor := &mockAuditor{}
			destinations := &mockDestinations{}
			destFactory := func() *client.Destinations { return destinations.Destinations }
			bufferSize := 100
			senderDoneChan := make(chan *sync.WaitGroup)
			flushWg := &sync.WaitGroup{}
			pipelineMonitor := metrics.NewNoopPipelineMonitor("test")

			// Create sender
			sender := NewSenderV2(
				config,
				auditor,
				destFactory,
				bufferSize,
				senderDoneChan,
				flushWg,
				tc.workerCount,
				pipelineMonitor,
			)

			assert.Equal(t, tc.expectedWorkers, len(sender.workers))
		})
	}
}
