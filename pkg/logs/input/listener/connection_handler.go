// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	"io"
	"log"
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// ConnectionHandler reads bytes from new connections, passes them to a decoder and
// transforms decoder output into messages to forward them
type ConnectionHandler struct {
	pp     pipeline.Provider
	source *config.IntegrationConfigLogSource
}

// forwardMessages forwards messages to output channel
func (connHandler *ConnectionHandler) forwardMessages(d *decoder.Decoder, outputChan chan message.Message) {
	for output := range d.OutputChan {
		if output.ShouldStop {
			return
		}

		netMsg := message.NewNetworkMessage(output.Content)
		o := message.NewOrigin()
		o.LogSource = connHandler.source
		netMsg.SetOrigin(o)
		outputChan <- netMsg
	}
}

// handleConnection reads bytes from a connection and passes them to a decoder
func (connHandler *ConnectionHandler) handleConnection(conn net.Conn) {
	d := decoder.InitializeDecoder(connHandler.source)
	d.Start()
	go connHandler.forwardMessages(d, connHandler.pp.NextPipelineChan())
	for {
		inBuf := make([]byte, 4096)
		n, err := conn.Read(inBuf)
		if err == io.EOF {
			d.Stop()
			return
		}
		if err != nil {
			log.Println("Couldn't read message from connection:", err)
			d.Stop()
			return
		}
		d.InputChan <- decoder.NewInput(inBuf[:n])
	}
}
