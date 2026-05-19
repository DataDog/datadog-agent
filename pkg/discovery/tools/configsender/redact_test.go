// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactSensitive(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// redis.conf style (space-separated)
		{
			name: "requirepass with value",
			in:   "requirepass mySecret123",
			want: "requirepass [REDACTED]",
		},
		{
			name: "masterauth with trailing comment",
			in:   "masterauth secret  # used for replica",
			want: "masterauth [REDACTED]  # used for replica",
		},
		{
			name: "redis.conf full sample",
			in: "bind 127.0.0.1\nport 6379\nrequirepass topSecret\nmaxmemory 256mb\n",
			want: "bind 127.0.0.1\nport 6379\nrequirepass [REDACTED]\nmaxmemory 256mb\n",
		},

		// yaml / colon-separated
		{
			name: "yaml password unquoted",
			in:   "password: hunter2",
			want: "password: [REDACTED]",
		},
		{
			name: "yaml password quoted",
			in:   `password: "hunter2"`,
			want: `password: [REDACTED]`,
		},
		{
			name: "yaml nested dotted key",
			in:   "xpack.security.transport.ssl.keystore.password: foo",
			want: "xpack.security.transport.ssl.keystore.password: [REDACTED]",
		},
		{
			name: "yaml indented key",
			in:   "  api_key: deadbeef",
			want: "  api_key: [REDACTED]",
		},

		// ini / equals-separated
		{
			name: "ini-style equals",
			in:   "auth_token=abc123",
			want: "auth_token=[REDACTED]",
		},

		// non-secrets must not be touched
		{
			name: "bind address kept",
			in:   "bind 127.0.0.1",
			want: "bind 127.0.0.1",
		},
		{
			name: "port kept",
			in:   "port: 6379",
			want: "port: 6379",
		},
		{
			name: "comment mentioning password is not modified",
			in:   "# read the docs about password rotation",
			want: "# read the docs about password rotation",
		},
		{
			name: "key with empty value not modified",
			in:   "password:",
			want: "password:",
		},

		// nested instance (yaml lists with key/value)
		{
			name: "yaml list element with password",
			in:   "  - host: localhost\n    port: 6379\n    password: realsecret",
			want: "  - host: localhost\n    port: 6379\n    password: [REDACTED]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(redactSensitive([]byte(tc.in)))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRedactDoesNotPanicOnEmpty(t *testing.T) {
	assert.Equal(t, "", string(redactSensitive(nil)))
	assert.Equal(t, "", string(redactSensitive([]byte(""))))
}
