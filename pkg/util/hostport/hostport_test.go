// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package hostport

import "testing"

func TestJoin(t *testing.T) {
	for _, tc := range []struct {
		name, host, port, want string
	}{
		{"ipv4", "1.2.3.4", "443", "1.2.3.4:443"},
		{"hostname", "example.com", "443", "example.com:443"},
		{"ipv6 bare", "fd38::1", "443", "[fd38::1]:443"},
		{"ipv6 bracketed", "[fd38::1]", "443", "[fd38::1]:443"},
		{"ipv6 loopback bare", "::1", "443", "[::1]:443"},
		{"ipv6 loopback bracketed", "[::1]", "443", "[::1]:443"},
		{"ipv6 listen-all bare", "::", "443", "[::]:443"},
		{"ipv6 listen-all bracketed", "[::]", "443", "[::]:443"},
		{"empty host", "", "443", ":443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Join(tc.host, tc.port); got != tc.want {
				t.Errorf("Join(%q, %q) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
		})
	}
}
