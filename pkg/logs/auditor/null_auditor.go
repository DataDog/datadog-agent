package auditor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// NullAuditor is an auditor not doing anything with the message it received.
// It has been introduced for the Serverless Agent which doesn't need
// to maintain a registry.
type NullAuditor struct {
	channel     chan *message.Message
	stopChannel chan struct{}
}

// NewNullAuditor returns an instanciated NullAuditor. `Start()` is the next method
// that should be used on this NullAuditor.
func NewNullAuditor() *NullAuditor {
	return &NullAuditor{
		channel:     make(chan *message.Message),
		stopChannel: make(chan struct{}),
	}
}

// GetOffset returns an empty string.
func (a *NullAuditor) GetOffset(identifier string) string { return "" }

// GetTailingMode returns an empty string.
func (a *NullAuditor) GetTailingMode(identifier string) string { return "" }

// Start starts the NullAuditor main loop.
func (a *NullAuditor) Start() {
	go a.run()
}

// Stop stops the NullAuditor main loop.
func (a *NullAuditor) Stop() {
	a.stopChannel <- struct{}{}
}

// Channel returns the channel on which should be sent the messages.
func (a *NullAuditor) Channel() chan *message.Message {
	return a.channel
}

func (a *NullAuditor) run() {
	for {
		select {
		case <-a.channel:
			// draining the channel, we're not doing anything with the message
		case <-a.stopChannel:
			// TODO(remy): close the message channel
			return
		}
	}
}
