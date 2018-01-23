// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"io"
	"net"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// ConnectionHandler reads bytes from new connections, passes them to a decoder and
// transforms decoder output into messages to forward them
type ConnectionHandler struct {
	pp          pipeline.Provider
	source      *config.IntegrationConfigLogSource
	connections []net.Conn
}

// NewConnectionHandler returns a new ConnectionHandler
func NewConnectionHandler(pp pipeline.Provider, source *config.IntegrationConfigLogSource) *ConnectionHandler {
	return &ConnectionHandler{
		pp:          pp,
		source:      source,
		connections: []net.Conn{},
	}
}

// HandleConnection reads bytes from a connection and passes them to a decoder
func (h *ConnectionHandler) HandleConnection(conn net.Conn) {
	h.connections = append(h.connections, conn)
	decoder := decoder.InitializeDecoder(h.source)
	decoder.Start()
	go h.forwardMessages(decoder, h.pp.NextPipelineChan())
	go h.readForever(conn, decoder)
}

// Stop closes all the open connections
func (h *ConnectionHandler) Stop() {
	for _, conn := range h.connections {
		conn.Close()
	}
	h.connections = h.connections[:0]
}

// forwardMessages forwards messages to output channel
func (h *ConnectionHandler) forwardMessages(d *decoder.Decoder, outputChan chan message.Message) {
	for output := range d.OutputChan {
		if output.ShouldStop {
			return
		}

		netMsg := message.NewNetworkMessage(output.Content)
		o := message.NewOrigin()
		o.LogSource = h.source
		netMsg.SetOrigin(o)
		outputChan <- netMsg
	}
}

// readForever reads the data from conn until an error or EOF is reached
func (h *ConnectionHandler) readForever(conn net.Conn, d *decoder.Decoder) {
	for {
		inBuf := make([]byte, 4096)
		n, err := conn.Read(inBuf)
		if err == io.EOF {
			d.Stop()
			return
		}
		if err != nil {
			h.source.Tracker.TrackError(err)
			log.Warn("Couldn't read message from connection: ", err)
			d.Stop()
			return
		}
		d.InputChan <- decoder.NewInput(inBuf[:n])
	}
}
