package epforwarder

import (
	"fmt"
	"sync"

	"github.com/DataDog/agent-payload/v5/contimage"
	"github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var globalReceiver *BufferedMessageReceiver

// MessageReceiver interface to handle messages for diagnostics
type MessageReceiver interface {
	HandleMessage(message.Message, string, string)
}

type messagePair struct {
	msg       *message.Message
	eventType string
}

// BufferedMessageReceiver handles in coming log messages and makes them available for diagnostics
type BufferedMessageReceiver struct {
	inputChan chan messagePair
	enabled   bool
	m         sync.RWMutex
}

// Filters for processing log messages
type Filters struct {
	Type string `json:"type"`
}

// NewBufferedMessageReceiver creates a new MessageReceiver
func NewBufferedMessageReceiver() *BufferedMessageReceiver {
	if globalReceiver == nil {
		globalReceiver = &BufferedMessageReceiver{
			inputChan: make(chan messagePair, config.ChanSize),
		}
	}

	return globalReceiver
}

// Start opens new input channel
func (b *BufferedMessageReceiver) Start() {
	b.inputChan = make(chan messagePair, config.ChanSize)
}

// Stop closes the input channel
func (b *BufferedMessageReceiver) Stop() {
	close(b.inputChan)
}

// Clear empties buffered messages
func (b *BufferedMessageReceiver) clear() {
	l := len(b.inputChan)
	for i := 0; i < l; i++ {
		<-b.inputChan
	}
}

// SetEnabled start collecting log messages for diagnostics. Returns true if state was successfully changed
func (b *BufferedMessageReceiver) SetEnabled(e bool) bool {
	b.m.Lock()
	defer b.m.Unlock()

	if b.enabled == e {
		return false
	}

	b.enabled = e
	if !e {
		b.clear()
	}
	return true
}

// IsEnabled returns the enabled state of the message receiver
func (b *BufferedMessageReceiver) IsEnabled() bool {
	b.m.RLock()
	defer b.m.RUnlock()
	return b.enabled
}

// HandleMessage buffers a message for diagnostic processing
func (b *BufferedMessageReceiver) HandleMessage(m message.Message, eventType string, contentType string) {
	if !b.IsEnabled() {
		return
	}

	// For now, only support the protobuf events for no reason other than I am lazy.
	// TODO support json events as well
	if contentType != http.ProtobufContentType {
		return
	}
	b.inputChan <- messagePair{&m, eventType}
}

// Filter writes the buffered events from the input channel formatted as a string to the output channel
func (b *BufferedMessageReceiver) Filter(filters *Filters, done <-chan struct{}) <-chan string {
	out := make(chan string, config.ChanSize)
	go func() {
		defer close(out)
		for {
			select {
			case msgPair := <-b.inputChan:
				if shouldHandleMessage(msgPair.eventType, filters) {
					out <- formatMessage(msgPair.msg, msgPair.eventType)
				}
			case <-done:
				return
			}
		}
	}()
	return out
}

func shouldHandleMessage(eventType string, filters *Filters) bool {
	if filters == nil {
		return true
	}

	shouldHandle := true

	if filters.Type != "" {
		shouldHandle = shouldHandle && eventType == filters.Type
	}

	return shouldHandle
}

func formatMessage(m *message.Message, eventType string) string {
	// TODO Need to do this a better way, but I just want something working first
	output := fmt.Sprintf("type: %v | ", eventType)

	switch eventType {
	case EventTypeContainerLifecycle:
		var msg contlcycle.EventsPayload
		proto.Unmarshal(m.Content, &msg)
		output += msg.String()
	case EventTypeContainerImages:
		var msg contimage.ContainerImagePayload
		proto.Unmarshal(m.Content, &msg)
		output += msg.String()
	case EventTypeContainerSBOM:
		var msg sbom.SBOMPayload
		proto.Unmarshal(m.Content, &msg)
		output += msg.String()
	default:
		output += "UNKNOWN"
	}
	output += "\n"
	return output
}

func GetGlobalReceiver() *BufferedMessageReceiver {
	return globalReceiver
}
