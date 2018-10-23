// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"io"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// Tailer reads data from a connection
type Tailer struct {
	source     *config.LogSource
	conn       net.Conn
	outputChan chan *message.Message
	read       func(*Tailer) ([]byte, error)
	decoder    *decoder.Decoder
	stop       chan struct{}
	done       chan struct{}
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, conn net.Conn, outputChan chan *message.Message, read func(*Tailer) ([]byte, error)) *Tailer {
	return &Tailer{
		source:     source,
		conn:       conn,
		outputChan: outputChan,
		read:       read,
		decoder:    decoder.InitializeDecoder(source, parser.NoopParser),
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Start prepares the tailer to read and decode data from the connection
func (t *Tailer) Start() {
	go t.forwardMessages()
	t.decoder.Start()
	go t.readForever()
}

// Stop stops the tailer and waits for the decoder to be flushed
func (t *Tailer) Stop() {
	t.stop <- struct{}{}
	t.conn.Close()
	<-t.done
}

// forwardMessages forwards messages to output channel
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		t.done <- struct{}{}
	}()
	for output := range t.decoder.OutputChan {
		metrics.LogsCollected.Add(1)
		output.Origin = message.NewOrigin(t.source)
		output.SetStatus(message.StatusInfo)
		t.outputChan <- output
	}
}

// readForever reads the data from conn.
func (t *Tailer) readForever() {
	defer func() {
		t.conn.Close()
		t.decoder.Stop()
	}()
	for {
		select {
		case <-t.stop:
			// stop reading data from the connection
			return
		default:
			data, err := t.read(t)
			if err != nil && err == io.EOF {
				// connection has been closed client-side, stop from reading new data
				return
			}
			if err != nil {
				// an error occurred, stop from reading new data
				log.Warnf("Couldn't read message from connection: %v", err)
				return
			}
			t.decoder.InputChan <- decoder.NewInput(data)
		}
	}
}
