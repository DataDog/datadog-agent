package model

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDecodeZstd05Payload ensures backward compatibility with our intake
func TestDecodeZstd05Payload(t *testing.T) {
	file := "./test_zstd.0.5.dump"
	expected := Message{
		Header: MessageHeader{
			Version:  MessageV3,
			Encoding: MessageEncodingZstdPB,
			Type:     TypeCollectorProc,
		},
		Body: &CollectorProc{
			HostName: "test",
		},
	}

	raw, err := ioutil.ReadFile(file)
	assert.NoError(t, err)

	msg, err := DecodeMessage(raw)
	assert.NoError(t, err)

	assert.Equal(t, expected, msg)
}
