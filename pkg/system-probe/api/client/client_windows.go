// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	idleConnTimeout = 5 * time.Second
)

// DialContextFunc returns a function to be used in http.Transport.DialContext for connecting to system-probe.
// The result will be OS-specific.
func DialContextFunc(namedPipePath string) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		// Go clients do not immediately close (named pipe) connections when done,
		// they keep connections idle for a while.  Make sure the idle time
		// is not too high and the timeout is generous enough for pending connections.
		var timeout = 30 * time.Second

		namedPipe, err := winio.DialPipe(namedPipePath, &timeout)
		if err != nil {
			// Since connection errors can be expected at startup we must not
			// log an error here above debug level. We expect this error to be
			// silenced for a startup period before being logged by the caller,
			// see IgnoreStartupError.
			err = fmt.Errorf("error connecting to named pipe %q: %w", namedPipePath, err)
			// We log here at debug level for diagnostics in case it is not logged by a caller.
			log.Debugf("%s", err)
			return nil, err
		}

		return namedPipe, nil
	}
}
