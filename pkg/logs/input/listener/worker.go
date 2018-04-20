// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"io"
	"net"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// defaultTimeout represents the time after which a connection is closed when no data is read
const defaultTimeout = 60000 * time.Millisecond

// Worker reads data from a connection
type Worker struct {
	source     *config.LogSource
	conn       net.Conn
	outputChan chan message.Message
	decoder    *decoder.Decoder
	shouldStop bool
	stop       chan struct{}
	done       chan struct{}
}

// NewWorker returns a new Worker
func NewWorker(source *config.LogSource, conn net.Conn, outputChan chan message.Message) *Worker {
	return &Worker{
		source:     source,
		conn:       conn,
		outputChan: outputChan,
		decoder:    decoder.InitializeDecoder(source),
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Start prepares the worker to read and decode data from the connection
func (w *Worker) Start() {
	go w.forwardMessages()
	w.decoder.Start()
	go w.readForever()
}

// Stop stops the worker and wait the decoder to be flushed
func (w *Worker) Stop() {
	w.stop <- struct{}{}
	w.conn.Close()
	<-w.done
}

// forwardMessages forwards messages to output channel
func (w *Worker) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		w.shouldStop = true
		w.done <- struct{}{}
	}()
	for output := range w.decoder.OutputChan {
		origin := message.NewOrigin(w.source)
		w.outputChan <- message.New(output.Content, origin, nil)
	}
}

// readForever reads the data from conn until timeout or an error occurs
func (w *Worker) readForever() {
	defer func() {
		w.conn.Close()
		w.decoder.Stop()
	}()
	for {
		select {
		case <-w.stop:
			// stop reading data from the connection
			return
		default:
			if !w.mustKeepConnAlive() {
				w.conn.SetReadDeadline(time.Now().Add(defaultTimeout))
			}
			inBuf := make([]byte, 4096)
			n, err := w.conn.Read(inBuf)
			if err == io.EOF {
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// timeout expired, stop from reading new data
				return
			}
			if err != nil {
				// an error occurred, stop from reading new data
				w.source.Status.Error(err)
				log.Warn("Couldn't read message from connection: ", err)
				return
			}
			w.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}
	}
}

// mustKeepConnAlive returns if the connection must remain open
// returns false otherwise
func (w *Worker) mustKeepConnAlive() bool {
	switch w.source.Config.Type {
	case config.UDPType:
		return true
	case config.TCPType:
		// prevent client from keeping too many open connections
		return false
	}
	return false
}
