package api

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteCounter(t *testing.T) {
	assert := assert.New(t)
	var buf bytes.Buffer
	wc := newWriteCounter(&buf)
	assert.Zero(wc.N())
	wc.Write([]byte{1})
	wc.Write([]byte{2})
	assert.EqualValues(wc.N(), 2)
	assert.EqualValues(buf.Bytes(), []byte{1, 2})
	wc.Write([]byte{3})
	assert.EqualValues(wc.N(), 3)
	assert.EqualValues(buf.Bytes(), []byte{1, 2, 3})
}
