// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Tailer consumes and processes a channel of strings, and sends them to a stream of log messages.
type Tailer struct {
	source     *config.LogSource
	inputChan  chan *config.ChannelMessage
	outputChan chan *message.Message
	done       chan interface{}
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, inputChan chan *config.ChannelMessage, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		source:     source,
		inputChan:  inputChan,
		outputChan: outputChan,
		done:       make(chan interface{}, 1),
	}
}

// Start starts the tailer.
func (t *Tailer) Start() {
	go t.run()
}

// WaitFlush waits for all items in the input channel to be processed.
func (t *Tailer) WaitFlush() {
	close(t.inputChan)
	<-t.done
}

func (t *Tailer) run() {
	defer func() {
		t.done <- true
	}()

	// Loop terminates when the channel is closed.
	for logline := range t.inputChan {
		origin := message.NewOrigin(t.source)
		tags := origin.Tags()

		if logline.Lambda != nil && logline.Lambda.FunctionName != "" {
			origin.SetService(logline.Lambda.FunctionName)
		} else {
			origin.SetService("agent")
		}

		if len(t.source.Config.Tags) > 0 {
			tags = append(tags, t.source.Config.Tags...)
		}
		origin.SetTags(tags)
		if logline.Lambda != nil {
			t.outputChan <- message.NewMessageFromLambda(logline.Content, origin, message.StatusInfo, logline.Timestamp, logline.Lambda.ARN, logline.Lambda.RequestID, time.Now().UnixNano())
		} else {
			t.outputChan <- message.NewMessage(logline.Content, origin, message.StatusInfo, time.Now().UnixNano())
		}
	}
}
