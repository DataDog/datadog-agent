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

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/tags"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nanoToMillis = 1000000

// batchEntry is a per-message sidecar used during batch tokenization.
// It keeps msg, preprocessed content, and jsonContext aligned so that
// tokenization results can be correctly associated with each message
// even when some messages are skipped (empty content).
type batchEntry struct {
	msg         *message.Message
	content     string // preprocessed content (JSON extracted message, or raw string)
	jsonContext []byte // from PreprocessJSON; nil if not JSON
}

const (
	// defaultTokenizeBatchSize is the maximum number of messages to accumulate
	// before calling TokenizeBatch. Larger batches amortize CGo overhead more.
	// The non-blocking drain strategy means this is an upper bound only — batches
	// are flushed immediately when the input channel is empty, so single-message
	// latency is not affected.
	defaultTokenizeBatchSize = 20
)

// dvTypeBackings holds the three oneof wrapper types for a single DynamicValue in one
// contiguous allocation. Each wildcard position uses exactly one of the three fields;
// grouping them avoids three separate heap allocations per wildcard position.
type dvTypeBackings struct {
	intOneof    statefulpb.DynamicValue_IntValue
	dictOneof   statefulpb.DynamicValue_DictIndex
	stringOneof statefulpb.DynamicValue_StringValue
}

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	clusterManager         *clustering.ClusterManager
	patternEvictionManager *clustering.EvictionManager
	tagManager             *tags.TagManager
	tagEvictionManager     *tags.TagEvictionManager
	tokenizer              token.Tokenizer

	pipelineName string

	// tagCache caches the last computed tag set to avoid recomputation across messages
	// with identical metadata (common in single-source pipelines).
	tagCache struct {
		origin         *message.Origin
		hostname       string
		service        string
		source         string
		status         string
		processingTags string // joined ProcessingTags; part of cache key
		tagSet         *statefulpb.TagSet
		dictID         uint64
		tagStr         string
	}
}

// NewMessageTranslator creates a new MessageTranslator instance with the specified tokenizer.
func NewMessageTranslator(pipelineName string, tokenizer token.Tokenizer) *MessageTranslator {
	mt := &MessageTranslator{
		clusterManager: clustering.NewClusterManagerWithConfig(
			pkgconfigsetup.Datadog().GetBool("logs_config.patterns.first_word_protection"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.first_word_max_cardinality"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.saturation_threshold"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.max_patterns_per_cluster"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.pattern_scan_budget"),
		),
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

		batch := make([]batchEntry, 0, defaultTokenizeBatchSize)

		addEntry := func(msg *message.Message) {
			content := msg.GetContent()
			if len(content) == 0 {
				return // skip empty messages — no sidecar entry, no alignment break
			}
			entry := batchEntry{msg: msg}
			if results := processor.PreprocessJSON(content); results.Message != "" {
				entry.content = results.Message
				entry.jsonContext = results.JSONContext
			} else {
				entry.content = string(content)
			}
			batch = append(batch, entry)
		}

		for msg := range inputChan {
			addEntry(msg)

			// Non-blocking drain: accumulate additional messages that are already
			// queued, up to the batch size. Flush immediately when channel is empty.
			// This gives zero added latency at low throughput and batch amortization
			// at high throughput when messages arrive in bursts.
		drain:
			for len(batch) < defaultTokenizeBatchSize {
				select {
				case msg, ok := <-inputChan:
					if !ok {
						mt.processBatch(batch, outputChan)
						return
					}
					addEntry(msg)
				default:
					break drain // channel empty, flush what we have
				}
			}

			if len(batch) > 0 {
				mt.processBatch(batch, outputChan)
				batch = batch[:0]
			}
		}

		// Flush any remaining entries after channel close
		if len(batch) > 0 {
			mt.processBatch(batch, outputChan)
		}
	}()
	return outputChan
}

// processBatch tokenizes a batch of pre-screened entries in one TokenizeBatch call,
// then processes each sequentially through clustering and datum building.
// All entries in the batch have non-empty content (empty messages are skipped before enqueueing).
func (mt *MessageTranslator) processBatch(batch []batchEntry, outputChan chan *message.StatefulMessage) {
	if len(batch) == 0 {
		return
	}

	// Extract content strings for batch tokenization (aligned 1:1 with batch entries).
	contents := make([]string, len(batch))
	for i, e := range batch {
		contents[i] = e.content
	}

	// One TokenizeBatch call for the entire batch.
	// With the serial fallback (current): same per-log CGo cost as before.
	// With a future patterns_tokenize_logs_batch FFI: one LockOSThread + one Rust call.
	tokenResults, _ := mt.tokenizer.TokenizeBatch(contents)

	// Process each entry sequentially — clustering is stateful and must be sequential.
	// Alignment is guaranteed: tokenResults[i] corresponds to batch[i].
	for i, entry := range batch {
		if i >= len(tokenResults) {
			break
		}
		if tokenResults[i].Err != nil {
			log.Warnf("Failed to tokenize log message: %v", tokenResults[i].Err)
			continue
		}
		mt.processPreTokenized(entry.msg, tokenResults[i].TokenList, entry.jsonContext, outputChan)
	}
}

// processMessage is retained for testing and direct single-message use.
// The Start loop now uses the batch pipeline (processBatch → processPreTokenized).
func (mt *MessageTranslator) processMessage(msg *message.Message, outputChan chan *message.StatefulMessage) {
	content := msg.GetContent()
	if len(content) == 0 {
		return
	}
	var jsonContext []byte
	contentStr := string(content)
	if results := processor.PreprocessJSON(content); results.Message != "" {
		contentStr = results.Message
		jsonContext = results.JSONContext
	}
	tokenList, err := mt.tokenizer.Tokenize(contentStr)
	if err != nil {
		log.Warnf("Failed to tokenize log message: %v", err)
		return
	}
	mt.processPreTokenized(msg, tokenList, jsonContext, outputChan)
}

// processPreTokenized handles post-tokenization: clustering, eviction, datum construction, and sending.
// Called by both processBatch (batch pipeline) and processMessage (single-message path).
func (mt *MessageTranslator) processPreTokenized(msg *message.Message, tokenList *token.TokenList, jsonContext []byte, outputChan chan *message.StatefulMessage) {
	var patternDefineSent bool
	var patternDefineParamCount uint32

	ts := getMessageTimestamp(msg)

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

	// Encode wildcard values with type inference (int64 → dict_index → string_value).
	// Two contiguous allocations (dvBacking + typeBacking) replace the previous five
	// (dvBacking + intBacking + dictBacking + stringBacking + dynamicValues).
	// Each wildcard position uses exactly one field of dvTypeBackings; the unused
	// fields consume memory but avoid per-position heap allocs.
	// High-cardinality values (UUIDs, IPs, request IDs) that are not already in the
	// dict are sent inline as string_value — no dict entry created, stopping unbounded
	// TagManager growth.
	n := len(wildcardValues)
	dvBacking := make([]statefulpb.DynamicValue, n)
	typeBacking := make([]dvTypeBackings, n)
	dynamicValues := make([]*statefulpb.DynamicValue, n)
	for i := range dvBacking {
		dynamicValues[i] = &dvBacking[i]
	}
	for i, val := range wildcardValues {
		mt.fillDynamicValue(&dvBacking[i], &typeBacking[i].intOneof, &typeBacking[i].dictOneof, &typeBacking[i].stringOneof, val)
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
// All tags are joined as a single string, encoded as a single dictionary entry in the TagSet.
// A single-entry cache keyed on (origin ptr, hostname, service, source, status) avoids all
// allocations in the common case where these inputs are constant across messages (single-source pipeline).
func (mt *MessageTranslator) buildTagSet(msg *message.Message) (*statefulpb.TagSet, string, uint64, bool) {
	// Read current inputs
	currentOrigin := msg.Origin
	currentHostname := msg.MessageMetadata.Hostname
	currentService := msg.Origin.Service()
	currentSource := msg.Origin.Source()
	currentStatus := msg.MessageMetadata.GetStatus()
	currentProcessingTags := strings.Join(msg.MessageMetadata.ProcessingTags, ",")

	// Cache hit: all inputs identical → return cached result (zero allocations)
	if mt.tagCache.tagSet != nil &&
		mt.tagCache.origin == currentOrigin &&
		mt.tagCache.hostname == currentHostname &&
		mt.tagCache.service == currentService &&
		mt.tagCache.source == currentSource &&
		mt.tagCache.status == currentStatus &&
		mt.tagCache.processingTags == currentProcessingTags {
		return mt.tagCache.tagSet, mt.tagCache.tagStr, mt.tagCache.dictID, false
	}

	// Cache miss: build tag string normally.

	// Start with metadata tags (container tags, source config tags, processing tags)
	baseTags := msg.MessageMetadata.Tags()
	tagStrings := make([]string, len(baseTags), len(baseTags)+4)
	copy(tagStrings, baseTags)

	// Add log-level fields as tags (these are separate JSON fields in HTTP pipeline)
	// Required tags per proto: hostname, service
	// Other tags per proto: status, source (ddsource)

	if currentHostname != "" {
		tagStrings = append(tagStrings, "hostname:"+currentHostname)
	}

	if currentService != "" {
		tagStrings = append(tagStrings, "service:"+currentService)
	}

	if currentSource != "" {
		tagStrings = append(tagStrings, "ddsource:"+currentSource)
	}

	if currentStatus != "" {
		tagStrings = append(tagStrings, "status:"+currentStatus)
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

	// Populate cache for next call
	mt.tagCache.origin = currentOrigin
	mt.tagCache.hostname = currentHostname
	mt.tagCache.service = currentService
	mt.tagCache.source = currentSource
	mt.tagCache.status = currentStatus
	mt.tagCache.processingTags = currentProcessingTags
	mt.tagCache.tagSet = tagSet
	mt.tagCache.dictID = dictID
	mt.tagCache.tagStr = allTagsString

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

// sendRawLog creates and sends a raw log datum (currently unused)
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
	// Try parsing as int64
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
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

// fillDynamicValue fills a pre-allocated DynamicValue in-place using pre-allocated oneof wrappers,
// eliminating the 3 heap allocations per wildcard that encodeDynamicValue incurs.
// Encoding priority: int_value → dict_index (existing entries only) → string_value inline.
// New high-cardinality values (UUIDs, IPs, request IDs) are sent as string_value to prevent
// unbounded TagManager growth; no DictEntryDefine is emitted for wildcard values.
func (mt *MessageTranslator) fillDynamicValue(
	dv *statefulpb.DynamicValue,
	oneofInt *statefulpb.DynamicValue_IntValue,
	oneofDict *statefulpb.DynamicValue_DictIndex,
	oneofStr *statefulpb.DynamicValue_StringValue,
	value string,
) {
	// Integer values: encode efficiently as int_value
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		oneofInt.IntValue = intVal
		dv.Value = oneofInt
		return
	}
	// Already in dict (e.g., from a previous tag encoding): reuse existing ID
	if dictID, ok := mt.tagManager.GetStringID(value); ok {
		oneofDict.DictIndex = dictID
		dv.Value = oneofDict
		return
	}
	// New value: send inline as string_value — no dict entry created.
	// High-cardinality values (UUIDs, IPs, request IDs) never repeat,
	// so dict encoding provides zero compression benefit for them.
	oneofStr.StringValue = value
	dv.Value = oneofStr
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
