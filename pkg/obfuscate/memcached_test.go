// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObfuscateMemcachedKeepCommand(t *testing.T) {
	for _, tt := range []struct {
		in, out     string
		keepCommand bool
	}{
		{
			"set mykey 0 60 5\r\nvalue",
			"set mykey 0 60 5",
			true,
		},
		{
			"get mykey",
			"get mykey",
			true,
		},
		{
			"add newkey 0 60 5\r\nvalue",
			"add newkey 0 60 5",
			true,
		},
		{
			"add newkey 0 60 5\r\nvalue",
			"add newkey 0 60 5",
			true,
		},
		{
			"decr mykey 5",
			"decr mykey 5",
			true,
		},
		{
			"set mykey 0 60 5\r\nvalue",
			"",
			false,
		},
		{
			"get mykey",
			"",
			false,
		},
		{
			"get", // this is invalid, but it shouldn't crash
			"",
			false,
		},
		{
			"get\r\nvalue", // this is invalid, but it shouldn't crash
			"",
			false,
		},
	} {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.out, NewObfuscator(Config{Memcached: MemcachedConfig{
				Enabled:     true,
				KeepCommand: tt.keepCommand,
			}}).ObfuscateMemcachedString(tt.in))
		})
	}
}
