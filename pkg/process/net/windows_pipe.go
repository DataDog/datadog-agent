// +build windows

package net

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"net"
)

type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

// Sets up a TCP listener for now, will eventually be a named pipe
func NewListener(cfg *config.AgentConfig) (*WindowsPipeListener, error) {
	l, err := net.Listen("tcp", "localhost:3333")
	return &WindowsPipeListener{l, "path"}, err
}

func (wp *WindowsPipeListener) GetListener() net.Listener {
	return wp.conn
}

// Stop closes the UDSListener connection and stops listening
func (wp *WindowsPipeListener) Stop() {
	wp.conn.Close()
}
