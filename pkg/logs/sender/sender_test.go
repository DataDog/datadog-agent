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
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

func TestNewSenderWorkerDistribution(t *testing.T) {
	tests := []struct {
		name            string
		workersPerQueue int
		queuesCount     int
		expectedWorkers int
	}{
		{
			name:            "default worker count",
			workersPerQueue: DefaultWorkersPerQueue,
			queuesCount:     DefaultQueuesCount,
			expectedWorkers: 1,
		},
		{
			name:            "custom worker count",
			workersPerQueue: 3,
			queuesCount:     2,
			expectedWorkers: 6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config := configmock.New(t)
			auditor := auditor.NewNullAuditor()
			destinations := &client.Destinations{}
			destFactory := func() *client.Destinations { return destinations }
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
				tc.queuesCount,
				tc.workersPerQueue,
				pipelineMonitor,
			)

			assert.Equal(t, tc.expectedWorkers, len(sender.workers))

			chanMap := make(map[chan *message.Payload]struct{})
			for range 20 {
				chanMap[sender.In()] = struct{}{}
			}

			assert.Equal(t, tc.queuesCount, len(chanMap))
		})
	}
}
