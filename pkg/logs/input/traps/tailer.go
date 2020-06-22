// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tailer consumes and processes a stream of trap packets, and sends them to a stream of log messages.
type Tailer struct {
	source     *config.LogSource
	inputChan  traps.OutputChannel
	outputChan chan *message.Message
	done       chan interface{}
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, inputChan traps.OutputChannel, outputChan chan *message.Message) *Tailer {
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

// Stop waits for the input buffer to be flushed.
func (t *Tailer) Stop() {
	<-t.done
}

func (t *Tailer) run() {
	defer func() {
		t.done <- true
	}()

	for packet := range t.inputChan {
		content, err := FormatPacketJSON(packet)
		if err != nil {
			log.Errorf("failed to format packet: %s", err)
			continue
		}
		origin := message.NewOrigin(t.source)
		origin.SetTags(FormatPacketTags(packet))
		t.outputChan <- message.NewMessage(content, origin, message.StatusInfo)
	}
}
