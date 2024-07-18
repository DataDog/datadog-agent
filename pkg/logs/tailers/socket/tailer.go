// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package socket

import (
	"fmt"
	"io"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Tailer reads data from a net.Conn.  It uses a `read` callback to be generic
// over types of connections.
type Tailer struct {
	source      *sources.LogSource
	Conn        net.Conn
	tagProvider tag.Provider
	outputChan  chan *message.Message
	read        func(*Tailer) ([]byte, error)
	decoder     *decoder.Decoder
	stop        chan struct{}
	done        chan struct{}
}

// NewTailer returns a new Tailer
func NewTailer(source *sources.LogSource, conn net.Conn, outputChan chan *message.Message, read func(*Tailer) ([]byte, error)) *Tailer {
	tagProvider := tag.NewLocalProvider([]string{})
	return &Tailer{
		source:      source,
		Conn:        conn,
		tagProvider: tagProvider,
		outputChan:  outputChan,
		read:        read,
		// tailer info is currently unused for this tailer type.
		decoder: decoder.InitializeDecoder(sources.NewReplaceableSource(source), noop.New(), status.NewInfoRegistry()),
		stop:    make(chan struct{}, 1),
		done:    make(chan struct{}, 1),
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
	t.Conn.Close()
	<-t.done
}

// forwardMessages forwards messages to output channel
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		t.done <- struct{}{}
	}()
	for output := range t.decoder.OutputChan {
		if len(output.GetContent()) > 0 {
			origin := message.NewOrigin(t.source)
			source_host_tag := fmt.Sprintf("source_host:%d", t.source.Config.Port)
			origin.SetTags(append(t.source.Config.Tags, source_host_tag))
			t.outputChan <- message.NewMessage(output.GetContent(), origin, output.Status, output.IngestionTimestamp)
		}
	}
}

// readForever reads the data from conn.
func (t *Tailer) readForever() {
	defer func() {
		t.Conn.Close()
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
			t.source.RecordBytes(int64(len(data)))
			t.decoder.InputChan <- decoder.NewInput(data)
		}
	}
}
