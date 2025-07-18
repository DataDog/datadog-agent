// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

// SourceSinkHandle contains a platform's Source and Sink implementation
type SourceSinkHandle struct {
	Source Source
	Sink   Sink
	// MustClosePort means the traceroute must close the handle they used to reserve a port.
	// It's a Windows-specific hack -- on Windows, you can't actually capture all
	// packets with a raw socket.  By reserving a port, packets go to that socket instead of your
	// raw socket. This can only be addressed using a Windows driver.
	MustClosePort bool
}
