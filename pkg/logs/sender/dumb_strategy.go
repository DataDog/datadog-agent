// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sender provides log message sending functionality
package sender

import (
	"bytes"
	"encoding/gob"
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

// PatternData is a simple intermediate format for patterns (avoids import cycles with grpc package)
// stream_worker will convert this to protobuf PatternDefine or PatternUpdate
type PatternData struct {
	PatternID  uint64
	Template   string
	ParamCount uint32
	PosList    []uint32
	IsUpdate   bool // true for PatternUpdate, false for PatternDefine
}

// LogData represents a log with pattern reference and wildcard values
type LogData struct {
	PatternID      uint64
	WildcardValues []string
	Timestamp      uint64
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
	// Use rendered content for pattern extraction (plain text), not encoded content (binary)
	content := m.GetRenderedContent()
	if len(content) == 0 {
		return
	}

	// Debug: Check content
	previewLen := 100
	if len(content) < previewLen {
		previewLen = len(content)
	}
	log.Infof("🔍 Tokenizing rendered content (first %d chars): %q", previewLen, content[:previewLen])

	contentStr := bytesToString(content)

	// Simple pattern extraction for POC
	tokenList := automaton.TokenizeString(contentStr)
	if tokenList != nil && !tokenList.IsEmpty() {
		cluster, changeType := s.clusterManager.Add(tokenList)
		if cluster != nil {
			cluster.GeneratePattern()

			// Log pattern changes
			switch changeType {
			case clustering.PatternNew:
				log.Infof("📝 NEW pattern discovered: PatternID=%d, Template='%s', Size=%d",
					cluster.GetPatternID(), cluster.GetPatternString(), cluster.Size())
			case clustering.PatternUpdated:
				log.Infof("🔄 Pattern UPDATED: PatternID=%d, Template='%s', Size=%d",
					cluster.GetPatternID(), cluster.GetPatternString(), cluster.Size())
			case clustering.PatternNoChange:
				log.Debugf("Pattern matched: PatternID=%d", cluster.GetPatternID())
			}

			// Step 1: Send pattern change (define/update) if needed
			if changeType == clustering.PatternNew || changeType == clustering.PatternUpdated {
				patternPayload, err := s.buildPatternChangePayload(m, cluster, changeType)
				if err != nil {
					log.Warn("Failed to build pattern change payload", err)
					return
				}
				log.Debugf("⏫ Queuing pattern payload (changeType=%v, patternID=%d) to outputChan", changeType, cluster.GetPatternID())
				s.outputChan <- patternPayload
				log.Debugf("✅ Pattern payload queued successfully")
			}

			// Step 2: Send log with pattern reference + wildcard values
			logPayload, err := s.buildLogPayload(m, cluster)
			if err != nil {
				log.Warn("Failed to build log payload", err)
				return
			}
			log.Debugf("⏫ Queuing log payload (patternID=%d) to outputChan", cluster.GetPatternID())
			s.outputChan <- logPayload
			log.Debugf("✅ Log payload queued successfully")
		}
	}
}

// buildPatternChangePayload creates a payload for PatternDefine or PatternUpdate
func (s *dumbStrategy) buildPatternChangePayload(m *message.Message, cluster *clustering.Cluster, changeType clustering.PatternChangeType) (*message.Payload, error) {
	// Get character positions where wildcards appear in the template string
	charPos := cluster.GetWildcardCharPositions()
	posList := make([]uint32, len(charPos))
	for i, pos := range charPos {
		posList[i] = uint32(pos)
	}

	// Create pattern data
	patternData := &PatternData{
		PatternID:  cluster.GetPatternID(),
		Template:   cluster.GetPatternString(),
		ParamCount: uint32(len(charPos)),
		PosList:    posList,
		IsUpdate:   changeType == clustering.PatternUpdated,
	}

	// Serialize to binary format
	return s.serializePattern(patternData, m)
}

// buildLogPayload creates a payload for Log with StructuredLog (pattern_id + wildcard values)
func (s *dumbStrategy) buildLogPayload(m *message.Message, cluster *clustering.Cluster) (*message.Payload, error) {
	// Extract wildcard values from the cluster
	wildcardValues := cluster.GetWildcardValues()

	// Create log data
	logData := &LogData{
		PatternID:      cluster.GetPatternID(),
		WildcardValues: wildcardValues,
		Timestamp:      uint64(m.IngestionTimestamp),
	}

	// Serialize to binary format
	return s.serializeLog(logData, m)
}

// serializePattern serializes pattern data using gob encoding
func (s *dumbStrategy) serializePattern(pattern *PatternData, m *message.Message) (*message.Payload, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)

	if err := encoder.Encode(pattern); err != nil {
		return nil, err
	}

	// Create payload with original message metadata
	metaCopy := m.MessageMetadata
	// Add pattern change indicator to processing tags
	if pattern.IsUpdate {
		metaCopy.ProcessingTags = append(metaCopy.ProcessingTags, "data_type:pattern_update")
	} else {
		metaCopy.ProcessingTags = append(metaCopy.ProcessingTags, "data_type:pattern_define")
	}

	return message.NewPayload(
		[]*message.MessageMetadata{&metaCopy}, // original message metadata with pattern tag
		buf.Bytes(),                           // gob-encoded pattern data
		"",                                    // no content encoding - gRPC handles compression
		buf.Len(),                             // gob size
	), nil
}

// serializeLog serializes log data using gob encoding
func (s *dumbStrategy) serializeLog(logData *LogData, m *message.Message) (*message.Payload, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)

	if err := encoder.Encode(logData); err != nil {
		return nil, err
	}

	// Create payload with original message metadata
	metaCopy := m.MessageMetadata
	// Add log with pattern reference indicator
	metaCopy.ProcessingTags = append(metaCopy.ProcessingTags, "data_type:log_with_pattern")

	return message.NewPayload(
		[]*message.MessageMetadata{&metaCopy}, // original message metadata with log tag
		buf.Bytes(),                           // gob-encoded log data
		"",                                    // no content encoding - gRPC handles compression
		buf.Len(),                             // gob size
	), nil
}

func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}
