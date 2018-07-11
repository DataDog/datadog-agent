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

// Reader reads data from a connection
type Reader struct {
	source               *config.LogSource
	conn                 net.Conn
	outputChan           chan message.Message
	keepAlive            bool
	handleUngracefulStop func(*Reader)
	decoder              *decoder.Decoder
	stop                 chan struct{}
	done                 chan struct{}
}

// NewReader returns a new Reader
func NewReader(source *config.LogSource, conn net.Conn, outputChan chan message.Message, keepAlive bool, handleUngracefulStop func(*Reader)) *Reader {
	return &Reader{
		source:               source,
		conn:                 conn,
		outputChan:           outputChan,
		keepAlive:            keepAlive,
		handleUngracefulStop: handleUngracefulStop,
		decoder:              decoder.InitializeDecoder(source),
		stop:                 make(chan struct{}, 1),
		done:                 make(chan struct{}, 1),
	}
}

// Start prepares the reader to read and decode data from the connection
func (r *Reader) Start() {
	go r.forwardMessages()
	r.decoder.Start()
	go r.readForever()
}

// Stop stops the reader and waits for the decoder to be flushed
func (r *Reader) Stop() {
	r.stop <- struct{}{}
	r.conn.Close()
	<-r.done
}

// forwardMessages forwards messages to output channel
func (r *Reader) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		r.done <- struct{}{}
	}()
	for output := range r.decoder.OutputChan {
		origin := message.NewOrigin(r.source)
		r.outputChan <- message.New(output.Content, origin, "")
	}
}

// readForever reads the data from conn until timeout or an error occurt
func (r *Reader) readForever() {
	defer func() {
		r.conn.Close()
		r.decoder.Stop()
	}()
	for {
		select {
		case <-r.stop:
			// stop reading data from the connection
			return
		default:
			if !r.keepAlive {
				r.conn.SetReadDeadline(time.Now().Add(defaultTimeout))
			}
			inBuf := make([]byte, 4096)
			n, err := r.conn.Read(inBuf)
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
				r.source.Status.Error(err)
				r.handleUngracefulStop(r)
				return
			}
			r.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}
	}
}
