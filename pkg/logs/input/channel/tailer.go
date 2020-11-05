// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package channel

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
)

// Tailer consumes and processes a channel of strings, and sends them to a stream of log messages.
type Tailer struct {
	source     *config.LogSource
	inputChan  chan aws.LogMessage
	outputChan chan *message.Message
	done       chan interface{}
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, inputChan chan aws.LogMessage, outputChan chan *message.Message) *Tailer {
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
		tags = append(tags, "source:agent") // FIXME(remy): to remove
		if len(t.source.Config.Tags) > 0 {
			tags = append(tags, t.source.Config.Tags...)
		}
		origin.SetTags(tags)
		t.outputChan <- message.NewMessageWithTime([]byte(logline.StringRecord), origin, message.StatusInfo, logline.Time)
	}
}
