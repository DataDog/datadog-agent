// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package noneimpl implements a noop version of the auth_token component
package noneimpl

import (
	"crypto/tls"

	"github.com/DataDog/datadog-agent/comp/core/ipc"
)

type ipcComponent struct {
}

var _ ipc.Component = (*ipcComponent)(nil)

// NewNoopIPC return a void implementation of the ipc.Component
func NewNoopIPC() ipc.Component {
	return &ipcComponent{}
}

// Get returns the session token
func (ipc *ipcComponent) Get() string {
	return ""
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (ipc *ipcComponent) GetTLSClientConfig() *tls.Config {
	return &tls.Config{}
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (ipc *ipcComponent) GetTLSServerConfig() *tls.Config {
	return &tls.Config{}
}
