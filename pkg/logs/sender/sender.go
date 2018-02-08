// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	log "github.com/cihub/seelog"
)

// A Sender sends messages from an inputChan to datadog's intake,
// handling connections and retries.
type Sender struct {
	inputChan      chan message.Message
	outputChan     chan message.Message
	connManager    *ConnectionManager
	conn           net.Conn
	framePrefix    []byte
	frameDelimiter Delimiter
	done           chan struct{}
}

// New returns an initialized Sender
func New(inputChan, outputChan chan message.Message, connManager *ConnectionManager, frameDelimiter Delimiter) *Sender {
	return &Sender{
		inputChan:      inputChan,
		outputChan:     outputChan,
		connManager:    connManager,
		framePrefix:    getFramePrefix(),
		frameDelimiter: frameDelimiter,
		done:           make(chan struct{}),
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
		frame, err := s.toFrame(payload.Content())
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

func (s *Sender) toFrame(content []byte) ([]byte, error) {
	// Prefix the content with the API key
	payload := append(s.framePrefix, content...)
	// As we write into a raw socket, add a delimiter to mark subsequent frames
	return s.frameDelimiter.delimit(payload)
}

// getFramePrefix returns an API key that is prepended to each message sent to check is authenticity.
func getFramePrefix() []byte {
	apikey := config.LogsAgent.GetString("api_key")
	logset := config.LogsAgent.GetString("logset") // TODO Logset is deprecated and should be removed eventually.
	if logset != "" {
		apikey = fmt.Sprintf("%s/%s", apikey, logset)
	}
	return append([]byte(apikey), ' ')
}
