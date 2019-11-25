// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package obfuscate

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestObfuscateMemcached(t *testing.T) {
	const k = "memcached.command"
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
		span := pb.Span{
			Type: "memcached",
			Meta: map[string]string{k: tt.in},
		}
		NewObfuscator(nil).obfuscateMemcached(&span)
		assert.Equal(t, tt.out, span.Meta[k])
	}
}
