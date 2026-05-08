// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/tags"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	rtokenizer "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nanoToMillis = 1000000
const defaultStaleTTL = 5 * time.Minute
const staleSweepInterval = 30 * time.Second

// batchEntry is a per-message sidecar used during batch tokenization.
// It keeps msg, preprocessed content, and JSON context fields aligned so that
// tokenization results can be correctly associated with each message
// even when some messages are skipped (empty content).
type batchEntry struct {
	msg               *message.Message
	content           string // preprocessed content (JSON extracted message, or raw string)
	messageKey        string // JSON key the message was extracted from (e.g. "msg", "message")
	jsonContextKeys   []string
	jsonContextValues []interface{}
	isRawJSON         bool // true when patterns.json_as_raw=true — skip tokenization, send as RawLog
}

func getTranslatorContent(msg *message.Message) []byte {
	if len(msg.PreEncodedContent) > 0 {
		return msg.PreEncodedContent
	}
	return msg.GetContent()
}

// toValidUTF8 returns s unchanged if it is already valid UTF-8 (zero allocation).
// Otherwise replaces each maximal run of invalid bytes with U+FFFD.
// Required before writing to proto3 string fields, which must be valid UTF-8 —
// invalid bytes would corrupt or drop the datum entirely.
func toValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

const (
	// defaultTokenizeBatchSize is the maximum number of messages to accumulate
	// before calling TokenizeBatch. Larger batches amortize CGo overhead more.
	// The non-blocking drain strategy means this is an upper bound only — batches
	// are flushed immediately when the input channel is empty, so single-message
	// latency is not affected.
	defaultTokenizeBatchSize = 20
)

const (
	jsonValueKindNull byte = iota
	jsonValueKindInt
	jsonValueKindFloat
	jsonValueKindBoolFalse
	jsonValueKindBoolTrue
	jsonValueKindString
	jsonValueKindDict
	jsonValueKindRaw
	jsonValueKindIntAsString
	jsonValueKindFloatAsString
	jsonValueKindBoolFalseAsString
	jsonValueKindBoolTrueAsString
)

// dvTypeBackings holds the three oneof wrapper types for a single DynamicValue in one
// contiguous allocation. Each wildcard position uses exactly one of the three fields;
// grouping them avoids three separate heap allocations per wildcard position.
type dvTypeBackings struct {
	intOneof     statefulpb.DynamicValue_IntValue
	floatOneof   statefulpb.DynamicValue_FloatValue
	boolOneof    statefulpb.DynamicValue_BoolValue
	dictOneof    statefulpb.DynamicValue_DictIndex
	rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	stringOneof  statefulpb.DynamicValue_StringValue
}

type compactJSONContextValues struct {
	kinds        []byte
	ints         []int64
	floats       []float64
	dicts        []uint64
	rawValues    [][]byte
	stringValues []string
}

type dictEntryDefinition struct {
	id    uint64
	value string
}

type tagCacheEntry struct {
	origin         *message.Origin
	hostname       string
	source         string
	processingTags string // joined ProcessingTags; part of cache key
	tagSet         *statefulpb.TagSet
	dictID         uint64
	tagStr         string
}

type jsonSchemaState struct {
	schemaID     uint64
	schemaKey    string
	messageKeyID uint64
	keyIDs       []uint64
}

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	clusterManager         *clustering.ClusterManager
	patternEvictionManager *clustering.EvictionManager
	tagManager             *tags.TagManager
	tagEvictionManager     *tags.TagEvictionManager
	tokenizer              token.Tokenizer
	jsonLogsAsRaw          bool // when true, JSON logs bypass stateful encoding and are sent as RawLog
	jsonSchemaToID         map[string]uint64
	jsonSchemaByID         map[uint64]*jsonSchemaState
	jsonSchemaIDsByDictID  map[uint64]map[uint64]struct{}
	nextJSONSchemaID       uint64

	pipelineName   string
	lastStaleSweep time.Time
	staleTTL       time.Duration

	// tagCache caches the last computed tag set to avoid recomputation across messages
	// with identical metadata (common in single-source pipelines).
	tagCache struct {
		origin         *message.Origin
		hostname       string
		source         string
		tagsString     string
		processingTags string // joined ProcessingTags; part of cache key
		tagSet         *statefulpb.TagSet
		dictID         uint64
		tagStr         string
	}
}

// MessageTranslatorOption configures MessageTranslator behavior.
type MessageTranslatorOption func(*MessageTranslator)

// WithMessageTranslatorStaleTTL overrides how long pattern and dictionary state
// may remain idle before the stale sweep evicts it.
func WithMessageTranslatorStaleTTL(ttl time.Duration) MessageTranslatorOption {
	return func(mt *MessageTranslator) {
		mt.staleTTL = ttl
	}
}

// NewMessageTranslator creates a new MessageTranslator instance with the specified tokenizer.
func NewMessageTranslator(pipelineName string, tokenizer token.Tokenizer, opts ...MessageTranslatorOption) *MessageTranslator {
	staleTTL := time.Duration(pkgconfigsetup.Datadog().GetInt("logs_config.patterns.stale_ttl_seconds")) * time.Second
	if staleTTL <= 0 {
		staleTTL = defaultStaleTTL
	}

	mt := &MessageTranslator{
		clusterManager: clustering.NewClusterManagerWithConfig(
			pkgconfigsetup.Datadog().GetBool("logs_config.patterns.first_word_protection"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.first_word_max_cardinality"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.saturation_threshold"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.max_patterns_per_cluster"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.pattern_scan_budget"),
			pkgconfigsetup.Datadog().GetInt("logs_config.patterns.max_template_bytes"),
		),
		patternEvictionManager: clustering.NewEvictionManager(),
		tagManager:             tags.NewTagManager(),
		tagEvictionManager:     tags.NewTagEvictionManager(),
		tokenizer:              tokenizer,
		jsonLogsAsRaw:          pkgconfigsetup.Datadog().GetBool("logs_config.patterns.json_as_raw"),
		jsonSchemaToID:         make(map[string]uint64),
		jsonSchemaByID:         make(map[uint64]*jsonSchemaState),
		jsonSchemaIDsByDictID:  make(map[uint64]map[uint64]struct{}),
		nextJSONSchemaID:       flatLogEmptyDictIndex,
		pipelineName:           pipelineName,
		lastStaleSweep:         time.Now(),
		staleTTL:               staleTTL,
	}
	for _, opt := range opts {
		opt(mt)
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

// StartMessageTranslator starts the default message translator used by the pipeline.
func StartMessageTranslator(inputChan chan *message.Message, outputChan chan *message.StatefulMessage) {
	translator := NewMessageTranslator("logs", rtokenizer.NewRustTokenizer())
	translated := translator.Start(inputChan, cap(outputChan))
	go func() {
		for msg := range translated {
			outputChan <- msg
		}
		close(outputChan)
	}()
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
			content := getTranslatorContent(msg)
			if len(content) == 0 {
				return // skip empty messages — no sidecar entry, no alignment break
			}
			entry := batchEntry{msg: msg}
			if mt.jsonLogsAsRaw && len(content) > 0 && content[0] == '{' {
				// json_as_raw: bypass stateful encoding for JSON logs entirely.
				// Send as RawLog — no tokenization, no clustering, no snapshot state.
				entry.content = string(content)
				entry.isRawJSON = true
			} else if results := processor.PreprocessJSON(content); results.Message != "" {
				entry.content = results.Message
				entry.messageKey = results.MessageKey
				entry.jsonContextKeys = results.JSONContextKeys
				entry.jsonContextValues = results.JSONContextValues
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
// Entries with isRawJSON=true bypass tokenization and are sent as RawLog datums directly.
func (mt *MessageTranslator) processBatch(batch []batchEntry, outputChan chan *message.StatefulMessage) {
	if len(batch) == 0 {
		return
	}

	// Partition: send raw JSON entries immediately, collect the rest for batch tokenization.
	tokenBatch := batch[:0:0]
	for _, entry := range batch {
		if entry.isRawJSON {
			ts := getMessageTimestamp(entry.msg)
			service, serviceDictID, serviceIsNew := mt.buildServiceField(entry.msg)
			if serviceIsNew {
				mt.sendDictEntryDefine(outputChan, entry.msg, serviceDictID, entry.msg.Origin.Service())
			}
			statusDictID, status, statusIsNew := mt.buildStatusField(entry.msg)
			if statusIsNew {
				mt.sendDictEntryDefine(outputChan, entry.msg, statusDictID, status)
			}
			tagSet, allTagsStr, dictID, isNew := mt.buildTagSet(entry.msg)
			if isNew {
				mt.sendDictEntryDefine(outputChan, entry.msg, dictID, allTagsStr)
			}
			mt.sendRawLog(outputChan, entry.msg, entry.content, ts, tagSet, service, statusDictID)
		} else {
			tokenBatch = append(tokenBatch, entry)
		}
	}

	if len(tokenBatch) == 0 {
		return
	}

	// Extract content strings for batch tokenization (aligned 1:1 with tokenBatch entries).
	contents := make([]string, len(tokenBatch))
	for i, e := range tokenBatch {
		contents[i] = e.content
	}

	// One TokenizeBatch call for the entire batch.
	// With the serial fallback (current): same per-log CGo cost as before.
	// With a future patterns_tokenize_logs_batch FFI: one LockOSThread + one Rust call.
	tokenResults, _ := mt.tokenizer.TokenizeBatch(contents)

	// Process each entry sequentially — clustering is stateful and must be sequential.
	// Alignment is guaranteed: tokenResults[i] corresponds to tokenBatch[i].
	for i, entry := range tokenBatch {
		if i >= len(tokenResults) {
			break
		}
		if tokenResults[i].Err != nil {
			log.Warnf("Failed to tokenize log message: %v", tokenResults[i].Err)
			continue
		}
		mt.processPreTokenized(entry.msg, tokenResults[i].TokenList, entry.messageKey, entry.jsonContextKeys, entry.jsonContextValues, outputChan)
	}
}

// processMessage is retained for testing and direct single-message use.
// The Start loop now uses the batch pipeline (processBatch → processPreTokenized).
func (mt *MessageTranslator) processMessage(msg *message.Message, outputChan chan *message.StatefulMessage) {
	content := getTranslatorContent(msg)
	if len(content) == 0 {
		return
	}
	if mt.jsonLogsAsRaw && len(content) > 0 && content[0] == '{' {
		ts := getMessageTimestamp(msg)
		service, serviceDictID, serviceIsNew := mt.buildServiceField(msg)
		if serviceIsNew {
			mt.sendDictEntryDefine(outputChan, msg, serviceDictID, msg.Origin.Service())
		}
		statusDictID, status, statusIsNew := mt.buildStatusField(msg)
		if statusIsNew {
			mt.sendDictEntryDefine(outputChan, msg, statusDictID, status)
		}
		tagSet, allTagsStr, dictID, isNew := mt.buildTagSet(msg)
		if isNew {
			mt.sendDictEntryDefine(outputChan, msg, dictID, allTagsStr)
		}
		mt.sendRawLog(outputChan, msg, string(content), ts, tagSet, service, statusDictID)
		return
	}
	contentStr := string(content)
	var messageKey string
	var jsonContextKeys []string
	var jsonContextValues []interface{}
	if results := processor.PreprocessJSON(content); results.Message != "" {
		contentStr = results.Message
		messageKey = results.MessageKey
		jsonContextKeys = results.JSONContextKeys
		jsonContextValues = results.JSONContextValues
	}
	tokenList, err := mt.tokenizer.Tokenize(contentStr)
	if err != nil {
		log.Warnf("Failed to tokenize log message: %v", err)
		return
	}
	mt.processPreTokenized(msg, tokenList, messageKey, jsonContextKeys, jsonContextValues, outputChan)
}

// processPreTokenized handles post-tokenization: clustering, eviction, datum construction, and sending.
// Called by both processBatch (batch pipeline) and processMessage (single-message path).
func (mt *MessageTranslator) processPreTokenized(msg *message.Message, tokenList *token.TokenList, messageKey string, jsonContextKeys []string, jsonContextValues []interface{}, outputChan chan *message.StatefulMessage) {
	var patternDefineSent bool
	var patternDefineParamCount uint32

	ts := getMessageTimestamp(msg)

	// Process tokenized log through cluster manager to get/create pattern
	pattern, changeType, patternCount, estimatedBytes := mt.clusterManager.Add(tokenList)

	// Log exceeds max_template_bytes — send as RawLog, don't store any pattern state.
	if changeType == clustering.PatternTooLarge {
		service, serviceDictID, serviceIsNew := mt.buildServiceField(msg)
		if serviceIsNew {
			mt.sendDictEntryDefine(outputChan, msg, serviceDictID, msg.Origin.Service())
		}
		statusDictID, status, statusIsNew := mt.buildStatusField(msg)
		if statusIsNew {
			mt.sendDictEntryDefine(outputChan, msg, statusDictID, status)
		}
		tagSet, allTagsStr, dictID, isNew := mt.buildTagSet(msg)
		if isNew {
			mt.sendDictEntryDefine(outputChan, msg, dictID, allTagsStr)
		}
		mt.sendRawLog(outputChan, msg, string(getTranslatorContent(msg)), ts, tagSet, service, statusDictID)
		return
	}

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
	mt.touchCurrentMessageDictReferences(msg, messageKey, jsonContextKeys)
	tagCount := mt.tagManager.Count()
	tagMemoryBytes := mt.tagManager.EstimatedMemoryBytes()
	tagCountOverLimit, tagBytesOverLimit := mt.tagEvictionManager.ShouldEvict(tagCount, tagMemoryBytes)
	if tagCountOverLimit || tagBytesOverLimit {
		mt.sendDictEntryDeletes(outputChan, msg, mt.tagEvictionManager.Evict(mt.tagManager, tagCount, tagMemoryBytes, tagCountOverLimit, tagBytesOverLimit))
	}

	// Periodic TTL sweep: remove entries not accessed within the configured TTL.
	// This prevents stale entries from accumulating in state and inflating snapshot replays.
	if time.Since(mt.lastStaleSweep) >= staleSweepInterval {
		mt.lastStaleSweep = time.Now()

		for _, evictedPattern := range mt.clusterManager.EvictStalePatterns(mt.staleTTL) {
			mt.sendPatternDelete(evictedPattern.PatternID, msg, outputChan)
		}
		mt.sendDictEntryDeletes(outputChan, msg, mt.tagManager.EvictStaleEntries(mt.staleTTL))
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
	// Repeated low-cardinality values can enter the dictionary once they cross the
	// admission threshold. One-off or filtered values stay inline to avoid unbounded
	// TagManager growth.
	n := len(wildcardValues)
	dvBacking := make([]statefulpb.DynamicValue, n)
	typeBacking := make([]dvTypeBackings, n)
	dynamicValues := make([]*statefulpb.DynamicValue, n)
	for i := range dvBacking {
		dynamicValues[i] = &dvBacking[i]
	}
	for i, val := range wildcardValues {
		dictID, dictValue, isNew := mt.fillWildcardDynamicValue(&dvBacking[i], &typeBacking[i].intOneof, &typeBacking[i].dictOneof, &typeBacking[i].stringOneof, val)
		if isNew {
			mt.sendDictEntryDefine(outputChan, msg, dictID, dictValue)
		}
	}

	// Encode JSON context schema as state and values as DynamicValues.
	var messageKeyDV *statefulpb.DynamicValue
	var jsonContextSchemaID uint64
	var jsonContextValuesDV []*statefulpb.DynamicValue
	var compactJSONContext compactJSONContextValues
	if messageKey != "" || len(jsonContextKeys) > 0 {
		messageKeyDV, jsonContextSchemaID = mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, messageKey, jsonContextKeys)

		var dictDefs []dictEntryDefinition
		compactJSONContext, dictDefs = mt.compactJSONContextValues(jsonContextValues)
		for _, dictDef := range dictDefs {
			mt.sendDictEntryDefine(outputChan, msg, dictDef.id, dictDef.value)
		}

		// Keep the legacy field empty for FlatLog. Consumers that do not understand the compact
		// streams should ignore the json schema when json_context_values is absent.
		jsonContextValuesDV = nil
	}

	service, serviceDictID, serviceIsNew := mt.buildServiceField(msg)
	if serviceIsNew {
		mt.sendDictEntryDefine(outputChan, msg, serviceDictID, msg.Origin.Service())
	}

	statusDictID, status, statusIsNew := mt.buildStatusField(msg)
	if statusIsNew {
		mt.sendDictEntryDefine(outputChan, msg, statusDictID, status)
	}

	// Build complete tag list and encode as TagSet
	tagSet, allTagsString, dictID, isNew := mt.buildTagSet(msg)
	if isNew {
		mt.sendDictEntryDefine(outputChan, msg, dictID, allTagsString)
	}

	// Send StructuredLog with all fields
	tsMillis := ts.UnixNano() / nanoToMillis
	mt.sendStructuredLog(outputChan, msg, tsMillis, patternID, dynamicValues, tagSet, service, statusDictID, messageKeyDV, jsonContextSchemaID, jsonContextValuesDV, compactJSONContext)
}

// buildTagSet constructs the complete tag list for a message and encodes it as a TagSet.
// This includes log-level fields (hostname, ddsource) as tags,
// plus all other tags from the message metadata (container tags, source config tags, processing tags).
// All tags are joined as a single string, encoded as a single dictionary entry in the TagSet.
// A single-entry cache keyed on (origin ptr, hostname, source, tags) avoids all
// allocations in the common case where these inputs are constant across messages (single-source pipeline).
func (mt *MessageTranslator) buildTagSet(msg *message.Message) (*statefulpb.TagSet, string, uint64, bool) {
	// Read current inputs
	currentOrigin := msg.Origin
	currentHostname := msg.MessageMetadata.Hostname
	currentSource := msg.Origin.Source()
	currentTagsString := msg.MessageMetadata.TagsToString()
	currentProcessingTags := strings.Join(msg.MessageMetadata.ProcessingTags, ",")

	// Cache hit: all inputs identical and cached dict index still live (not evicted).
	if mt.tagCache.tagSet != nil &&
		mt.tagCache.origin == currentOrigin &&
		mt.tagCache.hostname == currentHostname &&
		mt.tagCache.source == currentSource &&
		mt.tagCache.tagsString == currentTagsString &&
		mt.tagCache.processingTags == currentProcessingTags &&
		mt.tagManager.TouchDictID(mt.tagCache.dictID) {
		return mt.tagCache.tagSet, mt.tagCache.tagStr, mt.tagCache.dictID, false
	}

	// Cache miss: build tag string normally.

	// Start with metadata tags (container tags, source config tags, processing tags)
	baseTags := msg.MessageMetadata.Tags()
	tagStrings := make([]string, len(baseTags), len(baseTags)+4)
	copy(tagStrings, baseTags)

	// Add log-level fields as tags (these are separate JSON fields in HTTP pipeline).
	// Service is now encoded in the dedicated top-level proto field instead of the joined tagset.

	if currentHostname != "" {
		tagStrings = append(tagStrings, "hostname:"+currentHostname)
	}

	if currentSource != "" {
		tagStrings = append(tagStrings, "ddsource:"+currentSource)
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
	mt.tagCache.source = currentSource
	mt.tagCache.tagsString = currentTagsString
	mt.tagCache.processingTags = currentProcessingTags
	mt.tagCache.tagSet = tagSet
	mt.tagCache.dictID = dictID
	mt.tagCache.tagStr = allTagsString

	return tagSet, allTagsString, dictID, isNew
}

// invalidateTagCache clears the in-memory tag cache when it references dictID.
// Used when dictionary entries are removed out-of-band (e.g. TTL eviction).
func (mt *MessageTranslator) invalidateTagCache(dictID uint64) {
	if mt.tagCache.dictID != dictID {
		return
	}
	mt.tagCache.tagSet = nil
	mt.tagCache.dictID = 0
	mt.tagCache.tagStr = ""
}

func (mt *MessageTranslator) touchCurrentMessageDictReferences(msg *message.Message, messageKey string, jsonContextKeys []string) {
	mt.touchJsonSchemaReferencesForKeys(messageKey, jsonContextKeys)
	mt.touchCurrentService(msg)
	mt.touchCurrentStatus(msg)
	mt.touchCurrentTagSet(msg)
}

func (mt *MessageTranslator) touchCurrentService(msg *message.Message) {
	if msg.Origin == nil {
		return
	}
	service := msg.Origin.Service()
	if service == "" {
		return
	}
	if id, ok := mt.tagManager.GetStringID(service); ok {
		mt.tagManager.TouchDictID(id)
	}
}

func (mt *MessageTranslator) touchCurrentStatus(msg *message.Message) {
	status := msg.MessageMetadata.GetStatus()
	if status == "" {
		return
	}
	if id, ok := mt.tagManager.GetStringID(status); ok {
		mt.tagManager.TouchDictID(id)
	}
}

func (mt *MessageTranslator) touchCurrentTagSet(msg *message.Message) {
	if mt.tagCache.tagSet == nil || msg.Origin == nil {
		return
	}
	currentProcessingTags := strings.Join(msg.MessageMetadata.ProcessingTags, ",")
	if mt.tagCache.origin != msg.Origin ||
		mt.tagCache.hostname != msg.MessageMetadata.Hostname ||
		mt.tagCache.source != msg.Origin.Source() ||
		mt.tagCache.processingTags != currentProcessingTags {
		return
	}
	mt.tagManager.TouchDictID(mt.tagCache.dictID)
}

func (mt *MessageTranslator) buildServiceField(msg *message.Message) (*statefulpb.DynamicValue, uint64, bool) {
	if msg.Origin == nil {
		return nil, 0, false
	}
	service := msg.Origin.Service()
	if service == "" {
		return nil, 0, false
	}
	dictID, isNew := mt.tagManager.AddString(service)
	return &statefulpb.DynamicValue{
		Value: &statefulpb.DynamicValue_DictIndex{
			DictIndex: dictID,
		},
	}, dictID, isNew
}

func (mt *MessageTranslator) buildStatusField(msg *message.Message) (uint64, string, bool) {
	status := msg.MessageMetadata.GetStatus()
	if status == "" {
		return 0, "", false
	}
	dictID, isNew := mt.tagManager.AddString(status)
	return dictID, status, isNew
}

func (mt *MessageTranslator) sendJsonSchemaDefineIfNeeded(outputChan chan *message.StatefulMessage, msg *message.Message, messageKey string, keys []string) (*statefulpb.DynamicValue, uint64) {
	if messageKey == "" {
		messageKey = "message"
	}
	messageKeyID, messageKeyIsNew := mt.tagManager.AddString(messageKey)
	if messageKeyIsNew {
		mt.sendDictEntryDefine(outputChan, msg, messageKeyID, messageKey)
	}

	keyIDs := make([]uint64, 0, len(keys))
	for _, key := range keys {
		keyID, keyIsNew := mt.tagManager.AddString(key)
		if keyIsNew {
			mt.sendDictEntryDefine(outputChan, msg, keyID, key)
		}
		keyIDs = append(keyIDs, keyID)
	}

	schemaKey := buildJsonSchemaKey(messageKey, keys)
	schemaID, ok := mt.jsonSchemaToID[schemaKey]
	if ok && !mt.jsonSchemaMatchesDictIDs(schemaID, messageKeyID, keyIDs) {
		mt.sendJsonSchemaDelete(outputChan, msg, schemaID)
		mt.untrackJsonSchema(schemaID)
		ok = false
	}
	if !ok {
		mt.nextJSONSchemaID++
		schemaID = mt.nextJSONSchemaID
		mt.jsonSchemaToID[schemaKey] = schemaID
		mt.trackJsonSchema(schemaID, schemaKey, messageKeyID, keyIDs)
		mt.sendJsonSchemaDefine(outputChan, msg, schemaID, keyIDs, messageKeyID)
	}

	return &statefulpb.DynamicValue{
		Value: &statefulpb.DynamicValue_DictIndex{
			DictIndex: messageKeyID,
		},
	}, schemaID
}

func (mt *MessageTranslator) jsonSchemaMatchesDictIDs(schemaID uint64, messageKeyID uint64, keyIDs []uint64) bool {
	state := mt.jsonSchemaByID[schemaID]
	if state == nil || state.messageKeyID != messageKeyID || len(state.keyIDs) != len(keyIDs) {
		return false
	}
	for i, keyID := range keyIDs {
		if state.keyIDs[i] != keyID {
			return false
		}
	}
	return true
}

func buildJsonSchemaKey(messageKey string, keys []string) string {
	schemaKeyBuilder := strings.Builder{}
	schemaKeyBuilder.WriteString(messageKey)
	for _, key := range keys {
		schemaKeyBuilder.WriteByte('\x00')
		schemaKeyBuilder.WriteString(key)
	}
	return schemaKeyBuilder.String()
}

func (mt *MessageTranslator) trackJsonSchema(schemaID uint64, schemaKey string, messageKeyID uint64, keyIDs []uint64) {
	state := &jsonSchemaState{
		schemaID:     schemaID,
		schemaKey:    schemaKey,
		messageKeyID: messageKeyID,
		keyIDs:       append([]uint64(nil), keyIDs...),
	}
	mt.jsonSchemaByID[schemaID] = state
	mt.addJsonSchemaDictReference(messageKeyID, schemaID)
	for _, keyID := range keyIDs {
		mt.addJsonSchemaDictReference(keyID, schemaID)
	}
}

func (mt *MessageTranslator) addJsonSchemaDictReference(dictID uint64, schemaID uint64) {
	schemaIDs := mt.jsonSchemaIDsByDictID[dictID]
	if schemaIDs == nil {
		schemaIDs = make(map[uint64]struct{})
		mt.jsonSchemaIDsByDictID[dictID] = schemaIDs
	}
	schemaIDs[schemaID] = struct{}{}
}

func (mt *MessageTranslator) touchJsonSchemaReferences(schemaID uint64) {
	state := mt.jsonSchemaByID[schemaID]
	if state == nil {
		return
	}
	mt.tagManager.TouchDictID(state.messageKeyID)
	for _, keyID := range state.keyIDs {
		mt.tagManager.TouchDictID(keyID)
	}
}

func (mt *MessageTranslator) touchJsonSchemaReferencesForKeys(messageKey string, keys []string) {
	if messageKey == "" && len(keys) == 0 {
		return
	}
	if messageKey == "" {
		messageKey = "message"
	}
	schemaKey := buildJsonSchemaKey(messageKey, keys)
	schemaID, ok := mt.jsonSchemaToID[schemaKey]
	if !ok {
		return
	}
	mt.touchJsonSchemaReferences(schemaID)
}

func (mt *MessageTranslator) sendDictEntryDeletes(outputChan chan *message.StatefulMessage, msg *message.Message, evictedIDs []uint64) {
	for _, evictedID := range evictedIDs {
		mt.sendJsonSchemaDeletesForDictID(outputChan, msg, evictedID)
		mt.sendDictEntryDelete(outputChan, msg, evictedID)
	}
}

func (mt *MessageTranslator) sendJsonSchemaDeletesForDictID(outputChan chan *message.StatefulMessage, msg *message.Message, dictID uint64) {
	for schemaID := range mt.jsonSchemaIDsByDictID[dictID] {
		mt.sendJsonSchemaDelete(outputChan, msg, schemaID)
		mt.untrackJsonSchema(schemaID)
	}
}

func (mt *MessageTranslator) untrackJsonSchema(schemaID uint64) {
	state := mt.jsonSchemaByID[schemaID]
	if state == nil {
		return
	}
	delete(mt.jsonSchemaByID, schemaID)
	delete(mt.jsonSchemaToID, state.schemaKey)
	mt.removeJsonSchemaDictReference(state.messageKeyID, schemaID)
	for _, keyID := range state.keyIDs {
		mt.removeJsonSchemaDictReference(keyID, schemaID)
	}
}

func (mt *MessageTranslator) removeJsonSchemaDictReference(dictID uint64, schemaID uint64) {
	schemaIDs := mt.jsonSchemaIDsByDictID[dictID]
	if schemaIDs == nil {
		return
	}
	delete(schemaIDs, schemaID)
	if len(schemaIDs) == 0 {
		delete(mt.jsonSchemaIDsByDictID, dictID)
	}
}

// getMessageTimestamp returns the timestamp for the message, preferring the HTTP
// encoder timestamp when available so the dual-send paths stay aligned.
func getMessageTimestamp(msg *message.Message) time.Time {
	if msg.MessageMetadata.EncodedTimestampMs != 0 {
		return time.UnixMilli(msg.MessageMetadata.EncodedTimestampMs).UTC()
	}
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		return msg.ServerlessExtra.Timestamp
	}
	return time.Now().UTC()
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

func (mt *MessageTranslator) sendJsonSchemaDelete(outputChan chan *message.StatefulMessage, msg *message.Message, schemaID uint64) {
	deleteDatum := buildJsonSchemaDelete(schemaID)

	bytesRemoved := float64(proto.Size(deleteDatum))
	tlmPipelineStateSize.Sub(bytesRemoved, mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    deleteDatum,
		Metadata: &msg.MessageMetadata,
	}
}

func (mt *MessageTranslator) sendJsonSchemaDefine(outputChan chan *message.StatefulMessage, msg *message.Message, schemaID uint64, keyIDs []uint64, messageKeyID uint64) {
	schemaDatum := buildJsonSchemaDefine(schemaID, keyIDs, messageKeyID)

	bytesAdded := float64(proto.Size(schemaDatum))
	tlmPipelineStateSize.Add(bytesAdded, mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    schemaDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendRawLog creates and sends a raw log datum (currently unused)
func (mt *MessageTranslator) sendRawLog(outputChan chan *message.StatefulMessage, msg *message.Message, contentStr string, ts time.Time, tagSet *statefulpb.TagSet, service *statefulpb.DynamicValue, statusDictID uint64) {
	// Proto3 string fields require valid UTF-8; replace invalid sequences to avoid corrupt datums.
	logDatum := buildRawLog(toValidUTF8(contentStr), ts, tagSet, msg.MessageMetadata.DualSendUUID, service, statusDictID)

	tlmPipelineRawLogsProcessed.Inc(mt.pipelineName)
	tlmPipelineRawLogsProcessedBytes.Add(float64(proto.Size(logDatum)), mt.pipelineName)

	outputChan <- &message.StatefulMessage{
		Datum:    logDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendStructuredLog creates and sends a StructuredLog datum
func (mt *MessageTranslator) sendStructuredLog(outputChan chan *message.StatefulMessage, msg *message.Message, timestamp int64, patternID uint64, dynamicValues []*statefulpb.DynamicValue, tagSet *statefulpb.TagSet, service *statefulpb.DynamicValue, statusDictID uint64, messageKey *statefulpb.DynamicValue, jsonContextSchemaID uint64, jsonContextValues []*statefulpb.DynamicValue, compactJSONContext compactJSONContextValues) {
	logDatum := buildStructuredLog(timestamp, patternID, dynamicValues, tagSet, msg.MessageMetadata.DualSendUUID, service, statusDictID, messageKey, jsonContextSchemaID, jsonContextValues, compactJSONContext)

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

func buildJsonSchemaDefine(schemaID uint64, keyIDs []uint64, messageKeyID uint64) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_JsonSchemaDefine{
			JsonSchemaDefine: &statefulpb.JsonSchemaDefine{
				SchemaId:     schemaID,
				Keys:         keyIDs,
				MessageKeyId: messageKeyID,
			},
		},
	}
}

func buildJsonSchemaDelete(schemaID uint64) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_JsonSchemaDelete{
			JsonSchemaDelete: &statefulpb.JsonSchemaDelete{
				SchemaId: schemaID,
			},
		},
	}
}

// parseLosslessIntString returns an int64 only when the original string is already
// the canonical base-10 representation of that integer. Numeric-looking strings
// like "00123" are kept as strings so they can round-trip without losing lexeme
// fidelity when reconstructed downstream.
func parseLosslessIntString(value string) (int64, bool) {
	intVal, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	if strconv.FormatInt(intVal, 10) != value {
		return 0, false
	}
	return intVal, true
}

// parseLosslessFloatString returns a float64 only when the original string is already
// the canonical representation produced by strconv.FormatFloat(..., 'g', -1, 64).
func parseLosslessFloatString(value string) (float64, bool) {
	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	if strconv.FormatFloat(floatVal, 'g', -1, 64) != value {
		return 0, false
	}
	return floatVal, true
}

func parseLosslessBoolString(value string) (bool, bool) {
	switch value {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// encodeDynamicValue encodes a wildcard value with type inference.
// Priority: canonical int64 → dict_index (via tagManager)
// Returns the encoded DynamicValue and whether a new dict entry was created
func (mt *MessageTranslator) encodeDynamicValue(value string) (*statefulpb.DynamicValue, uint64, bool) {
	if intVal, ok := parseLosslessIntString(value); ok {
		return &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_IntValue{
				IntValue: intVal,
			},
			RenderAsString: true,
		}, 0, false
	}
	if floatVal, ok := parseLosslessFloatString(value); ok {
		return &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_FloatValue{
				FloatValue: floatVal,
			},
			RenderAsString: true,
		}, 0, false
	}
	if boolVal, ok := parseLosslessBoolString(value); ok {
		return &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_BoolValue{
				BoolValue: boolVal,
			},
			RenderAsString: true,
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

// fillDynamicValue fills a pre-allocated DynamicValue in-place for a typed JSON context value.
// Primitive JSON types preserve their native type; nested objects/arrays arrive as JSON strings.
// String values may use numeric or bool encodings with render_as_string when they can round-trip exactly.
func (mt *MessageTranslator) fillDynamicValue(
	dv *statefulpb.DynamicValue,
	oneofInt *statefulpb.DynamicValue_IntValue,
	oneofFloat *statefulpb.DynamicValue_FloatValue,
	oneofBool *statefulpb.DynamicValue_BoolValue,
	oneofDict *statefulpb.DynamicValue_DictIndex,
	oneofRawJSON *statefulpb.DynamicValue_RawJsonValue,
	oneofStr *statefulpb.DynamicValue_StringValue,
	value interface{},
) (dictID uint64, dictValue string, isNew bool) {
	dv.RenderAsString = false
	switch typed := value.(type) {
	case nil:
		dv.Value = nil
		return 0, "", false
	case string:
		if intVal, ok := parseLosslessIntString(typed); ok {
			oneofInt.IntValue = intVal
			dv.Value = oneofInt
			dv.RenderAsString = true
			return 0, "", false
		}
		if floatVal, ok := parseLosslessFloatString(typed); ok {
			oneofFloat.FloatValue = floatVal
			dv.Value = oneofFloat
			dv.RenderAsString = true
			return 0, "", false
		}
		if boolVal, ok := parseLosslessBoolString(typed); ok {
			oneofBool.BoolValue = boolVal
			dv.Value = oneofBool
			dv.RenderAsString = true
			return 0, "", false
		}
		typed = toValidUTF8(typed)
		if dictID, isNew, shouldEncode := mt.tagManager.ObserveDynamicString(typed); shouldEncode {
			oneofDict.DictIndex = dictID
			dv.Value = oneofDict
			return dictID, typed, isNew
		}
		oneofStr.StringValue = typed
		dv.Value = oneofStr
		return 0, "", false
	case json.Number:
		raw := typed.String()
		if intVal, ok := parseLosslessIntString(raw); ok {
			oneofInt.IntValue = intVal
			dv.Value = oneofInt
			return 0, "", false
		}
		if floatVal, ok := parseLosslessFloatString(raw); ok {
			oneofFloat.FloatValue = floatVal
			dv.Value = oneofFloat
			return 0, "", false
		}
		oneofRawJSON.RawJsonValue = []byte(raw)
		dv.Value = oneofRawJSON
		return 0, "", false
	case float64:
		if !math.IsInf(typed, 0) && !math.IsNaN(typed) && math.Trunc(typed) == typed && typed >= math.MinInt64 && typed <= math.MaxInt64 {
			oneofInt.IntValue = int64(typed)
			dv.Value = oneofInt
			return 0, "", false
		}
		oneofFloat.FloatValue = typed
		dv.Value = oneofFloat
		return 0, "", false
	case bool:
		oneofBool.BoolValue = typed
		dv.Value = oneofBool
		return 0, "", false
	default:
		rawJSON, err := json.Marshal(typed)
		if err != nil {
			log.Warnf("Failed to marshal nested JSON context value: %v", err)
			oneofStr.StringValue = ""
			dv.Value = oneofStr
			return 0, "", false
		}
		oneofRawJSON.RawJsonValue = rawJSON
		dv.Value = oneofRawJSON
		return 0, "", false
	}
}

func (mt *MessageTranslator) compactJSONContextValues(values []interface{}) (compactJSONContextValues, []dictEntryDefinition) {
	compact := compactJSONContextValues{kinds: make([]byte, 0, len(values))}
	dictDefs := make([]dictEntryDefinition, 0)
	for _, value := range values {
		dictID, dictValue, isNew := mt.appendCompactJSONContextValue(&compact, value)
		if isNew {
			dictDefs = append(dictDefs, dictEntryDefinition{id: dictID, value: dictValue})
		}
	}
	return compact, dictDefs
}

func (mt *MessageTranslator) appendCompactJSONContextValue(compact *compactJSONContextValues, value interface{}) (dictID uint64, dictValue string, isNew bool) {
	switch typed := value.(type) {
	case nil:
		compact.kinds = append(compact.kinds, jsonValueKindNull)
		return 0, "", false
	case string:
		return mt.appendCompactJSONString(compact, typed)
	case json.Number:
		return appendCompactJSONNumber(compact, typed.String())
	case float64:
		if !math.IsInf(typed, 0) && !math.IsNaN(typed) && math.Trunc(typed) == typed && typed >= math.MinInt64 && typed <= math.MaxInt64 {
			compact.kinds = append(compact.kinds, jsonValueKindInt)
			compact.ints = append(compact.ints, int64(typed))
			return 0, "", false
		}
		compact.kinds = append(compact.kinds, jsonValueKindFloat)
		compact.floats = append(compact.floats, typed)
		return 0, "", false
	case bool:
		if typed {
			compact.kinds = append(compact.kinds, jsonValueKindBoolTrue)
		} else {
			compact.kinds = append(compact.kinds, jsonValueKindBoolFalse)
		}
		return 0, "", false
	default:
		rawJSON, err := json.Marshal(typed)
		if err != nil {
			log.Warnf("Failed to marshal nested JSON context value: %v", err)
			compact.kinds = append(compact.kinds, jsonValueKindString)
			compact.stringValues = append(compact.stringValues, "")
			return 0, "", false
		}
		compact.kinds = append(compact.kinds, jsonValueKindRaw)
		compact.rawValues = append(compact.rawValues, rawJSON)
		return 0, "", false
	}
}

func (mt *MessageTranslator) appendCompactJSONString(compact *compactJSONContextValues, value string) (dictID uint64, dictValue string, isNew bool) {
	if intVal, ok := parseLosslessIntString(value); ok {
		compact.kinds = append(compact.kinds, jsonValueKindIntAsString)
		compact.ints = append(compact.ints, intVal)
		return 0, "", false
	}
	if floatVal, ok := parseLosslessFloatString(value); ok {
		compact.kinds = append(compact.kinds, jsonValueKindFloatAsString)
		compact.floats = append(compact.floats, floatVal)
		return 0, "", false
	}
	if boolVal, ok := parseLosslessBoolString(value); ok {
		if boolVal {
			compact.kinds = append(compact.kinds, jsonValueKindBoolTrueAsString)
		} else {
			compact.kinds = append(compact.kinds, jsonValueKindBoolFalseAsString)
		}
		return 0, "", false
	}
	value = toValidUTF8(value)
	if dictID, isNew, shouldEncode := mt.tagManager.ObserveDynamicString(value); shouldEncode {
		compact.kinds = append(compact.kinds, jsonValueKindDict)
		compact.dicts = append(compact.dicts, dictID)
		return dictID, value, isNew
	}
	compact.kinds = append(compact.kinds, jsonValueKindString)
	compact.stringValues = append(compact.stringValues, value)
	return 0, "", false
}

func appendCompactJSONNumber(compact *compactJSONContextValues, raw string) (dictID uint64, dictValue string, isNew bool) {
	if intVal, ok := parseLosslessIntString(raw); ok {
		compact.kinds = append(compact.kinds, jsonValueKindInt)
		compact.ints = append(compact.ints, intVal)
		return 0, "", false
	}
	if floatVal, ok := parseLosslessFloatString(raw); ok {
		compact.kinds = append(compact.kinds, jsonValueKindFloat)
		compact.floats = append(compact.floats, floatVal)
		return 0, "", false
	}
	compact.kinds = append(compact.kinds, jsonValueKindRaw)
	compact.rawValues = append(compact.rawValues, []byte(raw))
	return 0, "", false
}

func (mt *MessageTranslator) fillWildcardDynamicValue(
	dv *statefulpb.DynamicValue,
	oneofInt *statefulpb.DynamicValue_IntValue,
	oneofDict *statefulpb.DynamicValue_DictIndex,
	oneofStr *statefulpb.DynamicValue_StringValue,
	value string,
) (dictID uint64, dictValue string, isNew bool) {
	dv.RenderAsString = false
	// Only canonical base-10 integers are safe to encode numerically without
	// changing the original token's lexeme.
	if intVal, ok := parseLosslessIntString(value); ok {
		oneofInt.IntValue = intVal
		dv.Value = oneofInt
		dv.RenderAsString = true
		return 0, "", false
	}
	value = toValidUTF8(value)
	if dictID, isNew, shouldEncode := mt.tagManager.ObserveDynamicString(value); shouldEncode {
		oneofDict.DictIndex = dictID
		dv.Value = oneofDict
		return dictID, value, isNew
	}
	// New or filtered value: send inline as string_value — no dict entry created.
	// Proto3 requires valid UTF-8; invalid sequences were already replaced above.
	oneofStr.StringValue = value
	dv.Value = oneofStr
	return 0, "", false
}

func flatLogTagSetDictIndex(tagSet *statefulpb.TagSet) uint64 {
	if tagSet == nil || tagSet.Tagset == nil {
		return 0
	}
	return tagSet.Tagset.GetDictIndex()
}

func flatLogDynamicValueDictIndex(value *statefulpb.DynamicValue) uint64 {
	if value == nil {
		return 0
	}
	return value.GetDictIndex()
}

// buildStructuredLog creates a Datum containing a FlatLog with pattern references.
func buildStructuredLog(timestamp int64, patternID uint64, dynamicValues []*statefulpb.DynamicValue, tagSet *statefulpb.TagSet, uuid string, service *statefulpb.DynamicValue, statusDictID uint64, messageKey *statefulpb.DynamicValue, jsonContextSchemaID uint64, jsonContextValues []*statefulpb.DynamicValue, compactJSONContext compactJSONContextValues) *statefulpb.Datum {
	_ = messageKey
	log := &statefulpb.FlatLog{
		Timestamp: timestamp,
		Status:    flatLogDictIndex(statusDictID),
		Service:   flatLogDictIndex(flatLogDynamicValueDictIndex(service)),
		Tags:      flatLogDictIndex(flatLogTagSetDictIndex(tagSet)),

		PatternId:               patternID,
		DynamicValues:           dynamicValues,
		JsonSchemaId:            flatLogDictIndex(jsonContextSchemaID),
		JsonContextValues:       jsonContextValues,
		JsonContextValueKinds:   compactJSONContext.kinds,
		JsonContextIntValues:    compactJSONContext.ints,
		JsonContextFloatValues:  compactJSONContext.floats,
		JsonContextDictValues:   compactJSONContext.dicts,
		JsonContextRawValues:    compactJSONContext.rawValues,
		JsonContextStringValues: compactJSONContext.stringValues,
	}
	if uuid != "" {
		log.Uuid = &uuid
	}
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_FlatLog{
			FlatLog: log,
		},
	}
}

// buildRawLog creates a Datum containing a FlatLog with raw content (no pattern).
func buildRawLog(content string, ts time.Time, tagSet *statefulpb.TagSet, uuid string, service *statefulpb.DynamicValue, statusDictID uint64) *statefulpb.Datum {
	log := &statefulpb.FlatLog{
		Timestamp: ts.UnixNano() / nanoToMillis,
		Status:    flatLogDictIndex(statusDictID),
		Service:   flatLogDictIndex(flatLogDynamicValueDictIndex(service)),
		Tags:      flatLogDictIndex(flatLogTagSetDictIndex(tagSet)),
		RawLog:    content,

		JsonSchemaId: flatLogEmptyDictIndex,
	}
	if uuid != "" {
		log.Uuid = &uuid
	}
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_FlatLog{
			FlatLog: log,
		},
	}
}
