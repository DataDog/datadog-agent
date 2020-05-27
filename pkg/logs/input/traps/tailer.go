// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
)

// Tailer consumes and processes a stream of trap packets, and sends them to a stream of log messages.
type Tailer struct {
	source     *config.LogSource
	inputChan  traps.OutputChannel
	outputChan chan *message.Message
	done       chan bool
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, inputChan traps.OutputChannel, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		source:     source,
		inputChan:  inputChan,
		outputChan: outputChan,
		done:       make(chan bool, 1),
	}
}

// Start starts the tailer.
func (t *Tailer) Start() {
	go t.run()
}

// Stop waits for the input buffer to be flushed.
func (t *Tailer) Stop() {
	<-t.done
}

func (t *Tailer) run() {
	defer func() {
		t.done <- true
	}()

	origin := message.NewOrigin(t.source)
	status := message.StatusInfo // TODO

	for packet := range t.inputChan {
		content := encodePacket(packet)
		t.outputChan <- message.NewMessage(content, origin, status)
	}
}

// encodePacket converts an SNMP trap packet to a log message content.
func encodePacket(p *traps.SnmpPacket) []byte {
	content := make([]byte, 0)
	// TODO fill content with packet data.
	content = append(content, []byte("Hello, traps!")...)
	return content
}
