// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"fmt"
	"io"
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tailer reads syslog messages from a TCP connection, parses them, and
// forwards structured messages to the pipeline.
//
// This follows the journald tailer pattern: the tailer owns both framing
// (via Reader) and parsing (via buildStructuredMessage), producing
// StateStructured messages directly and pushing them to the decoder's InputChan.
type Tailer struct {
	decoder     decoder.Decoder
	source      *sources.LogSource
	outputChan  chan *message.Message
	conn        net.Conn
	reader      *Reader
	stop        chan struct{}
	done        chan struct{}
	tagProvider tag.Provider
}

// NewTailer returns a new syslog Tailer for the given TCP connection.
func NewTailer(source *sources.LogSource, outputChan chan *message.Message, conn net.Conn) *Tailer {
	return &Tailer{
		decoder:     decoder.NewNoopDecoder(),
		source:      source,
		outputChan:  outputChan,
		conn:        conn,
		reader:      NewReader(conn),
		stop:        make(chan struct{}, 1),
		done:        make(chan struct{}, 1),
		tagProvider: tag.NewLocalProvider(source.Config.Tags),
	}
}

// Start begins tailing the TCP connection.
func (t *Tailer) Start() {
	t.source.Status.Success()
	log.Infof("Start tailing syslog connection from %s", t.conn.RemoteAddr())

	go t.forwardMessages()
	t.decoder.Start()
	go t.tail()
}

// Stop stops the tailer and waits for it to finish.
func (t *Tailer) Stop() {
	log.Infof("Stop tailing syslog connection from %s", t.conn.RemoteAddr())
	t.stop <- struct{}{}
	t.conn.Close()
	<-t.done
}

// Identifier returns a unique identifier for this tailer.
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("syslog:%s", t.conn.RemoteAddr())
}

// forwardMessages reads decoded messages from the decoder output and
// forwards them to the pipeline output channel.
func (t *Tailer) forwardMessages() {
	defer func() {
		close(t.done)
	}()

	for decodedMessage := range t.decoder.OutputChan() {
		if len(decodedMessage.GetContent()) > 0 {
			t.outputChan <- decodedMessage
		}
	}
}

// tail reads syslog frames from the connection, parses them into
// structured messages, and sends them to the decoder's input channel.
func (t *Tailer) tail() {
	defer func() {
		t.decoder.Stop()
	}()

	for {
		select {
		case <-t.stop:
			return
		default:
			frame, err := t.reader.ReadFrame()
			if err != nil {
				if err != io.EOF {
					log.Warnf("Error reading syslog frame from %s: %v", t.conn.RemoteAddr(), err)
				}
				return
			}

			origin := t.getOrigin()
			msg, err := buildStructuredMessage(frame, origin)
			if err != nil {
				log.Debugf("Error parsing syslog message from %s: %v", t.conn.RemoteAddr(), err)
				// buildStructuredMessage returns a partial message on error;
				// continue processing it.
			}

			select {
			case <-t.stop:
				return
			case t.decoder.InputChan() <- msg:
			}
		}
	}
}

// getOrigin returns a new message origin for this tailer.
func (t *Tailer) getOrigin() *message.Origin {
	origin := message.NewOrigin(t.source)
	origin.Identifier = t.Identifier()
	origin.SetTags(t.tagProvider.GetTags())
	return origin
}
