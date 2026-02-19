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

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/tags"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nanoToMillis = 1000000

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	clusterManager         *clustering.ClusterManager
	patternEvictionManager *clustering.EvictionManager
	tagManager             *tags.TagManager
	tagEvictionManager     *tags.TagEvictionManager
	tokenizer              token.Tokenizer

	pipelineName string
}

// NewMessageTranslator creates a new MessageTranslator instance with the specified tokenizer.
func NewMessageTranslator(pipelineName string, tokenizer token.Tokenizer) *MessageTranslator {
	mt := &MessageTranslator{
		clusterManager:         clustering.NewClusterManager(),
		patternEvictionManager: clustering.NewEvictionManager(),
		tagManager:             tags.NewTagManager(),
		tagEvictionManager:     tags.NewTagEvictionManager(),
		tokenizer:              tokenizer,
		pipelineName:           pipelineName,
	}
	tlmPipelineStateSize.Set(0, pipelineName)
	return mt

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

	// Get message content
	content := msg.GetContent()
	if len(content) == 0 {
		return
	}

	// Try JSON preprocessing - if message extracted, use it for pattern matching
	contentStr := string(content)

	var jsonContext []byte
	if results := processor.PreprocessJSON(content); results.Message != "" {
		contentStr = results.Message
		jsonContext = results.JSONContext
	}

	// Tokenize the message content (either json extracted message or full content)
	tokenList, err := mt.tokenizer.Tokenize(contentStr)
	if err != nil {
		log.Warnf("Failed to tokenize log message: %v", err)
		return
	}

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
		mt.tagEvictionManager.Evict(mt.tagManager, tagCount, tagMemoryBytes, tagCountOverLimit, tagBytesOverLimit)
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

	// Build complete tag list and encode as TagSet
	tagSet, allTagsString, dictID, isNew := mt.buildTagSet(msg)
	if isNew {
		mt.sendDictEntryDefine(outputChan, msg, dictID, allTagsString)
	}

	// Send StructuredLog with all fields
	tsMillis := ts.UnixNano() / nanoToMillis
	mt.sendStructuredLog(outputChan, msg, tsMillis, patternID, dynamicValues, tagSet, jsonContext)
}

// buildTagSet constructs the complete tag list for a message and encodes it as a TagSet.
// This includes log-level fields (hostname, service, ddsource, status) as tags,
// plus all other tags from the message metadata (container tags, source config tags, processing tags).
// All tags are joined as a single string, encoded as a single dictionary entry in the TagSet
func (mt *MessageTranslator) buildTagSet(msg *message.Message) (*statefulpb.TagSet, string, uint64, bool) {
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

// getMessageTimestamp returns the timestamp for the message, preferring ServerlessExtra.Timestamp
func getMessageTimestamp(msg *message.Message) time.Time {
	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}
	return ts
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

// sendRawLog creates and sends a raw log datum
func (mt *MessageTranslator) sendRawLog(outputChan chan *message.StatefulMessage, msg *message.Message, contentStr string, ts time.Time, tagSet *statefulpb.TagSet) {
	logDatum := buildRawLog(contentStr, ts, tagSet)

	tlmPipelineRawLogsProcessed.Inc(mt.pipelineName)
	tlmPipelineRawLogsProcessedBytes.Add(float64(proto.Size(logDatum)), mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    logDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendStructuredLog creates and sends a StructuredLog datum
func (mt *MessageTranslator) sendStructuredLog(outputChan chan *message.StatefulMessage, msg *message.Message, timestamp int64, patternID uint64, dynamicValues []*statefulpb.DynamicValue, tagSet *statefulpb.TagSet, jsonContext []byte) {
	logDatum := buildStructuredLog(timestamp, patternID, dynamicValues, tagSet, jsonContext)

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
// Priority: int64 → dict_index (via tagManager)
// Returns the encoded DynamicValue and whether a new dict entry was created
func (mt *MessageTranslator) encodeDynamicValue(value string) (*statefulpb.DynamicValue, uint64, bool) {
	// Skip int conversion for values with leading zeros (e.g. "01", "-007") to preserve them as strings.
	// len > 1 check allows literal "0" to still be converted.
	if len(value) > 1 && (value[0] == '0' || (value[0] == '-' && len(value) > 2 && value[1] == '0')) {
		// fall through to string dictionary encoding
	} else if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_IntValue{
				IntValue: intVal,
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

// buildStructuredLog creates a Datum containing a StructuredLog
func buildStructuredLog(timestamp int64, patternID uint64, dynamicValues []*statefulpb.DynamicValue, tagSet *statefulpb.TagSet, jsonContext []byte) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: timestamp,
				Content: &statefulpb.Log_Structured{
					Structured: &statefulpb.StructuredLog{
						PatternId:     patternID,
						DynamicValues: dynamicValues,
						JsonContext:   jsonContext,
					},
				},
				Tags: tagSet,
			},
		},
	}
}

// buildRawLog creates a Datum containing a raw log (no pattern)
func buildRawLog(content string, ts time.Time, tagSet *statefulpb.TagSet) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: ts.UnixNano() / nanoToMillis,
				Content: &statefulpb.Log_Raw{
					Raw: content,
				},
				Tags: tagSet,
			},
		},
	}
}
