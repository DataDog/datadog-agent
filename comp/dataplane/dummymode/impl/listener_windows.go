// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package dummymodeimpl

import (
	"fmt"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

// pipeNamePrefix matches comp/dogstatsd/listeners/named_pipe_windows.go.
const pipeNamePrefix = `\\.\pipe\`

// listener describes the throwaway DogStatsD endpoint the dummy ADP binds. On Windows
// there is no unix socket support in the DogStatsD listeners, so a named pipe is used
// instead. The pipe name embeds the Core Agent's PID so that a leftover pipe from a
// previous run cannot be mistaken for this one.
type listener struct {
	pipeName string
}

// newListener builds the listener description for a working directory. workDir is unused
// on Windows because a named pipe is not a filesystem object.
func newListener(_ string) listener {
	return listener{pipeName: fmt.Sprintf("dd-adp-dummy-%d", os.Getpid())}
}

// validate reports whether the endpoint can be bound at all. Named pipe names are not
// filesystem paths, so there is no length constraint worth pre-checking here.
func (l listener) validate() error {
	return nil
}

// configOverrides returns the DogStatsD listener settings for the generated ADP config.
func (l listener) configOverrides() map[string]any {
	return map[string]any{
		"dogstatsd_pipe_name": l.pipeName,
		// Avoid any inherited value from causing ADP to try and bind to a Unix domain socket
		// on a non-Unix platform.
		"dogstatsd_socket": "",
	}
}

// probeAddr is the address the probe's DogStatsD client dials.
func (l listener) probeAddr() string {
	return pipeNamePrefix + l.pipeName
}

// ready reports whether ADP has bound the endpoint yet. A named pipe is not visible
// through os.Stat, so this dials it; the connection is closed immediately because the
// probe opens its own.
func (l listener) ready() bool {
	timeout := 100 * time.Millisecond
	conn, err := winio.DialPipe(l.probeAddr(), &timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// describe returns a human-readable form for logs.
func (l listener) describe() string {
	return l.probeAddr()
}
