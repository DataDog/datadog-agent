// +build windows

package net

import (
	"net"
)

// UDSListener (Unix Domain Socket Listener)
type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

func NewPipeListener(cfg *config.AgentConfig) (*WindowsPipeListener, error) {

}
