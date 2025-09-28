// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	grpc "github.com/DataDog/datadog-agent/pkg/logs/sender/grpc"
)

// TestGRPCEncoder tests that GRPCEncoder creates valid protobuf structures
func TestGRPCEncoder(t *testing.T) {
	// Create a test message
	msg := message.NewMessage([]byte("test log message"), nil, "", time.Now().UnixNano())
	msg.State = message.StateRendered

	// Encode with GRPCEncoder
	err := GRPCEncoder.Encode(msg, "test-hostname")
	require.NoError(t, err, "GRPCEncoder should encode message without error")

	// Verify the message has a gRPC datum
	grpcDatumAny := msg.GetGRPCDatum()
	require.NotNil(t, grpcDatumAny, "Message should have gRPC datum after encoding")

	// Type assert to *grpc.Datum
	datum, ok := grpcDatumAny.(*grpc.Datum)
	require.True(t, ok, "gRPC datum should be *grpc.Datum type")
	require.NotNil(t, datum, "Datum should not be nil")

	// Verify datum contains a Log
	log := datum.GetLogs()
	require.NotNil(t, log, "Datum should contain a Log")

	// Verify log structure
	assert.NotZero(t, log.GetTimestamp(), "Log should have timestamp")
	assert.Equal(t, "test log message", log.GetRaw(), "Log content should match original message")

	// Verify message state is set to encoded
	assert.Equal(t, message.StateEncoded, msg.State, "Message state should be set to encoded")
}

// TestGRPCEncoderWithDifferentContent tests encoding various content types
func TestGRPCEncoderWithDifferentContent(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"Empty message", ""},
		{"Simple text", "Hello, world!"},
		{"Unicode content", "Hello, 世界! 🌍"},
		{"JSON content", `{"level": "info", "message": "test"}`},
		{"Multiline content", "Line 1\nLine 2\nLine 3"},
		{"Binary-like content", "\x00\x01\x02\x03"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := message.NewMessage([]byte(tc.content), nil, "", time.Now().UnixNano())
			msg.State = message.StateRendered

			err := GRPCEncoder.Encode(msg, "test-hostname")
			require.NoError(t, err, "Should encode without error")

			grpcDatumAny := msg.GetGRPCDatum()
			require.NotNil(t, grpcDatumAny, "Should have gRPC datum")

			datum, ok := grpcDatumAny.(*grpc.Datum)
			require.True(t, ok, "Should be *grpc.Datum")

			log := datum.GetLogs()
			require.NotNil(t, log, "Should have log")

			// For binary content, toValidUtf8 may modify it, so we check it's not empty
			if tc.content == "" {
				assert.Equal(t, tc.content, log.GetRaw(), "Empty content should be preserved")
			} else {
				assert.NotEmpty(t, log.GetRaw(), "Content should not be empty after encoding")
			}
			assert.NotZero(t, log.GetTimestamp(), "Should have timestamp")
		})
	}
}

// TestGRPCEncoderRequiresRenderedState tests that encoder requires rendered state
func TestGRPCEncoderRequiresRenderedState(t *testing.T) {
	msg := message.NewMessage([]byte("test"), nil, "", time.Now().UnixNano())

	// Test with unrendered state - should fail
	msg.State = message.StateUnstructured
	err := GRPCEncoder.Encode(msg, "test-hostname")
	require.Error(t, err, "Should fail to encode unstructured message")
	assert.Contains(t, err.Error(), "isn't rendered", "Error should mention rendering requirement")

	// Test with rendered state - should succeed
	msg.State = message.StateRendered
	err = GRPCEncoder.Encode(msg, "test-hostname")
	require.NoError(t, err, "Should encode rendered message")

	// Verify state is set to encoded
	assert.Equal(t, message.StateEncoded, msg.State, "Message state should be encoded")
}

// TestMessageGRPCDatumMethods tests the gRPC datum getter/setter methods
func TestMessageGRPCDatumMethods(t *testing.T) {
	msg := message.NewMessage([]byte("test"), nil, "", time.Now().UnixNano())

	// Initially should be nil
	assert.Nil(t, msg.GetGRPCDatum(), "New message should not have gRPC datum")

	// Create a test datum
	datum := &grpc.Datum{
		Data: &grpc.Datum_Logs{
			Logs: &grpc.Log{
				Timestamp: uint64(time.Now().UnixNano()),
				Content: &grpc.Log_Raw{
					Raw: "test content",
				},
			},
		},
	}

	// Set the datum
	msg.SetGRPCDatum(datum)

	// Verify it was set
	retrievedDatum := msg.GetGRPCDatum()
	require.NotNil(t, retrievedDatum, "Should have gRPC datum after setting")

	// Verify it's the same datum
	assert.Equal(t, datum, retrievedDatum, "Retrieved datum should be the same as set")

	// Verify state was set to encoded
	assert.Equal(t, message.StateEncoded, msg.State, "State should be encoded after setting gRPC datum")
}

// TestPayloadGRPCDatums tests that payloads can hold gRPC datums
func TestPayloadGRPCDatums(t *testing.T) {
	// Create some test datums
	datum1 := &grpc.Datum{
		Data: &grpc.Datum_Logs{
			Logs: &grpc.Log{
				Timestamp: uint64(time.Now().UnixNano()),
				Content: &grpc.Log_Raw{
					Raw: "message 1",
				},
			},
		},
	}

	datum2 := &grpc.Datum{
		Data: &grpc.Datum_Logs{
			Logs: &grpc.Log{
				Timestamp: uint64(time.Now().UnixNano()),
				Content: &grpc.Log_Raw{
					Raw: "message 2",
				},
			},
		},
	}

	// Create payload with gRPC datums
	payload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{},
		Encoded:       []byte{},
		Encoding:      "",
		UnencodedSize: 0,
		GRPCDatums:    []any{datum1, datum2},
	}

	// Verify payload structure
	assert.Len(t, payload.GRPCDatums, 2, "Payload should contain 2 gRPC datums")

	// Verify each datum can be retrieved and used
	for i, datumAny := range payload.GRPCDatums {
		datum, ok := datumAny.(*grpc.Datum)
		require.True(t, ok, "Should be *grpc.Datum")

		log := datum.GetLogs()
		require.NotNil(t, log, "Should have log")

		expectedContent := "message " + string(rune('1'+i))
		assert.Equal(t, expectedContent, log.GetRaw(), "Content should match expected")
	}
}
