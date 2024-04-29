// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package message

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Payload represents an encoded collection of messages ready to be sent to the intake
type Payload struct {
	// The slice of sources messages encoded in the payload
	Messages []*Message
	// The encoded bytes to be sent to the intake (sometimes compressed)
	Encoded []byte
	// The content encoding. A header for HTTP, empty for TCP
	Encoding string
	// The size of the unencoded payload
	UnencodedSize int
}

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	MessageContent
	Hostname           string
	Origin             *Origin
	Status             string
	IngestionTimestamp int64
	RawDataLen         int
	// Tags added on processing
	ProcessingTags []string
	// Extra information from the parsers
	ParsingExtra
	// Extra information for Serverless Logs messages
	ServerlessExtra
}

// MessageContent contains the message and possibly the tailer internal representation
// of every message.
//
// To use the MessageContent struct, use `GetContent() []byte` or SetContent([]byte)`
// makes sure of doing the right thing depending on the MessageContent state.
//
// MessageContent different states:
//
//	+-------------------+
//	| StateUnstructured | ------
//	+-------------------+      |
//	                           |
//	                           v
//	                      ( Processor )    +---------------+    ( Processor )    +--------------+
//	                      (  Renders  ) -> | StateRendered | -> (  Encodes  ) -> | StateEncoded |
//	                           ^           +---------------+                     +--------------+
//	                           |                 |
//	+-------------------+      |                 v
//	|  StateStructured  | ------          (   Diagnostic   )
//	+-------------------+                 (Message Receiver)
//
// In `StateUnstructured`, the content in `Content` is the raw log collected by the tailer.
// In `StateStructured`, `Content` is empty and the log information are in `StructuredContent`.
// In `StateRendered`, `Content` contains rendered data (from raw/structured logs to something
// ready to be encoded), the rest should not be used.
// In `StateEncoded`, `Content` contains the encoded data, the rest should not be used.
//
// Note that there is no state distinction between parsed and unparsed content as none was needed
// for the current implementation, but it is a potential future change with a `StateParsed` state.
type MessageContent struct { //nolint:revive
	// unstructured content
	content []byte
	// structured content
	structuredContent StructuredContent
	State             MessageContentState
}

// MessageContentState is used to represent the MessageContent state.
type MessageContentState uint32 // nolint:revive

const (
	// StateUnstructured for unstructured content (e.g. file tailing)
	StateUnstructured MessageContentState = iota
	// StateStructured for structured content (e.g. journald tailing, windowsevent tailing)
	StateStructured
	// StateRendered means that the MessageContent contains rendered (i.e. structured content has been rendered)
	StateRendered
	// StateEncoded means the MessageContent passed through the encoder (e.g. json encoder, proto encoder, ...)
	StateEncoded
)

// GetContent returns the bytes array containing only the message content
// E.g. from a structured log:
//
//	Sep 12 14:38:14 user my-app[1316]: time="2023-09-12T14:38:14Z" level=info msg="Starting the main execution"
//
// It would only return the `[]byte` containing "Starting the main execution"
// While for unstructured log and for source configured with ProcessRawMessage=true,
// the whole `[]byte` content is returned.
// See `MessageContent` comment for more information as this method could also
// return the message content in different state (rendered, encoded).
func (m *MessageContent) GetContent() []byte {
	switch m.State {
	// for raw, rendered or encoded message, the data has
	// been written into m.Content
	case StateUnstructured, StateRendered, StateEncoded:
		return m.content
	// when using GetContent() on a structured log, we want
	// to only return the part containing the content (e.g. for message
	// processing or for scrubbing)
	case StateStructured:
		return m.structuredContent.GetContent()
	default:
		log.Error("Unknown state for message on call to SetContent:", m.State)
		return m.content
	}
}

// SetContent stores the given content as the content message.
// SetContent uses the current message state to know where
// to store the content.
func (m *MessageContent) SetContent(content []byte) {
	switch m.State {
	case StateStructured:
		m.structuredContent.SetContent(content)
	case StateUnstructured, StateRendered, StateEncoded:
		m.content = content
	default:
		log.Error("Unknown state for message on call to SetContent:", m.State)
		m.content = content
	}
}

// SetRendered sets the content for the MessageContent and sets MessageContent state to rendered.
func (m *MessageContent) SetRendered(content []byte) {
	m.content = content
	m.State = StateRendered
}

// SetEncoded sets the content for the MessageContent and sets MessageContent state to encoded.
func (m *MessageContent) SetEncoded(content []byte) {
	m.content = content
	m.State = StateEncoded
}

// ParsingExtra ships extra information parsers want to make available
// to the rest of the pipeline.
// E.g. Timestamp is used by the docker parsers to transmit a tailing offset.
type ParsingExtra struct {
	// Used by docker parsers to transmit an offset.
	Timestamp string
	IsPartial bool
}

// ServerlessExtra ships extra information from logs processing in serverless envs.
type ServerlessExtra struct {
	// Optional. Must be UTC. If not provided, time.Now().UTC() will be used
	// Used in the Serverless Agent
	Timestamp time.Time
	// Optional.
	// Used in the Serverless Agent
	Lambda *Lambda
}

// Lambda is a struct storing information about the Lambda function and function execution.
type Lambda struct {
	ARN       string
	RequestID string
}

// NewMessageWithSource constructs an unstructured message
// with content, status and a log source.
func NewMessageWithSource(content []byte, status string, source *sources.LogSource, ingestionTimestamp int64) *Message {
	return NewMessage(content, NewOrigin(source), status, ingestionTimestamp)
}

// NewMessage constructs an unstructured message with content,
// status, origin and the ingestion timestamp.
func NewMessage(content []byte, origin *Origin, status string, ingestionTimestamp int64) *Message {
	return &Message{
		MessageContent: MessageContent{
			content: content,
			State:   StateUnstructured,
		},
		Origin:             origin,
		Status:             status,
		IngestionTimestamp: ingestionTimestamp,
	}
}

// NewStructuredMessage creates a new message that had some structure the moment
// it has been captured through a tailer.
// e.g. a journald message which is a JSON object containing extra information, including
// the actual message of the entry. We need these objects to be able to apply
// processing on the message entry only, while we still have to send all
// the information to the intake.
func NewStructuredMessage(content StructuredContent, origin *Origin, status string, ingestionTimestamp int64) *Message {
	return &Message{
		MessageContent: MessageContent{
			structuredContent: content,
			State:             StateStructured,
		},
		Origin:             origin,
		Status:             status,
		IngestionTimestamp: ingestionTimestamp,
	}
}

// Render renders the message.
// The only state in which this call is changing the content for a StateStructured message.
func (m *Message) Render() ([]byte, error) {
	switch m.State {
	case StateUnstructured:
		return m.content, nil
	case StateStructured:
		data, err := m.MessageContent.structuredContent.Render()
		if err != nil {
			return nil, err
		}
		return data, nil
	case StateRendered:
		return m.content, nil
	case StateEncoded:
		return m.content, fmt.Errorf("render call on an encoded message")
	default:
		return m.content, fmt.Errorf("unknown message state for rendering")
	}
}

// StructuredContent stores enough information from a tailer to manipulate a
// structured log message (from journald or windowsevents) and to render it to
// be encoded later on in the pipeline.
type StructuredContent interface {
	Render() ([]byte, error)
	GetContent() []byte
	SetContent([]byte)
}

// BasicStructuredContent is used by tailers creating structured logs
// but with basic needs for transport.
// The message from the log is stored in the "message" key.
type BasicStructuredContent struct {
	Data map[string]interface{}
}

// Render renders in json the underlying data, it is then ready to be
// encoded and sent to the intake. See the `MessageContent` comment.
func (m *BasicStructuredContent) Render() ([]byte, error) {
	return json.Marshal(m.Data)
}

// GetContent returns the message part of the structured log,
// in the "message" key of the underlying map.
func (m *BasicStructuredContent) GetContent() []byte {
	if value, exists := m.Data["message"]; exists {
		return []byte(value.(string))
	}
	log.Error("BasicStructuredContent not containing any message")
	return []byte{}
}

// SetContent stores the message part of the structured log,
// in the "message" key of the underlying map.
func (m *BasicStructuredContent) SetContent(content []byte) {
	// we want to store it typed as a string for the json
	// marshaling to properly marshal it as a string.
	m.Data["message"] = string(content)
}

// NewMessageFromLambda construts a message with content, status, origin and with
// the given timestamp and Lambda metadata.
func NewMessageFromLambda(content []byte, origin *Origin, status string, utcTime time.Time, ARN, reqID string, ingestionTimestamp int64) *Message {
	return &Message{
		MessageContent: MessageContent{
			content: content,
			State:   StateUnstructured,
		},
		Origin:             origin,
		Status:             status,
		IngestionTimestamp: ingestionTimestamp,
		ServerlessExtra: ServerlessExtra{
			Timestamp: utcTime,
			Lambda: &Lambda{
				ARN:       ARN,
				RequestID: reqID,
			},
		},
	}
}

// GetStatus gets the status of the message.
// if status is not set, StatusInfo will be returned.
func (m *Message) GetStatus() string {
	if m.Status == "" {
		m.Status = StatusInfo
	}
	return m.Status
}

// GetLatency returns the latency delta from ingestion time until now
func (m *Message) GetLatency() int64 {
	return time.Now().UnixNano() - m.IngestionTimestamp
}

// Message returns all tags that this message is attached with.
func (m *Message) Tags() []string {
	return m.Origin.Tags(m.ProcessingTags)
}

// Message returns all tags that this message is attached with, as a string.
func (m *Message) TagsToString() string {
	return m.Origin.TagsToString(m.ProcessingTags)
}
