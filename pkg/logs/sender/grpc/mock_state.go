// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/agent-payload/v5/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const nanoToMillis = 1000000

// MessageTranslator handles translation of message.Message to message.StatefulMessage
// It manages pattern extraction, clustering, and stateful message creation
type MessageTranslator struct {
	pipelineName string
}

// NewMessageTranslator creates a new MessageTranslator instance
// If clusterManager is nil, a new one will be created
func NewMessageTranslator(pipelineName string) *MessageTranslator {
	return &MessageTranslator{
		pipelineName: pipelineName,
	}
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
	// TODO: in the future this is where pattern m

	ts := getMessageTimestamp(msg)

	// Get message content
	content := msg.GetContent()
	if len(content) == 0 {
		return
	}

	mt.sendRawLog(outputChan, msg, string(content), ts, mt.buildTagSet(msg))
}

// buildTagSet constructs the complete tag list for a message and encodes it as a TagSet.
// This includes log-level fields (hostname, service, ddsource, status) as tags,
// plus all other tags from the message metadata (container tags, source config tags, processing tags).
// All tags are joined as a single string, encoded as a single dictionary entry in the TagSet
func (mt *MessageTranslator) buildTagSet(msg *message.Message) *statefulpb.TagSet {
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
		return nil
	}

	tagSet := &statefulpb.TagSet{
		Tagset: &statefulpb.DynamicValue{
			Value: &statefulpb.DynamicValue_StringValue{
				StringValue: allTagsString,
			},
		},
	}

	return tagSet
}

// getMessageTimestamp returns the timestamp for the message, preferring ServerlessExtra.Timestamp
func getMessageTimestamp(msg *message.Message) time.Time {
	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}
	return ts
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
