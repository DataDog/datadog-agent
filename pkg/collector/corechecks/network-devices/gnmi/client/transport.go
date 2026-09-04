// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"crypto/tls"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TransportMode describes how the client secures the gRPC transport.
type TransportMode string

const (
	// TransportInsecure uses plaintext gRPC.
	TransportInsecure TransportMode = "insecure"
	// TransportTLS uses TLS for the gRPC transport.
	TransportTLS TransportMode = "tls"
)

// TransportConfig holds transport-layer settings for a gNMI client.
type TransportConfig struct {
	UseTLS             bool
	InsecureSkipVerify bool
}

func (c TransportConfig) mode() TransportMode {
	if c.UseTLS {
		return TransportTLS
	}
	return TransportInsecure
}

func (c TransportConfig) transportCredentials() credentials.TransportCredentials {
	if !c.UseTLS {
		return insecure.NewCredentials()
	}
	return credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify, //nolint:gosec // user-controlled for lab devices
		MinVersion:         tls.VersionTLS12,
	})
}
