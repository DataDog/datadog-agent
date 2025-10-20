// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sender provides log message sending functionality
package sender

import (
	"bytes"
	"encoding/json"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dumbStrategy is a minimal batching strategy that forwards one message per payload.
type dumbStrategy struct {
	inputChan      chan *message.Message
	clusterManager *clustering.ClusterManager
	outputChan     chan *message.Payload
	flushChan      chan struct{}
	serializer     Serializer
	compression    compression.Compressor
	pipelineName   string

	maxContentSize int

	stopChan chan struct{}
	buffer   []*message.Message
}

// Simple pattern payload for POC - just the essential fields
type PatternPayload struct {
	PatternID   uint64 `json:"pattern_id"`
	Pattern     string `json:"pattern"`
	ParamCount  int    `json:"param_count"`
	WildcardPos []int  `json:"wildcard_positions"`
	// OriginalMsg string `json:"original_message"` // For debugging and double checking if pattern is correct base on the original message. Might remove it after POC. Protobuf might not be happy with this.
}

// NewDumbStrategy returns a strategy that sends one message per payload using the
// provided serializer and compressor. Messages larger than maxContentSize are
// dropped to mimic batch strategy behaviour.
func NewDumbStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serializer Serializer,
	maxContentSize int,
	pipelineName string,
	compression compression.Compressor,
) Strategy {
	return &dumbStrategy{
		inputChan:      inputChan,
		outputChan:     outputChan,
		flushChan:      flushChan,
		serializer:     serializer,
		compression:    compression,
		pipelineName:   pipelineName,
		maxContentSize: maxContentSize,
		clusterManager: clustering.NewClusterManager(),
		stopChan:       make(chan struct{}),
		buffer:         make([]*message.Message, 0, 1),
	}
}

// Start begins processing messages from the input channel.
func (s *dumbStrategy) Start() {
	go func() {
		defer close(s.stopChan)
		for {
			select {
			case msg, ok := <-s.inputChan:
				if !ok {
					s.flushBuffer()
					return
				}
				s.bufferMessage(msg)
				s.flushBuffer()
			case <-s.flushChan:
				s.flushBuffer()
			}
		}
	}()
}

// Stop stops the strategy and waits for the processing goroutine to exit.
func (s *dumbStrategy) Stop() {
	close(s.inputChan)
	<-s.stopChan
}

func (s *dumbStrategy) bufferMessage(m *message.Message) {
	if m == nil {
		return
	}

	if s.maxContentSize > 0 && len(m.GetContent()) > s.maxContentSize {
		log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", s.pipelineName, len(m.GetContent()), s.maxContentSize)
		tlmDroppedTooLarge.Inc(s.pipelineName)
		return
	}

	s.buffer = append(s.buffer, m)
}

func (s *dumbStrategy) flushBuffer() {
	if len(s.buffer) > 0 {
		s.processMessage(s.buffer[0])
		s.buffer = s.buffer[:0]
	}
}

func (s *dumbStrategy) processMessage(m *message.Message) {
	content := m.GetContent()
	if len(content) == 0 {
		return
	}

	// Simple pattern extraction for POC
	tokenList := automaton.TokenizeString(bytesToString(content))
	if tokenList != nil && !tokenList.IsEmpty() {
		if cluster := s.clusterManager.Add(tokenList); cluster != nil {
			cluster.GeneratePattern()

			// Build simple pattern payload
			payload, err := s.buildSimplePatternPayload(m, cluster)
			if err != nil {
				log.Warn("Failed to build payload", err)
				return
			}

			s.outputChan <- payload
		}
	}
}

// Simple pattern payload builder for POC
func (s *dumbStrategy) buildSimplePatternPayload(m *message.Message, cluster *clustering.Cluster) (*message.Payload, error) {
	patternPayload := PatternPayload{
		PatternID:   cluster.GetPatternID(),
		Pattern:     cluster.GetPatternString(),
		ParamCount:  len(cluster.GetWildcardPositions()),
		WildcardPos: cluster.GetWildcardPositions(),
		// OriginalMsg: bytesToString(m.GetContent()), // Keep for POC debugging
	}

	// Use existing serialization with compression - intake handles decompression
	return s.serializePayload(patternPayload, m)
}

// ========== COMMENTED OUT COMPLEX LOGIC FOR POC ==========
/*
func (s *dumbStrategy) buildPayload(m *message.Message) (*message.Payload, error) {
	if s.cluster != nil && s.cluster.NeedsSending() {
		// Pattern needs to be sent (new or updated)
		if s.cluster.IsNewPattern() {
			return s.buildPatternCreationPayload(m)
		} else if s.cluster.WasUpdatedSinceLastSent() {
			return s.buildPatternUpdatePayload(m)
		}
	} else if s.cluster != nil {
		// Pattern already sent, just send wildcards
		return s.buildWildcardPayload(m)
	}

	// No pattern, send raw message (fallback)
	return s.buildRawPayload(m)
}

func (s *dumbStrategy) buildPatternCreationPayload(m *message.Message) (*message.Payload, error) {
	patternPayload := PatternPayload{
		StateChange: "pattern_create",
		PatternID:   s.cluster.GetPatternID(),
		Pattern:     s.cluster.GetPatternString(),
		ParamCount:  len(s.cluster.GetWildcardPositions()),
		WildcardPos: s.cluster.GetWildcardPositions(),
		OriginalMsg: bytesToString(m.GetContent()),
	}

	s.cluster.MarkAsSent()
	return s.serializePayload(patternPayload, m)
}

func (s *dumbStrategy) buildPatternUpdatePayload(m *message.Message) (*message.Payload, error) {
	patternPayload := PatternPayload{
		StateChange: "pattern_update",
		PatternID:   s.cluster.GetPatternID(),
		Pattern:     s.cluster.GetPatternString(),
		ParamCount:  len(s.cluster.GetWildcardPositions()),
		WildcardPos: s.cluster.GetWildcardPositions(),
	}

	s.cluster.MarkAsSent()
	return s.serializePayload(patternPayload, m)
}

func (s *dumbStrategy) buildWildcardPayload(m *message.Message) (*message.Payload, error) {
	// Extract wildcard values from the current message
	tokenList := automaton.TokenizeString(bytesToString(m.GetContent()))
	var wildcardValues []string
	if tokenList != nil {
		wildcardValues = s.cluster.ExtractWildcardValues(tokenList)
	}

	patternPayload := struct {
		PatternID     uint64   `json:"pattern_id"`
		DynamicValues []string `json:"dynamic_values"`
	}{
		PatternID:     s.cluster.GetPatternID(),
		DynamicValues: wildcardValues,
	}

	return s.serializePayload(patternPayload, m)
}

func (s *dumbStrategy) buildRawPayload(m *message.Message) (*message.Payload, error) {
	rawPayload := struct {
		Message string `json:"raw_message"`
	}{
		Message: bytesToString(m.GetContent()),
	}

	return s.serializePayload(rawPayload, m)
}
*/

func (s *dumbStrategy) serializePayload(payload interface{}, m *message.Message) (*message.Payload, error) {
	s.serializer.Reset()

	patternBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Compress the JSON data directly
	var encodedPayload bytes.Buffer
	compressor := s.compression.NewStreamCompressor(&encodedPayload)

	if _, err := compressor.Write(patternBytes); err != nil {
		compressor.Close()
		return nil, err
	}

	if err := compressor.Close(); err != nil {
		return nil, err
	}

	// Potentially seed some log payload instead here

	// Create payload with original message metadata
	metaCopy := m.MessageMetadata
	// Add pattern indicator to processing tags instead of encoding
	metaCopy.ProcessingTags = append(metaCopy.ProcessingTags, "data_type:pattern")

	return message.NewPayload(
		[]*message.MessageMetadata{&metaCopy}, // original message metadata with pattern tag
		encodedPayload.Bytes(),                // compressed pattern payload (sent as-is like HTTP/TCP)
		s.compression.ContentEncoding(),       // regular "gzip" or "zstd"
		len(patternBytes),                     // uncompressed pattern JSON size
	), nil
}

func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}
