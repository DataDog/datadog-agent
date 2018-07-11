// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"io"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// defaultTimeout represents the time after which a connection is closed when no data is read
const defaultTimeout = 60000 * time.Millisecond

// Tailer reads data from a connection
type Tailer struct {
	source           *config.LogSource
	conn             net.Conn
	outputChan       chan message.Message
	keepAlive        bool
	recoverFromError func(*Tailer)
	decoder          *decoder.Decoder
	stop             chan struct{}
	done             chan struct{}
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, conn net.Conn, outputChan chan message.Message, keepAlive bool, recoverFromError func(*Tailer)) *Tailer {
	return &Tailer{
		source:           source,
		conn:             conn,
		outputChan:       outputChan,
		keepAlive:        keepAlive,
		recoverFromError: recoverFromError,
		decoder:          decoder.InitializeDecoder(source),
		stop:             make(chan struct{}, 1),
		done:             make(chan struct{}, 1),
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
		origin := message.NewOrigin(t.source)
		t.outputChan <- message.New(output.Content, origin, "")
	}
}

// readForever reads the data from conn until timeout or an error occurt
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
			if !t.keepAlive {
				t.conn.SetReadDeadline(time.Now().Add(defaultTimeout))
			}
			inBuf := make([]byte, 4096)
			n, err := t.conn.Read(inBuf)
			if err == io.EOF {
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// timeout expired, stop from reading new data
				return
			}
			if err != nil {
				// an error occurred, stop from reading new data
				log.Warnf("Couldn't read message from connection: %v", err)
				t.recoverFromError(t)
				return
			}
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}
	}
}
