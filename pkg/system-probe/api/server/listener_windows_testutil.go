// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && windows

package server

import (
	"net"
	"os/user"
)

// NewListenerForCurrentUser sets up a named pipe listener for tests that mock system probe.
// Do not use this for the normal system probe named pipe.
func NewListenerForCurrentUser(namedPipeName string) (net.Listener, error) {
	// Prepare a security descriptor that allows the current user.
	currentUser, err := user.Current()
	if err != nil {
		return nil, err
	}

	sd, err := formatSecurityDescriptorWithSid(currentUser.Uid)
	if err != nil {
		return nil, err
	}

	return newListenerWithSecurityDescriptor(namedPipeName, sd)
}
