// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"io"
	"sync"

	"github.com/godbus/dbus/v5"
)

// rejectManagerInboundMethodCalls closes the transport before godbus dispatches
// an unsolicited method call. This client exports no objects; accepting method
// calls would only expose godbus's goroutine-per-call server path to an
// untrusted peer. The current call may still reach a single fail-closed handler,
// but closing the transport prevents any subsequent call from being decoded.
func rejectManagerInboundMethodCalls(transport io.Closer) dbus.Interceptor {
	var closeOnce sync.Once
	return func(message *dbus.Message) {
		if message != nil && message.Type == dbus.TypeMethodCall {
			closeOnce.Do(func() { _ = transport.Close() })
		}
	}
}
