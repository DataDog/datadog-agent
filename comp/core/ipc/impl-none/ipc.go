// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package noneimpl implements a noop version of the auth_token component
package noneimpl

import (
	"crypto/tls"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
)

type ipcComponent struct {
}

// Provides defines the output of the ipc component
type Provides struct {
	Comp ipc.Component
}

// NewNoopIPC return a void implementation of the ipc.Component
func NewNoopIPC() Provides {
	return Provides{
		Comp: &ipcComponent{},
	}
}

// GetAuthToken returns the session token
func (ipc *ipcComponent) GetAuthToken() string {
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

func (ipc *ipcComponent) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Noop
		next.ServeHTTP(w, r)
	})
}

func (ipc *ipcComponent) GetClient() ipc.HTTPClient {
	return nil // TODO IPC: could panic if dereferenced
}
