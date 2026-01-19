// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package grpc

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Helper to create test StatefulMessage with Datum
func createTestStatefulMessage(content string) *message.StatefulMessage {
	msg := message.NewMessage([]byte(content), nil, "", 0)
	msg.MessageMetadata.RawDataLen = len(content)

	datum := &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: 12345,
				Content: &statefulpb.Log_Raw{
					Raw: content,
				},
			},
		},
	}

	return &message.StatefulMessage{
		Metadata: &msg.MessageMetadata,
		Datum:    datum,
	}
}

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(
		input,
		output,
		flushChan,
		100*time.Millisecond,
		2, // maxBatchSize
		1000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message1 := createTestStatefulMessage("a")
	input <- message1

	message2 := createTestStatefulMessage("b")
	input <- message2

	// Expect payload to be sent because buffer is full
	payload := <-output
	assert.Equal(t, 2, len(payload.MessageMetas))
	assert.Equal(t, message1.Metadata, payload.MessageMetas[0])
	assert.Equal(t, message2.Metadata, payload.MessageMetas[1])
	assert.Equal(t, "identity", payload.Encoding)
	assert.Equal(t, 2, payload.UnencodedSize)

	// Verify the payload contains valid DatumSequence
	var datumSeq statefulpb.DatumSequence
	err := proto.Unmarshal(payload.Encoded, &datumSeq)
	require.NoError(t, err)
	assert.Equal(t, 2, len(datumSeq.Data))
	assert.Equal(t, "a", datumSeq.Data[0].GetLogs().GetRaw())
	assert.Equal(t, "b", datumSeq.Data[1].GetLogs().GetRaw())

	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		timerInterval,
		100, // maxBatchSize
		1000,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	for round := 0; round < 3; round++ {
		m := createTestStatefulMessage("test")
		input <- m

		// It should flush in this time
		clk.Add(2 * timerInterval)

		payload := <-output
		assert.EqualValues(t, m.Metadata, payload.MessageMetas[0])

		// Verify payload contains valid DatumSequence
		var datumSeq statefulpb.DatumSequence
		err := proto.Unmarshal(payload.Encoded, &datumSeq)
		require.NoError(t, err)
		assert.Equal(t, 1, len(datumSeq.Data))
	}

	s.Stop()
	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		100*time.Millisecond,
		2,
		1000,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message := createTestStatefulMessage("test")
	input <- message

	go func() {
		s.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	// Expect payload to be sent before timer, so we never advance the clock; if this
	// doesn't work, the test will hang
	payload := <-output
	assert.Equal(t, message.Metadata, payload.MessageMetas[0])
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(
		input,
		output,
		flushChan,
		100*time.Millisecond,
		2,
		1000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message := createTestStatefulMessage("test")
	input <- message

	go func() {
		s.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	payload := <-output
	assert.Equal(t, message.Metadata, payload.MessageMetas[0])
}

func TestBatchStrategySynchronousFlush(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// Batch size is large so it will not flush until we trigger it manually
	// Flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// All of these messages will get buffered
	messages := []*message.StatefulMessage{
		createTestStatefulMessage("a"),
		createTestStatefulMessage("b"),
		createTestStatefulMessage("c"),
	}

	messageMeta := make([]*message.MessageMetadata, len(messages))
	for idx, m := range messages {
		input <- m
		messageMeta[idx] = m.Metadata
	}

	// Since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	go func() {
		// Stop triggers the flush and make sure we can read the messages out now
		strategy.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	payload := <-output
	assert.ElementsMatch(t, messageMeta, payload.MessageMetas)

	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}
}

func TestBatchStrategyFlushChannel(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// Batch size is large so it will not flush until we trigger it manually
	// Flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// All of these messages will get buffered
	messages := []*message.StatefulMessage{
		createTestStatefulMessage("a"),
		createTestStatefulMessage("b"),
		createTestStatefulMessage("c"),
	}
	messageMeta := make([]*message.MessageMetadata, len(messages))
	for idx, m := range messages {
		input <- m
		messageMeta[idx] = m.Metadata
	}

	// Since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	// Trigger a manual flush
	flushChan <- struct{}{}

	payload := <-output
	assert.ElementsMatch(t, messageMeta, payload.MessageMetas)

	// Ensure we read all of the messages
	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}

	// End the test strategy
	go func() {
		// Stop triggers the flush and make sure we can read the messages out now
		strategy.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategyMessageTooLarge(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent deadlock
	flushChan := make(chan struct{})

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10, // Small content size limit
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Send a message that fits
	normalMessage := createTestStatefulMessage("small")
	input <- normalMessage

	// Send a message that's too large (will be dropped)
	largeMessage := createTestStatefulMessage("this message is way too large for the content size limit")
	input <- largeMessage

	// Trigger flush
	flushChan <- struct{}{}

	// Should only receive the normal message
	payload := <-output
	assert.Equal(t, 1, len(payload.MessageMetas))
	assert.Equal(t, normalMessage.Metadata, payload.MessageMetas[0])

	// Verify no more payloads
	select {
	case <-output:
		assert.Fail(t, "should not receive more payloads")
	default:
	}

	strategy.Stop()
}

func TestBatchStrategyInvalidDatum(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent deadlock
	flushChan := make(chan struct{})

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		1000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Send message with nil Datum
	msg1 := message.NewMessage([]byte("test"), nil, "", 0)
	invalidMsg1 := &message.StatefulMessage{
		Metadata: &msg1.MessageMetadata,
		Datum:    nil,
	}
	input <- invalidMsg1

	// Note: With strongly-typed Datum field, wrong type is prevented at compile time

	// Send a valid message
	validMsg := createTestStatefulMessage("valid")
	input <- validMsg

	// Trigger flush
	flushChan <- struct{}{}

	// Should only receive the valid message
	payload := <-output
	assert.Equal(t, 1, len(payload.MessageMetas))
	assert.Equal(t, validMsg.Metadata, payload.MessageMetas[0])

	strategy.Stop()
}

func TestBatchStrategyCompression(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent deadlock
	flushChan := make(chan struct{})

	// Use identity (no-op) compression for simplicity
	// Testing actual compression behavior is covered by the compression package tests
	compressor := compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1)

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10000,
		"test",
		compressor,
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Send several messages
	for i := 0; i < 5; i++ {
		msg := createTestStatefulMessage("test message")
		input <- msg
	}

	// Trigger flush
	flushChan <- struct{}{}

	payload := <-output
	assert.Equal(t, 5, len(payload.MessageMetas))
	assert.Equal(t, "identity", payload.Encoding)
	assert.NotEmpty(t, payload.Encoded)

	// Verify the payload contains valid DatumSequence (identity compression = no compression)
	var datumSeq statefulpb.DatumSequence
	err := proto.Unmarshal(payload.Encoded, &datumSeq)
	require.NoError(t, err)
	assert.Equal(t, 5, len(datumSeq.Data))
	for _, datum := range datumSeq.Data {
		assert.Equal(t, "test message", datum.GetLogs().GetRaw())
	}

	strategy.Stop()
}

// TestBatchStrategyStatefulExtra tests that state changes are correctly tracked in StatefulExtra
func TestBatchStrategyStatefulExtra(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent blocking
	flushChan := make(chan struct{})
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	strategy := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		timerInterval,
		10, // maxBatchSize - large enough to not trigger size-based flush
		1000,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Helper to create state change messages
	createPatternDefineMsg := func(id uint64, template string) *message.StatefulMessage {
		msg := message.NewMessage([]byte(""), nil, "", 0)
		msg.MessageMetadata.RawDataLen = 0
		return &message.StatefulMessage{
			Metadata: &msg.MessageMetadata,
			Datum: &statefulpb.Datum{
				Data: &statefulpb.Datum_PatternDefine{
					PatternDefine: &statefulpb.PatternDefine{
						PatternId: id,
						Template:  template,
					},
				},
			},
		}
	}

	createDictEntryDefineMsg := func(id uint64, value string) *message.StatefulMessage {
		msg := message.NewMessage([]byte(""), nil, "", 0)
		msg.MessageMetadata.RawDataLen = 0
		return &message.StatefulMessage{
			Metadata: &msg.MessageMetadata,
			Datum: &statefulpb.Datum{
				Data: &statefulpb.Datum_DictEntryDefine{
					DictEntryDefine: &statefulpb.DictEntryDefine{
						Id:    id,
						Value: value,
					},
				},
			},
		}
	}

	createPatternDeleteMsg := func(id uint64) *message.StatefulMessage {
		msg := message.NewMessage([]byte(""), nil, "", 0)
		msg.MessageMetadata.RawDataLen = 0
		return &message.StatefulMessage{
			Metadata: &msg.MessageMetadata,
			Datum: &statefulpb.Datum{
				Data: &statefulpb.Datum_PatternDelete{
					PatternDelete: &statefulpb.PatternDelete{
						PatternId: id,
					},
				},
			},
		}
	}

	createDictEntryDeleteMsg := func(id uint64) *message.StatefulMessage {
		msg := message.NewMessage([]byte(""), nil, "", 0)
		msg.MessageMetadata.RawDataLen = 0
		return &message.StatefulMessage{
			Metadata: &msg.MessageMetadata,
			Datum: &statefulpb.Datum{
				Data: &statefulpb.Datum_DictEntryDelete{
					DictEntryDelete: &statefulpb.DictEntryDelete{
						Id: id,
					},
				},
			},
		}
	}

	createLogMsg := func(content string) *message.StatefulMessage {
		msg := message.NewMessage([]byte(content), nil, "", 0)
		msg.MessageMetadata.RawDataLen = len(content)
		return &message.StatefulMessage{
			Metadata: &msg.MessageMetadata,
			Datum: &statefulpb.Datum{
				Data: &statefulpb.Datum_Logs{
					Logs: &statefulpb.Log{
						Timestamp: 12345,
						Content: &statefulpb.Log_Raw{
							Raw: content,
						},
					},
				},
			},
		}
	}

	// Send all 14 events in sequence
	// Batch 1 (5 entries): add p1, add d1, log, add p2, add d2
	input <- createPatternDefineMsg(1, "pattern1")
	input <- createDictEntryDefineMsg(1, "value1")
	input <- createLogMsg("log with p1/d1")
	input <- createPatternDefineMsg(2, "pattern2")
	input <- createDictEntryDefineMsg(2, "value2")

	// Advance clock to trigger timer-based flush for batch 1
	clk.Add(2 * timerInterval)

	// Receive and verify Batch 1
	payload1 := <-output
	require.Equal(t, 5, len(payload1.MessageMetas), "Batch 1 should have 5 messages")

	// Verify StatefulExtra for Batch 1
	require.NotNil(t, payload1.StatefulExtra, "Batch 1 should have StatefulExtra")
	extra1, ok := payload1.StatefulExtra.(*StatefulExtra)
	require.True(t, ok, "StatefulExtra should be of type *StatefulExtra")
	require.Equal(t, 4, len(extra1.StateChanges), "Batch 1 should have 4 state changes")

	// Check specific state changes in Batch 1
	assert.Equal(t, uint64(1), extra1.StateChanges[0].GetPatternDefine().PatternId)
	assert.Equal(t, "pattern1", extra1.StateChanges[0].GetPatternDefine().Template)
	assert.Equal(t, uint64(1), extra1.StateChanges[1].GetDictEntryDefine().Id)
	assert.Equal(t, "value1", extra1.StateChanges[1].GetDictEntryDefine().Value)
	assert.Equal(t, uint64(2), extra1.StateChanges[2].GetPatternDefine().PatternId)
	assert.Equal(t, "pattern2", extra1.StateChanges[2].GetPatternDefine().Template)
	assert.Equal(t, uint64(2), extra1.StateChanges[3].GetDictEntryDefine().Id)
	assert.Equal(t, "value2", extra1.StateChanges[3].GetDictEntryDefine().Value)

	// Batch 2 (6 entries): log, del p1, del d1, add p3, add d3, log
	input <- createLogMsg("log with p2/d2")
	input <- createPatternDeleteMsg(1)
	input <- createDictEntryDeleteMsg(1)
	input <- createPatternDefineMsg(3, "pattern3")
	input <- createDictEntryDefineMsg(3, "value3")
	input <- createLogMsg("log with p3/d3")

	// Advance clock to trigger timer-based flush for batch 2
	clk.Add(2 * timerInterval)

	// Receive and verify Batch 2
	payload2 := <-output
	require.Equal(t, 6, len(payload2.MessageMetas), "Batch 2 should have 6 messages")

	// Verify StatefulExtra for Batch 2
	require.NotNil(t, payload2.StatefulExtra, "Batch 2 should have StatefulExtra")
	extra2, ok := payload2.StatefulExtra.(*StatefulExtra)
	require.True(t, ok, "StatefulExtra should be of type *StatefulExtra")
	require.Equal(t, 4, len(extra2.StateChanges), "Batch 2 should have 4 state changes")

	// Check specific state changes in Batch 2
	assert.Equal(t, uint64(1), extra2.StateChanges[0].GetPatternDelete().PatternId)
	assert.Equal(t, uint64(1), extra2.StateChanges[1].GetDictEntryDelete().Id)
	assert.Equal(t, uint64(3), extra2.StateChanges[2].GetPatternDefine().PatternId)
	assert.Equal(t, "pattern3", extra2.StateChanges[2].GetPatternDefine().Template)
	assert.Equal(t, uint64(3), extra2.StateChanges[3].GetDictEntryDefine().Id)
	assert.Equal(t, "value3", extra2.StateChanges[3].GetDictEntryDefine().Value)

	// Batch 3 (3 entries): add p4, add d4, log
	input <- createPatternDefineMsg(4, "pattern4")
	input <- createDictEntryDefineMsg(4, "value4")
	input <- createLogMsg("log with p4/d4")

	// Advance clock to trigger timer-based flush for batch 3
	clk.Add(2 * timerInterval)

	// Receive and verify Batch 3
	payload3 := <-output
	require.Equal(t, 3, len(payload3.MessageMetas), "Batch 3 should have 3 messages")

	// Verify StatefulExtra for Batch 3
	require.NotNil(t, payload3.StatefulExtra, "Batch 3 should have StatefulExtra")
	extra3, ok := payload3.StatefulExtra.(*StatefulExtra)
	require.True(t, ok, "StatefulExtra should be of type *StatefulExtra")
	require.Equal(t, 2, len(extra3.StateChanges), "Batch 3 should have 2 state changes")

	// Check specific state changes in Batch 3
	assert.Equal(t, uint64(4), extra3.StateChanges[0].GetPatternDefine().PatternId)
	assert.Equal(t, "pattern4", extra3.StateChanges[0].GetPatternDefine().Template)
	assert.Equal(t, uint64(4), extra3.StateChanges[1].GetDictEntryDefine().Id)
	assert.Equal(t, "value4", extra3.StateChanges[1].GetDictEntryDefine().Value)

	strategy.Stop()
}
