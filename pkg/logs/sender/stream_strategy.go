// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// streamStrategy is a Strategy that creates one Payload for each Message, containing
// that Message's Content. This is used for TCP destinations, which stream the output
// without batching multiple messages together.
type streamStrategy struct {
	inputChan       chan message.TimedMessage[*message.Message]
	outputChan      chan *message.Payload
	contentEncoding ContentEncoding
	done            chan struct{}
}

// NewStreamStrategy creates a new stream strategy
func NewStreamStrategy(inputChan chan message.TimedMessage[*message.Message], outputChan chan *message.Payload, contentEncoding ContentEncoding) Strategy {
	return &streamStrategy{
		inputChan:       inputChan,
		outputChan:      outputChan,
		contentEncoding: contentEncoding,
		done:            make(chan struct{}),
	}
}

// Send sends one message at a time and forwards them to the next stage of the pipeline.
func (s *streamStrategy) Start() {
	go func() {
		for msg := range s.inputChan {

			metrics.TlmChanTime.Observe(float64(msg.SendDuration().Nanoseconds()), "stream")
			metrics.TlmChanTimeSkew.Set(telemetry.GetSkew(metrics.TlmChanTime, "stream"), "stream")

			if msg.Inner.Origin != nil {
				msg.Inner.Origin.LogSource.LatencyStats.Add(msg.Inner.GetLatency())
			}

			encodedPayload, err := s.contentEncoding.encode(msg.Inner.GetContent())
			metrics.TlmMessageLatency.Observe(float64(msg.Inner.GetLatency()))

			if err != nil {
				log.Warn("Encoding failed - dropping payload", err)
				return
			}

			s.outputChan <- &message.Payload{
				Messages:      []*message.Message{msg.Inner},
				Encoded:       encodedPayload,
				Encoding:      s.contentEncoding.name(),
				UnencodedSize: len(msg.Inner.GetContent()),
			}
		}
		s.done <- struct{}{}
	}()
}

// Stop stops the strategy
func (s *streamStrategy) Stop() {
	close(s.inputChan)
	<-s.done
}
