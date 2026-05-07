// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package hostport provides IPv6-aware host:port helpers.
package hostport

import "net"

// Join combines a host and port into the canonical "host:port" form, or
// "[host]:port" for IPv6 literals. Unlike net.JoinHostPort, it accepts an
// IPv6 host in either bare (`fd38::1`) or already-bracketed (`[fd38::1]`)
// form: net.JoinHostPort would otherwise double-bracket the latter.
func Join(host, port string) string {
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	return net.JoinHostPort(host, port)
}
