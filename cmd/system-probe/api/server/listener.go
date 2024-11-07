// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package server

import "net"

// Listener is a wrapper over some net.Listener
type Listener interface {
	// GetListener returns the underlying net.Listener
	GetListener() net.Listener

	// Stop and clean up resources for the underlying connection
	Stop()
}
