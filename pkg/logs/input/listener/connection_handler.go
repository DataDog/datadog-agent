// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"io"
	"net"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// defaultTimeout represents the time after which a connection is closed when no data is read
const defaultTimeout = 60000 * time.Millisecond

// ConnectionHandler reads bytes from new connections, passes them to a decoder and
// transforms decoder output into messages to forward them
type ConnectionHandler struct {
	pp          pipeline.Provider
	source      *config.IntegrationConfigLogSource
	connections []net.Conn
	shouldStop  bool
	mu          *sync.Mutex
	wg          *sync.WaitGroup
}

// NewConnectionHandler returns a new ConnectionHandler
func NewConnectionHandler(pp pipeline.Provider, source *config.IntegrationConfigLogSource) *ConnectionHandler {
	return &ConnectionHandler{
		pp:          pp,
		source:      source,
		connections: []net.Conn{},
		mu:          &sync.Mutex{},
		wg:          &sync.WaitGroup{},
	}
}

// HandleConnection reads bytes from a connection and passes them to a decoder
func (h *ConnectionHandler) HandleConnection(conn net.Conn) {
	h.mu.Lock()
	h.connections = append(h.connections, conn)
	h.wg.Add(1)
	decoder := decoder.InitializeDecoder(h.source)
	decoder.Start()
	go h.forwardMessages(decoder, h.pp.NextPipelineChan())
	go h.readForever(conn, decoder)
	h.mu.Unlock()
}

// Stop closes all open connections and waits for all data read to be decoded
func (h *ConnectionHandler) Stop() {
	h.mu.Lock()
	h.shouldStop = true
	for _, conn := range h.connections {
		conn.Close()
	}
	h.connections = h.connections[:0]
	h.wg.Wait()
	h.mu.Unlock()
}

// forwardMessages forwards messages to output channel
func (h *ConnectionHandler) forwardMessages(d *decoder.Decoder, outputChan chan message.Message) {
	for output := range d.OutputChan {
		if output.ShouldStop {
			h.wg.Done()
			return
		}

		netMsg := message.NewNetworkMessage(output.Content)
		o := message.NewOrigin()
		o.LogSource = h.source
		netMsg.SetOrigin(o)
		outputChan <- netMsg
	}
}

// readForever reads the data from conn until timeout or an error or EOF is reached
func (h *ConnectionHandler) readForever(conn net.Conn, d *decoder.Decoder) {
	defer func() {
		conn.Close()
		d.Stop()
	}()

	for {
		conn.SetReadDeadline(time.Now().Add(defaultTimeout))
		inBuf := make([]byte, 4096)
		n, err := conn.Read(inBuf)
		if err == io.EOF {
			return
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return
		}
		if err != nil {
			h.source.Tracker.TrackError(err)
			log.Warn("Couldn't read message from connection: ", err)
			return
		}
		d.InputChan <- decoder.NewInput(inBuf[:n])
	}
}
