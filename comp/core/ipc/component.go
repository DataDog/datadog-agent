// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package ipc takes care of the IPC artifacts lifecycle (creation, loading, deletion of auth_token, IPC certificate, IPC key).
// It also provides helpers to use them in the agent (TLS configuration, HTTP client, etc.).
package ipc

import (
	"crypto/tls"
)

// team: agent-runtimes

// Params defines the parameters for this component.
type Params struct {
	// AllowWriteArtifacts is a boolean that determines whether the component should allow writing auth artifacts on file system
	// or only read them.
	AllowWriteArtifacts bool
}

func ForDaemon() Params {
	return Params{
		AllowWriteArtifacts: true,
	}
}

func ForOneShot() Params {
	return Params{
		AllowWriteArtifacts: false,
	}
}

// Component is the component type.
type Component interface {
	Get() string
	GetTLSClientConfig() *tls.Config
	GetTLSServerConfig() *tls.Config
}
