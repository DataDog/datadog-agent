// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package failure holds the reasons a network device probe can fail.
package failure

// Reasons a probe reports when it cannot reach a device.
const (
	None                     = "none"
	Unreachable              = "unreachable"
	Timeout                  = "timeout"
	ConnectionRefused        = "connection_refused"
	HostUnreachable          = "host_unreachable"
	NetworkUnreachable       = "network_unreachable"
	AuthenticationFailed     = "authentication_failed"
	DecryptionFailed         = "decryption_failed"
	UnknownUser              = "unknown_user"
	UnsupportedSecurityLevel = "unsupported_security_level"
	UnknownEngineID          = "unknown_engine_id"
	Unknown                  = "unknown"
)
