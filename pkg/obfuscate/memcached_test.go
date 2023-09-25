// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObfuscateMemcached(t *testing.T) {
	for _, tt := range []struct {
		in, out string
	}{
		{
			"set mykey 0 60 5\r\nvalue",
			"set mykey 0 60 5",
		},
		{
			"get mykey",
			"get mykey",
		},
		{
			"add newkey 0 60 5\r\nvalue",
			"add newkey 0 60 5",
		},
		{
			"add newkey 0 60 5\r\nvalue",
			"add newkey 0 60 5",
		},
		{
			"decr mykey 5",
			"decr mykey 5",
		},
	} {
		assert.Equal(t, tt.out, NewObfuscator(Config{}).ObfuscateMemcachedString(tt.in))
	}
}

func TestObfuscateMemcachedRemoveKey(t *testing.T) {
	for _, tt := range []struct {
		in, out string
	}{
		{
			"set mykey 0 60 5\r\nvalue",
			"set ? 0 60 5",
		},
		{
			"get mykey",
			"get ?",
		},
		{
			"add newkey 0 60 5\r\nvalue",
			"add ? 0 60 5",
		},
		{
			"add newkey 0 60 5\r\nvalue",
			"add ? 0 60 5",
		},
		{
			"decr mykey 5",
			"decr ? 5",
		},
		{
			"get get get",
			"get ? get",
		},
		{
			"get", // this is invalid, but it shouldn't crash
			"get",
		},
		{
			"get\r\nvalue", // this is invalid, but it shouldn't crash
			"get",
		},
	} {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.out, NewObfuscator(Config{Memcached: MemcachedConfig{
				Enabled:   true,
				RemoveKey: true,
			}}).ObfuscateMemcachedString(tt.in))
		})
	}
}
