// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nanoToMillis = 1000000

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	clusterManager *clustering.ClusterManager
}

// NewMessageTranslator creates a new MessageTranslator instance
// If clusterManager is nil, a new one will be created
func NewMessageTranslator(clusterManager *clustering.ClusterManager) *MessageTranslator {
	return &MessageTranslator{
		clusterManager: clustering.NewClusterManager(),
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
	translator := NewMessageTranslator(nil)
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
		log.Debugf("[MSG_TRANSLATOR] Skipping empty message")
		return
	}

	// Tokenize the message content
	contentStr := string(content)
	tokenList := tokenizeMessage(contentStr)

	// Nil check but shouldn't happen at all
	if tokenList == nil || tokenList.IsEmpty() {
		log.Debugf("[MSG_TRANSLATOR] Skipping message with empty token list")
		mt.sendRawLog(outputChan, msg, contentStr, ts)
		return
	}

	// Process tokenized log through cluster manager to get/create pattern
	pattern, changeType := mt.clusterManager.Add(tokenList)
	if pattern == nil {
		log.Debugf("[MSG_TRANSLATOR] No pattern created, sending as raw log")
		mt.sendRawLog(outputChan, msg, contentStr, ts)
		return
	}

	// CRITICAL RACE CONDITION DETECTION: Capture pattern state IMMEDIATELY after Add() returns
	// The pattern pointer is shared and can be modified by other goroutines after ClusterManager's lock is released.
	// Log the state at multiple points to detect if it changes (proving race condition)
	patternID := pattern.PatternID
	capturedParamCount := uint32(pattern.GetWildcardCount())
	capturedTemplateLen := 0
	capturedPositionsLen := 0
	if pattern.Template != nil {
		capturedTemplateLen = pattern.Template.Length()
		capturedPositionsLen = len(pattern.Positions)
	}

	log.Infof("[RACE_DETECT] Step 1 - After Add(): pattern_id=%d paramCount=%d templateLen=%d positionsLen=%d",
		patternID, capturedParamCount, capturedTemplateLen, capturedPositionsLen)

	// Extract wildcard values NOW, using the current pattern state
	// This must happen before handlePatternChange() which might send PatternDefine
	// If pattern is modified by another goroutine between now and then, we'll have inconsistent state
	wildcardValues := mt.extractAndValidateWildcardValues(pattern, tokenList, capturedParamCount)

	log.Infof("[RACE_DETECT] Step 2 - After extract: pattern_id=%d wildcardValuesCount=%d",
		patternID, len(wildcardValues))

	// If wildcardValues is nil, it means tokenList doesn't match template structure
	// Send as raw log instead of StructuredLog to avoid intake errors
	if wildcardValues == nil {
		log.Warnf("[MSG_TRANSLATOR] Pattern mismatch detected for pattern_id=%d - tokenList doesn't match template structure. Sending as raw log instead.",
			patternID)
		mt.sendRawLog(outputChan, msg, contentStr, ts)
		return
	}

	// Always use pattern-based encoding (PatternDefine + StructuredLog)
	// - Patterns without wildcards: param_count=0, dynamic_values=[]
	// - Patterns with wildcards: param_count>0, dynamic_values=[...]
	// This ensures consistent behavior and proper pattern evolution tracking

	// Handle pattern state changes (send PatternDefine/PatternDelete as needed)
	// WARNING: Pattern may have been modified by another goroutine by now!
	// But we've already captured wildcardValues and paramCount, so we're consistent

	// Read pattern state BEFORE handlePatternChange to detect races
	beforeHandleParamCount := uint32(pattern.GetWildcardCount())
	beforeHandleTemplateLen := 0
	if pattern.Template != nil {
		beforeHandleTemplateLen = pattern.Template.Length()
	}

	log.Infof("[RACE_DETECT] Step 3 - Before handlePatternChange: pattern_id=%d paramCount=%d templateLen=%d",
		patternID, beforeHandleParamCount, beforeHandleTemplateLen)

	// Detect if pattern changed between capture and now
	if beforeHandleParamCount != capturedParamCount {
		log.Errorf("[RACE_DETECTED!!!] Pattern modified between Add() and handlePatternChange! pattern_id=%d captured=%d now=%d",
			patternID, capturedParamCount, beforeHandleParamCount)
	}
	if beforeHandleTemplateLen != capturedTemplateLen {
		log.Errorf("[RACE_DETECTED!!!] Template length changed! pattern_id=%d captured=%d now=%d",
			patternID, capturedTemplateLen, beforeHandleTemplateLen)
	}

	mt.handlePatternChange(pattern, changeType, msg, outputChan, &patternDefineSent, &patternDefineParamCount)

	// Read pattern state AFTER handlePatternChange to detect races
	afterHandleParamCount := uint32(pattern.GetWildcardCount())
	afterHandleTemplateLen := 0
	if pattern.Template != nil {
		afterHandleTemplateLen = pattern.Template.Length()
	}

	log.Infof("[RACE_DETECT] Step 4 - After handlePatternChange: pattern_id=%d paramCount=%d templateLen=%d patternDefineSent=%v patternDefineParamCount=%d",
		patternID, afterHandleParamCount, afterHandleTemplateLen, patternDefineSent, patternDefineParamCount)

	// Detect if pattern changed during handlePatternChange
	if afterHandleParamCount != beforeHandleParamCount {
		log.Errorf("[RACE_DETECTED!!!] Pattern modified DURING handlePatternChange! pattern_id=%d before=%d after=%d",
			patternID, beforeHandleParamCount, afterHandleParamCount)
	}

	// Use the captured paramCount to ensure consistency with wildcardValues we extracted
	// If PatternDefine was sent, it might have used updated pattern state, so validate
	if patternDefineSent {
		// PatternDefine was sent - validate that its paramCount matches what we captured
		if patternDefineParamCount != capturedParamCount {
			log.Warnf("[MSG_TRANSLATOR] Pattern paramCount changed during processing! pattern_id=%d captured=%d PatternDefine=%d. Using PatternDefine value.",
				patternID, capturedParamCount, patternDefineParamCount)
			// Use PatternDefine's paramCount, but validate wildcardValues matches
			if uint32(len(wildcardValues)) != patternDefineParamCount {
				log.Errorf("CRITICAL: Race condition detected! pattern_id=%d wildcardValuesCount=%d PatternDefineParamCount=%d | This WILL cause intake error!",
					patternID, len(wildcardValues), patternDefineParamCount)
				// Adjust wildcardValues to match PatternDefine
				wildcardValues = adjustWildcardValuesCount(wildcardValues, int(patternDefineParamCount))
			}
		}
	} else {
		// No PatternDefine sent - use captured paramCount
		patternDefineParamCount = capturedParamCount
	}

	// Always send StructuredLog with pattern_id + dynamic values
	mt.sendStructuredLog(outputChan, msg, pattern, wildcardValues, ts, patternDefineSent, patternDefineParamCount)
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
		mt.sendPatternDefine(pattern, msg, outputChan, "PatternNew", patternDefineSent, patternDefineParamCount)

	case clustering.PatternUpdated:
		// Pattern structure changed (e.g., 0→N wildcards, or N→M wildcards)
		// Since PatternDefine was sent before, we need to delete and redefine
		mt.sendPatternDelete(pattern.PatternID, msg, outputChan)
		mt.sendPatternDefine(pattern, msg, outputChan, "PatternUpdated", patternDefineSent, patternDefineParamCount)

	case clustering.PatternNoChange:
		// Pattern unchanged - no need to send PatternDefine
		// The snapshot already has the current pattern state
		log.Debugf("[MSG_TRANSLATOR] Pattern unchanged for pattern_id=%d, skipping PatternDefine", pattern.PatternID)
	}
}

// sendPatternDefine creates and sends a PatternDefine datum
func (mt *MessageTranslator) sendPatternDefine(pattern *clustering.Pattern, msg *message.Message, outputChan chan *message.StatefulMessage, reason string, patternDefineSent *bool, patternDefineParamCount *uint32) {
	patternDatum := buildPatternDefine(pattern)
	if pd := patternDatum.GetPatternDefine(); pd != nil {
		*patternDefineParamCount = pd.ParamCount
		log.Infof("[MSG_TRANSLATOR] Sending PatternDefine: pattern_id=%d paramCount=%d template=%q (%s)",
			pattern.PatternID, *patternDefineParamCount, pd.Template, reason)
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
	log.Infof("[MSG_TRANSLATOR] Sending PatternDelete: pattern_id=%d", patternID)
	outputChan <- &message.StatefulMessage{
		Datum:    deleteDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// extractAndValidateWildcardValues extracts wildcard values and validates the count
// Returns nil if tokenList doesn't match template structure (caller should send raw log)
func (mt *MessageTranslator) extractAndValidateWildcardValues(pattern *clustering.Pattern, tokenList *token.TokenList, patternDefineParamCount uint32) []string {
	wildcardValues := pattern.GetWildcardValues(tokenList)

	// nil indicates structure mismatch - return early
	if wildcardValues == nil {
		log.Warnf("[MSG_TRANSLATOR] GetWildcardValues returned nil for pattern_id=%d - structure mismatch detected",
			pattern.PatternID)
		return nil
	}

	currentWildcardCount := pattern.GetWildcardCount()

	// Adjust count if mismatch
	if len(wildcardValues) != currentWildcardCount {
		log.Warnf("[MSG_TRANSLATOR] Wildcard values count mismatch for pattern_id=%d: got %d, expected %d",
			pattern.PatternID, len(wildcardValues), currentWildcardCount)
		wildcardValues = adjustWildcardValuesCount(wildcardValues, currentWildcardCount)
	}

	// Validate against PatternDefine paramCount if it was sent
	mt.validateWildcardValuesCount(pattern, wildcardValues, currentWildcardCount, patternDefineParamCount)

	return wildcardValues
}

// adjustWildcardValuesCount adjusts the wildcard values slice to match expected count
func adjustWildcardValuesCount(wildcardValues []string, expectedCount int) []string {
	if len(wildcardValues) < expectedCount {
		// Pad with empty strings
		for len(wildcardValues) < expectedCount {
			wildcardValues = append(wildcardValues, "")
		}
	} else if len(wildcardValues) > expectedCount {
		// Truncate (shouldn't happen, but be safe)
		wildcardValues = wildcardValues[:expectedCount]
	}
	return wildcardValues
}

// validateWildcardValuesCount validates that wildcard values count matches PatternDefine paramCount
func (mt *MessageTranslator) validateWildcardValuesCount(pattern *clustering.Pattern, wildcardValues []string, currentWildcardCount int, patternDefineParamCount uint32) {
	if patternDefineParamCount > 0 {
		// If PatternDefine was sent in this cycle, validate against it
		if uint32(len(wildcardValues)) != patternDefineParamCount {
			log.Errorf("CRITICAL: StructuredLog count mismatch! pattern_id=%d StructuredLogCount=%d PatternDefineParamCount=%d (sent in this cycle) | This will cause intake error!",
				pattern.PatternID, len(wildcardValues), patternDefineParamCount)
		}
	} else {
		// PatternDefine was NOT sent in this cycle - validate against current pattern's wildcard count
		expectedParamCount := uint32(currentWildcardCount)
		if uint32(len(wildcardValues)) != expectedParamCount {
			log.Errorf("CRITICAL: StructuredLog count mismatch! pattern_id=%d StructuredLogCount=%d ExpectedParamCount=%d (no PatternDefine sent this cycle) | This will cause intake error!",
				pattern.PatternID, len(wildcardValues), expectedParamCount)
		}
	}
}

// sendRawLog creates and sends a raw log datum
func (mt *MessageTranslator) sendRawLog(outputChan chan *message.StatefulMessage, msg *message.Message, contentStr string, ts time.Time) {
	logDatum := buildRawLog(contentStr, ts)
	outputChan <- &message.StatefulMessage{
		Datum:    logDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// sendStructuredLog creates and sends a StructuredLog datum
func (mt *MessageTranslator) sendStructuredLog(outputChan chan *message.StatefulMessage, msg *message.Message, pattern *clustering.Pattern, wildcardValues []string, ts time.Time, patternDefineSent bool, patternDefineParamCount uint32) {
	logDatum := buildStructuredLog(pattern.PatternID, wildcardValues, ts)

	// Log StructuredLog details for debugging
	if logs, ok := logDatum.Data.(*statefulpb.Datum_Logs); ok {
		if sl := logs.Logs.GetStructured(); sl != nil {
			dynamicValuesStr := make([]string, len(sl.DynamicValues))
			for i, dv := range sl.DynamicValues {
				if sv := dv.GetStringValue(); sv != "" {
					dynamicValuesStr[i] = sv
				} else {
					dynamicValuesStr[i] = "<empty>"
				}
			}
			log.Infof("[MSG_TRANSLATOR] Sending StructuredLog: pattern_id=%d dynamicValuesCount=%d patternDefineSent=%v patternDefineParamCount=%d dynamicValues=%v",
				pattern.PatternID, len(sl.DynamicValues), patternDefineSent, patternDefineParamCount, dynamicValuesStr)

			// CRITICAL VALIDATION: Check if we're sending the right count
			currentPatternParamCount := uint32(pattern.GetWildcardCount())

			// Validate count matches what we're claiming to send
			if patternDefineSent {
				if uint32(len(sl.DynamicValues)) != patternDefineParamCount {
					log.Errorf("CRITICAL: StructuredLog mismatch with PatternDefine! pattern_id=%d dynamicValuesCount=%d patternDefineParamCount=%d | This WILL cause intake error!",
						pattern.PatternID, len(sl.DynamicValues), patternDefineParamCount)
				}
			} else {
				// No PatternDefine sent - intake will use previously defined pattern
				// Validate against current pattern state
				if uint32(len(sl.DynamicValues)) != currentPatternParamCount {
					log.Errorf("CRITICAL: StructuredLog mismatch with current pattern! pattern_id=%d dynamicValuesCount=%d currentPatternParamCount=%d (no PatternDefine sent) | This WILL cause intake error!",
						pattern.PatternID, len(sl.DynamicValues), currentPatternParamCount)
				}
			}

			// Log current pattern state for debugging
			if pattern.Template != nil {
				templateStr := pattern.GetPatternString()
				starCount := strings.Count(templateStr, "*")
				log.Infof("[MSG_TRANSLATOR] Current pattern state: pattern_id=%d templateStarCount=%d currentParamCount=%d template=%q",
					pattern.PatternID, starCount, currentPatternParamCount, templateStr)
			}
		}
	}

	outputChan <- &message.StatefulMessage{
		Datum:    logDatum,
		Metadata: &msg.MessageMetadata,
	}
}

// buildPatternDefine creates a PatternDefine Datum from a Pattern
func buildPatternDefine(pattern *clustering.Pattern) *statefulpb.Datum {
	// indice of wildcards in the pattern string
	charPositions := pattern.GetWildcardCharPositions()
	// is the indice that get converted to uint32
	posList := make([]uint32, len(charPositions))
	for i, pos := range charPositions {
		posList[i] = uint32(pos)
	}
	// count of wildcards in the pattern template
	paramCount := uint32(pattern.GetWildcardCount())
	// count of wildcards in the posList
	posListCount := uint32(len(posList))
	templateStr := pattern.GetPatternString()

	// Validate that the count of wildcards matches - they should always match
	// If they don't, it indicates pattern.Positions and Template.Tokens are out of sync
	if paramCount != posListCount {
		log.Errorf("CRITICAL: PatternDefine count mismatch! pattern_id=%d paramCount=%d (from pattern.Positions) posListCount=%d (from GetWildcardCharPositions) template=%q | This will cause intake error!",
			pattern.PatternID, paramCount, posListCount, templateStr)
		// Use posListCount as the authoritative source since it's what we're actually sending
		paramCount = posListCount
	}

	// Additional validation: count '*' in template string should match paramCount
	starCount := uint32(strings.Count(templateStr, "*"))
	if starCount != paramCount {
		log.Errorf("CRITICAL: PatternDefine template star count mismatch! pattern_id=%d template=%q starCount=%d paramCount=%d | This will cause intake error!",
			pattern.PatternID, templateStr, starCount, paramCount)
	}

	return &statefulpb.Datum{
		Data: &statefulpb.Datum_PatternDefine{
			PatternDefine: &statefulpb.PatternDefine{
				PatternId:  pattern.PatternID,
				Template:   pattern.GetPatternString(),
				ParamCount: paramCount,
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

// buildStructuredLog creates a Datum containing a StructuredLog
func buildStructuredLog(patternID uint64, wildcardValues []string, ts time.Time) *statefulpb.Datum {
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
			},
		},
	}
}

// buildRawLog creates a Datum containing a raw log (no pattern)
func buildRawLog(content string, ts time.Time) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: uint64(ts.UnixNano() / nanoToMillis),
				Content: &statefulpb.Log_Raw{
					Raw: content,
				},
			},
		},
	}
}

// toValidUtf8 ensures all characters are UTF-8
func toValidUtf8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}

	var str strings.Builder
	str.Grow(len(data))

	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		// in case of invalid utf-8, DecodeRune returns (utf8.RuneError, 1)
		// and since RuneError is the same as unicode.ReplacementChar
		// no need to handle the error explicitly
		str.WriteRune(r)
		data = data[size:]
	}
	return str.String()
}
