// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFloat64Bytes(t *testing.T) {
	assert := assert.New(t)

	data := []byte{
		0x2a,             // 42
		0xd1, 0xfb, 0x2e, // -1234
		0xcd, 0x0a, 0x9b, // 2715
		0xcb, 0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // float64(3.14)
	}

	var (
		f   float64
		err error
	)
	bts := data

	f, bts, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(42.0, f)

	f, bts, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(-1234.0, f)

	f, bts, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(2715.0, f)

	f, _, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(3.14, f)
}

func TestDecodeBytes(t *testing.T) {
	want := Traces{
		{{Service: "A", Name: "op"}},
		{{Service: "B"}},
		{{Service: "C"}},
	}

	var (
		bts []byte
		err error
	)
	if bts, err = want.MarshalMsg(nil); err != nil {
		t.Fatal(err)
	}
	var got Traces
	if _, err = got.UnmarshalMsg(bts); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, want, got)
}

func TestDecodeInvalidUTF8Bytes(t *testing.T) {
	provide := Traces{
		{&Span{Service: "A", Name: "op\x99\xbf"}},
		{&Span{Service: "B"}},
		{&Span{Service: "C"}},
	}
	accept := Traces{
		{&Span{Service: "A", Name: "op��"}},
		{&Span{Service: "B"}},
		{&Span{Service: "C"}},
	}
	var (
		bts []byte
		err error
	)
	if bts, err = provide.MarshalMsg(nil); err != nil {
		t.Fatal(err)
	}
	var got Traces
	if _, err = got.UnmarshalMsg(bts); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, accept, got)
}
