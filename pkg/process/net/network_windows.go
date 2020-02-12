// +build windows

package net

import (
	"net"

	"github.com/Datadog/datadog-agent/pkg/process/config"
)

func NewListener(cfg *config.AgentConfig) (*net.Listener, error) {
	conn, err := net.Listen("tcp", "localhost:3333")
	defer conn.Close()

	return conn
}
