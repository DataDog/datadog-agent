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
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

const nanoToMillis = 1000000

// StartMessageTranslator starts a goroutine that translates message.Message to message.StatefulMessage
// It handles pattern extraction by sending PatternDefine for new patterns, or PatternDelete+PatternDefine for updates,
// then sends the StructuredLog or raw log.
func StartMessageTranslator(inputChan chan *message.Message, outputChan chan *message.StatefulMessage) {
	go func() {
		defer close(outputChan)

		for msg := range inputChan {
			// Get timestamp - prefer message timestamp if available
			ts := time.Now().UTC()
			if !msg.ServerlessExtra.Timestamp.IsZero() {
				ts = msg.ServerlessExtra.Timestamp
			}

			// If pattern template needs to be sent, emit template definition message(s) first
			if msg.PatternTemplateState != clustering.TemplateNotNeeded {
				switch msg.PatternTemplateState {
				case clustering.TemplateIsNew:
					// New template - send PatternDefine
					patternDatum := buildPatternDefine(msg.Pattern)
					outputChan <- &message.StatefulMessage{
						Datum:    patternDatum,
						Metadata: &msg.MessageMetadata,
					}

				case clustering.TemplateChanged:
					// Updated template - send PatternDelete + PatternDefine to replace it
					// This ensures intake has the updated template definition
					deleteDatum := buildPatternDelete(msg.Pattern.PatternID)
					outputChan <- &message.StatefulMessage{
						Datum:    deleteDatum,
						Metadata: &msg.MessageMetadata,
					}

					defineDatum := buildPatternDefine(msg.Pattern)
					outputChan <- &message.StatefulMessage{
						Datum:    defineDatum,
						Metadata: &msg.MessageMetadata,
					}
				}
			}

			// Create the Log message - either structured (with pattern)
			logDatum := buildStructuredLog(msg.Pattern.PatternID, msg.WildcardValues, ts)

			// Create StatefulMessage with the log Datum
			statefulMsg := &message.StatefulMessage{
				Datum:    logDatum,
				Metadata: &msg.MessageMetadata,
			}

			outputChan <- statefulMsg
		}
	}()
}

// buildPatternDefine creates a PatternDefine Datum from a Pattern
func buildPatternDefine(pattern *clustering.Pattern) *statefulpb.Datum {
	// Get character positions where wildcards appear in the template string
	// This allows the backend to know where to insert dynamic values
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
				ParamCount: uint32(len(pattern.Positions)),
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
