// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tailer consumes and processes a stream of trap packets, and sends them to a stream of log messages.
type Tailer struct {
	source     *config.LogSource
	formatter  traps.Formatter
	inputChan  traps.PacketsChannel
	outputChan chan *message.Message
	done       chan interface{}
}

// NewTailer returns a new Tailer
func NewTailer(oidResolver traps.OIDResolver, source *config.LogSource, inputChan traps.PacketsChannel, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		source:     source,
		inputChan:  inputChan,
		outputChan: outputChan,
		formatter:  traps.NewJSONFormatter(oidResolver),
		done:       make(chan interface{}, 1),
	}
}

// Start starts the tailer.
func (t *Tailer) Start() {
	go t.run()
}

// WaitFlush waits for all items in the input channel to be processed.
func (t *Tailer) WaitFlush() {
	<-t.done
}

func (t *Tailer) run() {
	defer func() {
		t.done <- true
	}()

	// Loop terminates when the channel is closed.
	for packet := range t.inputChan {
		data, err := t.formatter.FormatPacket(packet)
		if err != nil {
			log.Errorf("failed to format packet: %s", err)
			continue
		}
		t.source.BytesRead.Add(int64(len(data)))
		origin := message.NewOrigin(t.source)
		origin.SetTags(t.formatter.GetTags(packet))
		t.outputChan <- message.NewMessage(data, origin, message.StatusInfo, time.Now().UnixNano())
	}
}
