// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package remote provides interfaces for remote device communications (SSH/Telnet) to retrieve configurations
package remote

import "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"

// Client defines the interface for a remote client that can create sessions to execute commands on a device
type Client interface {
	Connect() error
	NewSession() (Session, error)
	RetrieveRunningConfig() ([]byte, error)
	RetrieveStartupConfig() ([]byte, error)
	SetProfile(p *profile.NCMProfile)
	Close() error
}

// Session defines the interface for a session that can execute commands on a remote device
type Session interface {
	CombinedOutput(cmd string) ([]byte, error)
	Close() error
}
