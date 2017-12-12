// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package sender

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// A Sender sends messages from an inputChan to datadog's intake,
// handling connections and retries
type Sender struct {
	inputChan   chan message.Message
	outputChan  chan message.Message
	connManager *ConnectionManager
	conn        net.Conn
}

// New returns an initialized Sender
func New(inputChan, outputChan chan message.Message, connManager *ConnectionManager) *Sender {
	return &Sender{
		inputChan:   inputChan,
		outputChan:  outputChan,
		connManager: connManager,
	}
}

// Start starts the Sender
func (s *Sender) Start() {
	go s.run()
}

// run lets the sender wire messages
func (s *Sender) run() {
	for payload := range s.inputChan {
		s.wireMessage(payload)
	}
}

// wireMessage lets the Sender send a message to datadog's intake
func (s *Sender) wireMessage(payload message.Message) {
	for {
		if s.conn == nil {
			s.conn = s.connManager.NewConnection() // blocks until a new conn is ready
		}
		_, err := s.conn.Write(payload.Content())
		if err != nil {
			s.connManager.CloseConnection(s.conn)
			s.conn = nil
			continue
		}

		s.outputChan <- payload
		return
	}
}
