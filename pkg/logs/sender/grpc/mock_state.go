// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/tags"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

const nanoToMillis = 1000000

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	clusterManager *clustering.ClusterManager
	tagManager     *tags.TagManager
}

// NewMessageTranslator creates a new MessageTranslator instance
// If clusterManager is nil, a new one will be created
func NewMessageTranslator() *MessageTranslator {
	return &MessageTranslator{
		clusterManager: clustering.NewClusterManager(),
		tagManager:     tags.NewTagManager(),
	}

	// Would be shared cluster manager instead across pipelines when implemented.
	// if clusterManager == nil {
	// 	clusterManager = clustering.NewClusterManager()
	// }
	// return &MessageTranslator{
	// 	clusterManager: clusterManager,
	// }
}

// Start starts a goroutine that translates message.Message to message.StatefulMessage
// It handles pattern extraction by:
// 1. Tokenizing the message content
// 2. Using ClusterManager to create/update patterns
// 3. Sending PatternDefine for new patterns, or PatternDelete+PatternDefine for updates
// 4. Sending StructuredLog with wildcard values
// Returns the output channel for StatefulMessages
func (mt *MessageTranslator) Start(inputChan chan *message.Message, bufferSize int) chan *message.StatefulMessage {
	outputChan := make(chan *message.StatefulMessage, bufferSize)
	go func() {
		defer close(outputChan)

		for msg := range inputChan {
			mt.processMessage(msg, outputChan)
		}
	}()
	return outputChan
}

// StartMessageTranslator is a convenience function that creates a MessageTranslator with a cluster manager
// Returns the output channel for StatefulMessages
func StartMessageTranslator(inputChan chan *message.Message, bufferSize int) chan *message.StatefulMessage {
	// Use a shared cluster manager for all pipelines (patterns shared across pipelines)
	translator := NewMessageTranslator()
	return translator.Start(inputChan, bufferSize)
}

// processMessage handles a single message: tokenizes, creates patterns, and sends appropriate datums
func (mt *MessageTranslator) processMessage(msg *message.Message, outputChan chan *message.StatefulMessage) {
	var patternDefineSent bool
	var patternDefineParamCount uint32

	ts := getMessageTimestamp(msg)

	// Get message content
	content := msg.GetContent()
	if len(content) == 0 {
		return
	}

	// Tokenize the message content
	contentStr := string(content)
	tokenList := tokenizeMessage(contentStr)

	// Process tokenized log through cluster manager to get/create pattern
	pattern, changeType := mt.clusterManager.Add(tokenList)

	// Extract wildcard values from the pattern
	wildcardValues := pattern.GetWildcardValues(tokenList)

	// Handle sending PatternDefine or PatternDelete as needed
	mt.handlePatternChange(pattern, changeType, msg, outputChan, &patternDefineSent, &patternDefineParamCount)

	// Build complete tag list including log-level fields
	// These fields are sent as separate JSON fields in the HTTP pipeline,
	// but as tags in the gRPC stateful pipeline (per proto spec)
	tagStrings := mt.buildTagStrings(msg)

	// Encode tags
	encodedTags, newEntries := mt.tagManager.EncodeTagStrings(tagStrings)

	// Send any new dictionary entries first
	for id, value := range newEntries {
		mt.sendDictEntryDefine(outputChan, msg, id, value)
	}

	// Send StructuredLog with pattern_id + dynamic values
	mt.sendStructuredLog(outputChan, msg, pattern, wildcardValues, ts, encodedTags)
}

// buildTagStrings constructs the complete tag list for a message.
// This includes log-level fields (hostname, service, ddsource, status) as tags,
// plus all other tags from the message metadata (container tags, source config tags, processing tags).
func (mt *MessageTranslator) buildTagStrings(msg *message.Message) []string {
	// Start with metadata tags (container tags, source config tags, processing tags)
	tagStrings := msg.MessageMetadata.Tags()

	// Add log-level fields as tags (these are separate JSON fields in HTTP pipeline)
	// Required tags per proto: hostname, service
	// Other tags per proto: status, source (ddsource)

	if hostname := msg.MessageMetadata.Hostname; hostname != "" {
		tagStrings = append(tagStrings, "hostname:"+hostname)
	}

	if service := msg.Origin.Service(); service != "" {
		tagStrings = append(tagStrings, "service:"+service)
	}

	if source := msg.Origin.Source(); source != "" {
		tagStrings = append(tagStrings, "ddsource:"+source)
	}

	if status := msg.MessageMetadata.GetStatus(); status != "" {
		tagStrings = append(tagStrings, "status:"+status)
	}

	return tagStrings
}

// getMessageTimestamp returns the timestamp for the message, preferring ServerlessExtra.Timestamp
func getMessageTimestamp(msg *message.Message) time.Time {
	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}
	return ts
}

// tokenizeMessage tokenizes the message content string
func tokenizeMessage(contentStr string) *token.TokenList {
	tokenizer := automaton.NewTokenizer(contentStr)
	return tokenizer.Tokenize()
}

// handlePatternChange handles pattern changes based on PatternChangeType from cluster manager
// Uses the change type to determine if we need to send PatternDefine/PatternDelete
// The snapshot mechanism in inflight.go tracks what's been sent for stream recovery
func (mt *MessageTranslator) handlePatternChange(pattern *clustering.Pattern, changeType clustering.PatternChangeType, msg *message.Message, outputChan chan *message.StatefulMessage, patternDefineSent *bool, patternDefineParamCount *uint32) {
	switch changeType {
	case clustering.PatternNew:
		// New pattern - send PatternDefine (may have 0 wildcards initially)
		mt.sendPatternDefine(pattern, msg, outputChan, patternDefineSent, patternDefineParamCount)

	case clustering.PatternUpdated:
		// Pattern structure changed (e.g., 0→N wildcards, or N→M wildcards)
		mt.sendPatternDelete(pattern.PatternID, msg, outputChan)
		mt.sendPatternDefine(pattern, msg, outputChan, patternDefineSent, patternDefineParamCount)

	case clustering.PatternNoChange:
	}
}

// sendPatternDefine creates and sends a PatternDefine datum
func (mt *MessageTranslator) sendPatternDefine(pattern *clustering.Pattern, msg *message.Message, outputChan chan *message.StatefulMessage, patternDefineSent *bool, patternDefineParamCount *uint32) {
	patternDatum := buildPatternDefine(pattern)
	if pd := patternDatum.GetPatternDefine(); pd != nil {
		*patternDefineParamCount = pd.ParamCount
	}
	outputChan <- &message.StatefulMessage{
		Datum:    patternDatum,
		Metadata: &msg.MessageMetadata,
	}
	*patternDefineSent = true
}

// sendPatternDelete creates and sends a PatternDelete datum
func (mt *MessageTranslator) sendPatternDelete(patternID uint64, msg *message.Message, outputChan chan *message.StatefulMessage) {
	deleteDatum := buildPatternDelete(patternID)
	outputChan <- &message.StatefulMessage{
		Datum:    deleteDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendDictEntryDefine creates and sends a DictEntryDefine datum
func (mt *MessageTranslator) sendDictEntryDefine(outputChan chan *message.StatefulMessage, msg *message.Message, id uint64, value string) {
	dictDatum := buildDictEntryDefine(id, value)
	outputChan <- &message.StatefulMessage{
		Datum:    dictDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendRawLog creates and sends a raw log datum
// todo: AGNTLOG-414: Will be used for first log without a pattern
func (mt *MessageTranslator) sendRawLog(outputChan chan *message.StatefulMessage, msg *message.Message, contentStr string, ts time.Time, tags []*statefulpb.Tag) {
	logDatum := buildRawLog(contentStr, ts, tags)
	outputChan <- &message.StatefulMessage{
		Datum:    logDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendStructuredLog creates and sends a StructuredLog datum
func (mt *MessageTranslator) sendStructuredLog(outputChan chan *message.StatefulMessage, msg *message.Message, pattern *clustering.Pattern, wildcardValues []string, ts time.Time, tags []*statefulpb.Tag) {
	logDatum := buildStructuredLog(pattern.PatternID, wildcardValues, ts, tags)
	outputChan <- &message.StatefulMessage{
		Datum:    logDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// buildPatternDefine creates a PatternDefine Datum from a Pattern
func buildPatternDefine(pattern *clustering.Pattern) *statefulpb.Datum {
	charPositions := pattern.GetWildcardCharPositions()
	posList := make([]uint32, len(charPositions))
	for i, pos := range charPositions {
		posList[i] = uint32(pos)
	}

	return &statefulpb.Datum{
		Data: &statefulpb.Datum_PatternDefine{
			PatternDefine: &statefulpb.PatternDefine{
				PatternId:  pattern.PatternID,
				Template:   pattern.GetPatternString(),
				ParamCount: uint32(pattern.GetWildcardCount()),
				PosList:    posList,
			},
		},
	}
}

// buildPatternDelete creates a PatternDelete Datum for a pattern ID
func buildPatternDelete(patternID uint64) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_PatternDelete{
			PatternDelete: &statefulpb.PatternDelete{
				PatternId: patternID,
			},
		},
	}
}

// buildDictEntryDefine creates a DictEntryDefine Datum
func buildDictEntryDefine(id uint64, value string) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_DictEntryDefine{
			DictEntryDefine: &statefulpb.DictEntryDefine{
				Id:    id,
				Value: value,
			},
		},
	}
}

// buildStructuredLog creates a Datum containing a StructuredLog
func buildStructuredLog(patternID uint64, wildcardValues []string, ts time.Time, tags []*statefulpb.Tag) *statefulpb.Datum {
	// Convert wildcard values to DynamicValue format
	dynamicValues := make([]*statefulpb.DynamicValue, len(wildcardValues))
	for i, value := range wildcardValues {
		dynamicValues[i] = &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_StringValue{
				StringValue: value,
			},
		}
	}

	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: uint64(ts.UnixNano() / nanoToMillis),
				Content: &statefulpb.Log_Structured{
					Structured: &statefulpb.StructuredLog{
						PatternId:     patternID,
						DynamicValues: dynamicValues,
					},
				},
				// tags are already fully encoded in the tag manager
				Tags: tags,
			},
		},
	}
}

// buildRawLog creates a Datum containing a raw log (no pattern)
func buildRawLog(content string, ts time.Time, tags []*statefulpb.Tag) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: uint64(ts.UnixNano() / nanoToMillis),
				Content: &statefulpb.Log_Raw{
					Raw: content,
				},
				Tags: tags,
			},
		},
	}
}
