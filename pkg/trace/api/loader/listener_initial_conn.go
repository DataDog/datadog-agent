// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loader

import (
	"net"
	"sync/atomic"
)

type listenerInitialConn struct {
	net.Listener

	initialDone atomic.Bool
	initialConn net.Conn
}

// NewListenerInitialConn returns a listener which returns the initialConn on the first call to Accept
func NewListenerInitialConn(inner net.Listener, initialConn net.Conn) net.Listener {
	return &listenerInitialConn{
		Listener:    inner,
		initialConn: initialConn,
	}
}

func (l *listenerInitialConn) Accept() (net.Conn, error) {
	if l.initialDone.CompareAndSwap(false, true) {
		return l.initialConn, nil
	}
	return l.Listener.Accept()
}
