// +build windows

package net

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"net"
)

// WindowsPipeListener for communicating with Probe
type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

// NewListener sets up a TCP listener for now, will eventually be a named pipe
func NewListener(cfg *config.AgentConfig) (*WindowsPipeListener, error) {
	l, err := net.Listen("tcp", "localhost:3333")
	return &WindowsPipeListener{l, "path"}, err
}

// GetListener will return underlying Listener's conn
func (wp *WindowsPipeListener) GetListener() net.Listener {
	return wp.conn
}

// Stop closes the WindowsPipeListener connection and stops listening
func (wp *WindowsPipeListener) Stop() {
	wp.conn.Close()
}
