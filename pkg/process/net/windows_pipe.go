// +build windows

package net

import (
	"net"
	"github.com/DataDog/datadog-agent/pkg/process/config"

)

// UDSListener (Unix Domain Socket Listener)
type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

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
