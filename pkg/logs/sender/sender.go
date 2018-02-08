// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	log "github.com/cihub/seelog"
)

// A Sender sends messages from an inputChan to datadog's intake,
// handling connections and retries.
type Sender struct {
	inputChan   chan message.Message
	outputChan  chan message.Message
	connManager *ConnectionManager
	conn        net.Conn
	delimiter   Delimiter
	done        chan struct{}
}

// New returns an initialized Sender
func New(inputChan, outputChan chan message.Message, connManager *ConnectionManager, delimiter Delimiter) *Sender {
	return &Sender{
		inputChan:   inputChan,
		outputChan:  outputChan,
		connManager: connManager,
		delimiter:   delimiter,
		done:        make(chan struct{}),
	}
}

// Start starts the Sender
func (s *Sender) Start() {
	go s.run()
}

// Stop stops the Sender,
// this call blocks until inputChan is flushed
func (s *Sender) Stop() {
	close(s.inputChan)
	<-s.done
}

// run lets the sender wire messages
func (s *Sender) run() {
	defer func() {
		s.done <- struct{}{}
	}()
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
		frame, err := s.delimiter.delimit(payload.Content())
		if err != nil {
			log.Error("can't send payload: ", payload, err)
			continue
		}
		_, err = s.conn.Write(frame)
		if err != nil {
			s.connManager.CloseConnection(s.conn)
			s.conn = nil
			continue
		}
		s.outputChan <- payload
		return
	}
}
