// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//+build zlib

package jsonstream

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

type dummyMarshaller struct {
	items  []string
	header string
	footer string
}

func (d *dummyMarshaller) JSONHeader() []byte {
	return []byte(d.header)
}

func (d *dummyMarshaller) Len() int {
	return len(d.items)
}

func (d *dummyMarshaller) JSONItem(i int) ([]byte, error) {
	if i < 0 || i > d.Len()-1 {
		return nil, errors.New("out of range")
	}
	return []byte(d.items[i]), nil
}

func (d *dummyMarshaller) JSONFooter() []byte {
	return []byte(d.footer)
}

func payloadToString(payload []byte) string {
	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return err.Error()
	}
	defer r.Close()

	dst, err := ioutil.ReadAll(r)
	if err != nil {
		return err.Error()
	}
	return string(dst)
}

func TestCompressorSimple(t *testing.T) {
	c, err := newCompressor([]byte("{["), []byte("]}"))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		c.addItem([]byte("A"))
	}

	p, err := c.close()
	require.NoError(t, err)
	require.Equal(t, "{[A,A,A,A,A]}", payloadToString(p))
}

func TestOnePayloadSimple(t *testing.T) {
	m := &dummyMarshaller{
		items:  []string{"A", "B", "C"},
		header: "{[",
		footer: "]}",
	}

	payloads, err := Payloads(m)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	require.Equal(t, "{[A,B,C]}", payloadToString(*payloads[0]))
}
