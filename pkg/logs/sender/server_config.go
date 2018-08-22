// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"fmt"
)

// ServerConfig holds the network configuration of the server to send logs to.
type ServerConfig struct {
	Name   string
	Port   int
	UseSSL bool
}

// NewServerConfig returns a new server config.
func NewServerConfig(name string, port int, useSSL bool) ServerConfig {
	return ServerConfig{
		Name:   name,
		Port:   port,
		UseSSL: useSSL,
	}
}

// Address returns the address of the server to send logs to.
func (c ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Name, c.Port)
}
