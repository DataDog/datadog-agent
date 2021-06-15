// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// StreamStrategy is a shared stream strategy.
var StreamStrategy Strategy = &streamStrategy{}

// streamStrategy contains all the logic to send one log at a time.
type streamStrategy struct{}

func (s *streamStrategy) Flush(ctx context.Context) {
	// nothing to do
}

// Send sends one message at a time and forwards them to the next stage of the pipeline.
func (s *streamStrategy) Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	for message := range inputChan {
		if message.Origin != nil {
			message.Origin.LogSource.LatencyStats.Add(message.GetLatency())
		}
		err := send(message.Content)
		if err != nil {
			if shouldStopSending(err) {
				return
			}
			log.Warnf("Could not send payload: %v", err)
		}
		metrics.LogsSent.Add(1)
		metrics.TlmLogsSent.Inc()
		outputChan <- message
	}
}
