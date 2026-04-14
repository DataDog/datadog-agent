// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/agent-payload/v5/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/tags"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

const nanoToMillis = 1000000

const staleTTL = 5 * time.Minute
const staleSweepInterval = 30 * time.Second

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	clusterManager         *clustering.ClusterManager
	patternEvictionManager *clustering.EvictionManager
	tagManager             *tags.TagManager
	tagEvictionManager     *tags.TagEvictionManager

	pipelineName   string
	lastStaleSweep time.Time
}

// NewMessageTranslator creates a new MessageTranslator instance
// If clusterManager is nil, a new one will be created
func NewMessageTranslator(pipelineName string) *MessageTranslator {
	mt := &MessageTranslator{
		clusterManager:         clustering.NewClusterManager(),
		patternEvictionManager: clustering.NewEvictionManager(),
		tagManager:             tags.NewTagManager(),
		tagEvictionManager:     tags.NewTagEvictionManager(),
		pipelineName:           pipelineName,
		lastStaleSweep:         time.Now(),
	}
	tlmPipelineStateSize.Set(0, pipelineName)
	return mt
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

// // StartMessageTranslator is a convenience function that creates a MessageTranslator with a cluster manager
// // Returns the output channel for StatefulMessages
// func StartMessageTranslator(inputChan chan *message.Message, bufferSize int) chan *message.StatefulMessage {
// 	// Use a shared cluster manager for all pipelines (patterns shared across pipelines)
// 	translator := NewMessageTranslator()
// 	return translator.Start(inputChan, bufferSize)
// }

// processMessage handles a single message: tokenizes, creates patterns, and sends appropriate datums
func (mt *MessageTranslator) processMessage(msg *message.Message, outputChan chan *message.StatefulMessage) {
	var patternDefineSent bool
	var patternDefineParamCount uint32

	ts := getMessageTimestamp(msg)

	// Get message content - prefer PreEncodedContent (rendered bytes before JSON wrapping)
	// when available (set by JSONEncoder for gRPC dual-send path).
	content := msg.GetContent()
	if len(msg.PreEncodedContent) > 0 {
		content = msg.PreEncodedContent
	}
	if len(content) == 0 {
		return
	}

	// Try JSON preprocessing - if message extracted, use it for pattern matching
	contentStr := string(content)

	var messageKey string
	var jsonContextSchema string
	var jsonContextValues []string
	if results := processor.PreprocessJSON(content); results.Message != "" {
		contentStr = results.Message
		messageKey = results.MessageKey
		jsonContextSchema = results.JSONContextSchema
		jsonContextValues = results.JSONContextValues
	}

	// Tokenize the message content (either json extracted message or full content)
	tokenList := tokenizeMessage(contentStr)

	// Process tokenized log through cluster manager to get/create pattern
	pattern, changeType, patternCount, estimatedBytes := mt.clusterManager.Add(tokenList)

	// CRITICAL: Extract all pattern data BEFORE eviction to prevent agent panic/data corruption.
	patternID := pattern.PatternID
	wildcardValues := pattern.GetWildcardValues(tokenList)

	// Build PatternDefine datum before eviction (if needed)
	var patternDatum *statefulpb.Datum
	if changeType == clustering.PatternNew || changeType == clustering.PatternUpdated {
		patternDatum = buildPatternDefine(pattern)
	}

	// Check if pattern eviction is needed using high watermark threshold
	countOverLimit, bytesOverLimit := mt.patternEvictionManager.ShouldEvict(patternCount, estimatedBytes)
	if countOverLimit || bytesOverLimit {
		evicted := mt.patternEvictionManager.Evict(mt.clusterManager, patternCount, estimatedBytes, countOverLimit, bytesOverLimit)
		for _, evictedPattern := range evicted {
			mt.sendPatternDelete(evictedPattern.PatternID, msg, outputChan)
		}
	}

	// Check if tag dictionary eviction is needed using high watermark threshold
	tagCount := mt.tagManager.Count()
	tagMemoryBytes := mt.tagManager.EstimatedMemoryBytes()
	tagCountOverLimit, tagBytesOverLimit := mt.tagEvictionManager.ShouldEvict(tagCount, tagMemoryBytes)
	if tagCountOverLimit || tagBytesOverLimit {
		for _, evictedID := range mt.tagEvictionManager.Evict(mt.tagManager, tagCount, tagMemoryBytes, tagCountOverLimit, tagBytesOverLimit) {
			mt.sendDictEntryDelete(outputChan, msg, evictedID)
		}
	}

	// Periodic TTL sweep: remove entries not accessed in the last 5 minutes.
	// This prevents stale entries from accumulating in state and inflating snapshot replays.
	if time.Since(mt.lastStaleSweep) >= staleSweepInterval {
		mt.lastStaleSweep = time.Now()

		for _, evictedPattern := range mt.clusterManager.EvictStalePatterns(staleTTL) {
			mt.sendPatternDelete(evictedPattern.PatternID, msg, outputChan)
		}
		for _, evictedID := range mt.tagManager.EvictStaleEntries(staleTTL) {
			mt.sendDictEntryDelete(outputChan, msg, evictedID)
		}
	}

	// Send PatternDefine for new or updated patterns
	if patternDatum != nil {
		mt.sendPatternDefine(patternDatum, msg, outputChan, &patternDefineSent, &patternDefineParamCount)
	}

	// Encode wildcard values with type inference (int64 → dict_index → string)
	dynamicValues := make([]*statefulpb.DynamicValue, len(wildcardValues))
	for i, val := range wildcardValues {
		encoded, dictID, isNew := mt.encodeDynamicValue(val)
		dynamicValues[i] = encoded

		// Send DictEntryDefine if this created a new dictionary entry
		if isNew {
			mt.sendDictEntryDefine(outputChan, msg, dictID, val)
		}
	}

	// Encode message key as DynamicValue (dict-encoded for deduplication)
	var messageKeyDV *statefulpb.DynamicValue
	if messageKey != "" {
		encoded, mkDictID, mkIsNew := mt.encodeDynamicValue(messageKey)
		messageKeyDV = encoded
		if mkIsNew {
			mt.sendDictEntryDefine(outputChan, msg, mkDictID, messageKey)
		}
	}

	// Encode JSON context schema as dict entry (keys repeat across logs from same source)
	var jsonContextSchemaID uint64
	var jsonContextValuesDV []*statefulpb.DynamicValue
	if jsonContextSchema != "" {
		var schemaIsNew bool
		jsonContextSchemaID, schemaIsNew = mt.tagManager.AddString(jsonContextSchema)
		if schemaIsNew {
			mt.sendDictEntryDefine(outputChan, msg, jsonContextSchemaID, jsonContextSchema)
		}

		// Parse schema keys so we can decide per-key whether to dict-encode the value.
		schemaKeys := strings.Split(jsonContextSchema, ",")

		// Encode each JSON context value as a DynamicValue.
		// Only dict-encode values for keys known to be low-cardinality (e.g. "level", "logger").
		// All other values are sent inline as strings since they're typically high-cardinality
		// (timestamps, PIDs, paths, durations) and would bloat dictionary state.
		jsonContextValuesDV = make([]*statefulpb.DynamicValue, len(jsonContextValues))
		for i, val := range jsonContextValues {
			var key string
			if i < len(schemaKeys) {
				key = schemaKeys[i]
			}

			if shouldDictEncodeJSONValue(key) {
				encoded, valDictID, valIsNew := mt.encodeDynamicValue(val)
				jsonContextValuesDV[i] = encoded
				if valIsNew {
					mt.sendDictEntryDefine(outputChan, msg, valDictID, val)
				}
			} else {
				jsonContextValuesDV[i] = encodeInlineString(val)
			}
		}
	}

	// Build complete tag list and encode as TagSet (excludes status)
	tagSet, allTagsString, dictID, isNew := mt.buildTagSet(msg)
	if isNew {
		mt.sendDictEntryDefine(outputChan, msg, dictID, allTagsString)
	}

	// Encode status as a separate DynamicValue (dict-encoded, few distinct values)
	var statusDV *statefulpb.DynamicValue
	if status := msg.MessageMetadata.GetStatus(); status != "" {
		encoded, statusDictID, statusIsNew := mt.encodeDynamicValue(status)
		statusDV = encoded
		if statusIsNew {
			mt.sendDictEntryDefine(outputChan, msg, statusDictID, status)
		}
	}

	// Send StructuredLog with all fields
	tsMillis := ts.UnixNano() / nanoToMillis
	mt.sendStructuredLog(outputChan, msg, tsMillis, patternID, dynamicValues, tagSet, messageKeyDV, jsonContextSchemaID, jsonContextValuesDV, statusDV)
}

// buildTagSet constructs the complete tag list for a message and encodes it as a TagSet.
// This includes log-level fields (hostname, service, ddsource) as tags,
// plus all other tags from the message metadata (container tags, source config tags, processing tags).
// Status is excluded — it varies per log and is sent as a separate field to improve delta encoding.
// All tags are joined as a single string, encoded as a single dictionary entry in the TagSet.
func (mt *MessageTranslator) buildTagSet(msg *message.Message) (*statefulpb.TagSet, string, uint64, bool) {
	// Start with metadata tags (container tags, source config tags, processing tags)
	tagStrings := msg.MessageMetadata.Tags()

	// Add log-level fields as tags (these are separate JSON fields in HTTP pipeline)
	// Required tags per proto: hostname, service
	// Note: status is sent separately on Log.Status to avoid defeating tag delta encoding.

	if hostname := msg.MessageMetadata.Hostname; hostname != "" {
		tagStrings = append(tagStrings, "hostname:"+hostname)
	}

	if service := msg.Origin.Service(); service != "" {
		tagStrings = append(tagStrings, "service:"+service)
	}

	if source := msg.Origin.Source(); source != "" {
		tagStrings = append(tagStrings, "ddsource:"+source)
	}

	allTagsString := strings.Join(tagStrings, ",")
	if allTagsString == "" {
		return nil, "", 0, false
	}

	dictID, isNew := mt.tagManager.AddString(allTagsString)

	tagSet := &statefulpb.TagSet{
		Tagset: &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_DictIndex{
				DictIndex: dictID,
			},
		},
	}

	return tagSet, allTagsString, dictID, isNew
}

// getMessageTimestamp returns the timestamp for the message.
// It prefers EncodedTimestampMs (set by the JSON encoder for HTTP dual-send) so that the gRPC
// path uses the exact same millisecond timestamp, keeping UUID derivation consistent across both
// transports. Falls back to ServerlessExtra.Timestamp and then time.Now().
func getMessageTimestamp(msg *message.Message) time.Time {
	if msg.MessageMetadata.EncodedTimestampMs != 0 {
		return time.UnixMilli(msg.MessageMetadata.EncodedTimestampMs).UTC()
	}
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		return msg.ServerlessExtra.Timestamp
	}
	return time.Now().UTC()
}

// tokenizeMessage tokenizes the message content string
func tokenizeMessage(contentStr string) *token.TokenList {
	tokenizer := automaton.NewTokenizer(contentStr)
	return tokenizer.Tokenize()
}

// sendPatternDefine creates and sends a PatternDefine datum
func (mt *MessageTranslator) sendPatternDefine(patternDatum *statefulpb.Datum, msg *message.Message, outputChan chan *message.StatefulMessage, patternDefineSent *bool, patternDefineParamCount *uint32) {
	if pd := patternDatum.GetPatternDefine(); pd != nil {
		*patternDefineParamCount = pd.ParamCount
	}

	bytesAdded := float64(proto.Size(patternDatum))
	tlmPipelinePatternAdded.Inc(mt.pipelineName)
	tlmPipelinePatternBytesAdded.Add(bytesAdded, mt.pipelineName)
	tlmPipelineStateSize.Add(bytesAdded, mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    patternDatum,
		Metadata: &msg.MessageMetadata,
	}
	*patternDefineSent = true
}

// sendPatternDelete creates and sends a PatternDelete datum
func (mt *MessageTranslator) sendPatternDelete(patternID uint64, msg *message.Message, outputChan chan *message.StatefulMessage) {
	deleteDatum := buildPatternDelete(patternID)

	bytesRemoved := float64(proto.Size(deleteDatum))
	tlmPipelinePatternRemoved.Inc(mt.pipelineName)
	tlmPipelinePatternBytesRemoved.Add(bytesRemoved, mt.pipelineName)
	tlmPipelineStateSize.Sub(bytesRemoved, mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    deleteDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendDictEntryDefine creates and sends a DictEntryDefine datum
func (mt *MessageTranslator) sendDictEntryDefine(outputChan chan *message.StatefulMessage, msg *message.Message, id uint64, value string) {
	dictDatum := buildDictEntryDefine(id, value)

	bytesAdded := float64(proto.Size(dictDatum))
	tlmPipelineTokenAdded.Inc(mt.pipelineName)
	tlmPipelineTokenBytesAdded.Add(bytesAdded, mt.pipelineName)
	tlmPipelineStateSize.Add(bytesAdded, mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    dictDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendDictEntryDelete creates and sends a DictEntryDelete datum
func (mt *MessageTranslator) sendDictEntryDelete(outputChan chan *message.StatefulMessage, msg *message.Message, id uint64) {
	deleteDatum := &statefulpb.Datum{
		Data: &statefulpb.Datum_DictEntryDelete{
			DictEntryDelete: &statefulpb.DictEntryDelete{
				Id: id,
			},
		},
	}

	bytesRemoved := float64(proto.Size(deleteDatum))
	tlmPipelineStateSize.Sub(bytesRemoved, mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    deleteDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendStructuredLog creates and sends a StructuredLog datum
func (mt *MessageTranslator) sendStructuredLog(outputChan chan *message.StatefulMessage, msg *message.Message, timestamp int64, patternID uint64, dynamicValues []*statefulpb.DynamicValue, tagSet *statefulpb.TagSet, messageKey *statefulpb.DynamicValue, jsonContextSchemaID uint64, jsonContextValues []*statefulpb.DynamicValue, status *statefulpb.DynamicValue) {
	logDatum := buildStructuredLog(timestamp, patternID, dynamicValues, tagSet, msg.MessageMetadata.DualSendUUID, messageKey, jsonContextSchemaID, jsonContextValues, status)

	tlmPipelinePatternLogsProcessed.Inc(mt.pipelineName)
	tlmPipelinePatternLogsProcessedBytes.Add(float64(proto.Size(logDatum)), mt.pipelineName)

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

// encodeDynamicValue encodes a wildcard value with type inference
// Priority: int64 → float64 → inline string (high-cardinality) → dict_index
// Returns the encoded DynamicValue and whether a new dict entry was created
func (mt *MessageTranslator) encodeDynamicValue(value string) (*statefulpb.DynamicValue, uint64, bool) {
	// Skip int conversion for values with leading zeros (e.g. "01", "-007") to preserve them as strings.
	// len > 1 check allows literal "0" to still be converted.
	hasLeadingZero := len(value) > 1 && (value[0] == '0' || (value[0] == '-' && len(value) > 2 && value[1] == '0'))
	if !hasLeadingZero {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return &statefulpb.DynamicValue{
				Value: &statefulpb.DynamicValue_IntValue{
					IntValue: intVal,
				},
			}, 0, false
		}
	}

	// Send high-cardinality values as inline strings to avoid polluting the dictionary.
	// These values are almost never reused, so dict-encoding them wastes state and snapshot bytes.
	if shouldInlineString(value) {
		return &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_StringValue{
				StringValue: value,
			},
		}, 0, false
	}

	// Dictionary encoding for non-integer values
	dictID, isNew := mt.tagManager.AddString(value)
	return &statefulpb.DynamicValue{
		Value: &statefulpb.DynamicValue_DictIndex{
			DictIndex: dictID,
		},
	}, dictID, isNew
}

// dictEncodableJSONKeys is the set of JSON context keys whose values are low-cardinality
// and benefit from dictionary encoding. All other keys' values are sent inline.
var dictEncodableJSONKeys = map[string]bool{
	"level":    true,
	"severity": true,
	"logger":   true,
	"caller":   true,
	"source":   true,
	"agent":    true,
	"status":   true,
	"method":   true,
	"func":     true,
}

// shouldDictEncodeJSONValue returns true if the JSON context value for the given key
// should be dict-encoded. Only low-cardinality fields benefit from dict encoding;
// all others are sent inline to avoid bloating dictionary state.
func shouldDictEncodeJSONValue(key string) bool {
	return dictEncodableJSONKeys[key]
}

// encodeInlineString encodes a value as an inline string DynamicValue, bypassing the dictionary.
// Integers are still encoded as int_value for compactness.
func encodeInlineString(value string) *statefulpb.DynamicValue {
	// Still try int encoding — it's smaller than a string on the wire
	hasLeadingZero := len(value) > 1 && (value[0] == '0' || (value[0] == '-' && len(value) > 2 && value[1] == '0'))
	if !hasLeadingZero {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return &statefulpb.DynamicValue{
				Value: &statefulpb.DynamicValue_IntValue{
					IntValue: intVal,
				},
			}
		}
	}
	return &statefulpb.DynamicValue{
		Value: &statefulpb.DynamicValue_StringValue{
			StringValue: value,
		},
	}
}

// shouldInlineString returns true for values that are likely high-cardinality and should
// be sent as inline strings rather than dict-encoded. This avoids bloating the dictionary
// state with values that are never reused (UUIDs, timestamps, JSON blobs, epoch floats).
func shouldInlineString(value string) bool {
	if len(value) == 0 {
		return false
	}

	// JSON objects or arrays
	if value[0] == '{' || value[0] == '[' {
		return true
	}

	// UUID pattern: 8-4-4-4-12 hex chars (36 bytes total)
	if len(value) == 36 && value[8] == '-' && value[13] == '-' && value[18] == '-' && value[23] == '-' {
		return true
	}

	// Epoch float timestamps (e.g. "1.77551962625433e+12", "1.775519724235197e+09")
	if len(value) > 10 && value[0] == '1' && value[1] == '.' && strings.ContainsAny(value, "eE") {
		return true
	}

	// ISO8601-ish timestamps (e.g. "2026-04-07T10:30:18Z", "2026-04-07 10:30:18")
	if len(value) >= 19 && value[4] == '-' && value[7] == '-' && (value[10] == 'T' || value[10] == ' ') {
		return true
	}

	// Large strings (>128 bytes) are unlikely to repeat and expensive to keep in state
	if len(value) > 128 {
		return true
	}

	return false
}

// buildRawLog creates a Datum containing a raw (unstructured) log entry.
func buildRawLog(content string, timestamp time.Time, tagSet *statefulpb.TagSet, uuid string) *statefulpb.Datum {
	ts := timestamp.UnixNano() / nanoToMillis
	log := &statefulpb.Log{
		Timestamp: ts,
		Content: &statefulpb.Log_Raw{
			Raw: content,
		},
		Tags: tagSet,
	}
	if uuid != "" {
		log.Uuid = &uuid
	}
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: log,
		},
	}
}

// buildStructuredLog creates a Datum containing a StructuredLog
func buildStructuredLog(timestamp int64, patternID uint64, dynamicValues []*statefulpb.DynamicValue, tagSet *statefulpb.TagSet, uuid string, messageKey *statefulpb.DynamicValue, jsonContextSchemaID uint64, jsonContextValues []*statefulpb.DynamicValue, status *statefulpb.DynamicValue) *statefulpb.Datum {
	log := &statefulpb.Log{
		Timestamp: timestamp,
		Content: &statefulpb.Log_Structured{
			Structured: &statefulpb.StructuredLog{
				PatternId:           patternID,
				DynamicValues:       dynamicValues,
				JsonMessageKey:      messageKey,
				JsonContextSchemaId: jsonContextSchemaID,
				JsonContextValues:   jsonContextValues,
			},
		},
		Tags:   tagSet,
		Status: status,
	}
	if uuid != "" {
		log.Uuid = &uuid
	}

	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: log,
		},
	}
}
