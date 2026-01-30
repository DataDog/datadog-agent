// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package decoder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestSingleLineHandlerProcess(t *testing.T) {
	mockConfig := configmock.New(t)

	truncateTag := message.TruncatedReasonTag("single_line")
	tagTrunLogsFlag := mockConfig.GetBool("logs_config.tag_truncated_logs")
	defer mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", tagTrunLogsFlag)

	scenarios := []struct {
		name             string
		input            []string
		expected         []string
		expTags          [][]string
		tagTruncatedLogs bool
	}{
		{
			name:             "Base case",
			input:            []string{"This is a", "normal set  ", " of messages"},
			expected:         []string{"This is a", "normal set", "of messages"},
			expTags:          [][]string{nil, nil, nil},
			tagTruncatedLogs: true,
		},
		{
			name:  "Truncation base case",
			input: []string{"aaaaaaaaaaaaaaaaaaaaa", "aaaaaaaa", "wait, how many a's?"},
			expected: []string{
				"aaaaaaaaaaaaaaaaaaaaa" + string(message.TruncatedFlag),
				string(message.TruncatedFlag) + "aaaaaaaa",
				"wait, how many a's?",
			},
			expTags:          [][]string{{truncateTag}, {truncateTag}, nil},
			tagTruncatedLogs: true,
		},
		{
			name:  "Truncation with whitespace",
			input: []string{"aaaaaaaaaaaaaaaaa    ", "  aaaaaa", "wait, how many a's?"},
			expected: []string{
				"aaaaaaaaaaaaaaaaa" + string(message.TruncatedFlag),
				string(message.TruncatedFlag) + "aaaaaa",
				"wait, how many a's?",
			},
			expTags:          [][]string{{truncateTag}, {truncateTag}, nil},
			tagTruncatedLogs: true,
		},
		{
			name:  "Triple trunc",
			input: []string{"aaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaa", "wait, how many a's?"},
			expected: []string{
				"aaaaaaaaaaaaaaaaaaaaa" + string(message.TruncatedFlag),
				string(message.TruncatedFlag) + "aaaaaaaaaaaaaaaaaaaaa" + string(message.TruncatedFlag),
				string(message.TruncatedFlag) + "wait, how many a's?",
			},
			expTags:          [][]string{{truncateTag}, {truncateTag}, {truncateTag}},
			tagTruncatedLogs: true,
		},
		{
			name:  "Truncate tag disabled",
			input: []string{"aaaaaaaaaaaaaaaaaaaaa", "aaaaaaaa", "wait, how many a's?"},
			expected: []string{
				"aaaaaaaaaaaaaaaaaaaaa" + string(message.TruncatedFlag),
				string(message.TruncatedFlag) + "aaaaaaaa",
				"wait, how many a's?",
			},
			expTags:          [][]string{nil, nil, nil},
			tagTruncatedLogs: false,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var processedMessage *message.Message
			mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", scenario.tagTruncatedLogs)
			h := NewSingleLineHandler(func(m *message.Message) { processedMessage = m }, 20)

			for idx, input := range scenario.input {
				msg := message.NewMessage([]byte(input), nil, "", time.Now().UnixNano())
				h.process(msg)
				assert.Equal(t, []byte(scenario.expected[idx]), processedMessage.GetContent(), "Unexpected message content for run %d", idx)
				assert.Equal(t, scenario.expTags[idx], processedMessage.ParsingExtra.Tags, "Unexpected tag content for run %d", idx)
			}
		})
	}
}

func TestSingleLineHandlerExactLimit(t *testing.T) {
	scenarios := []struct {
		name           string
		input          []string
		expected       []string
		expIsTruncated []bool
	}{
		{
			name:           "Exactly at limit should not truncate",
			input:          []string{"aaaaaaaaaaaaaaaaaaaa", "next log"},
			expected:       []string{"aaaaaaaaaaaaaaaaaaaa", "next log"},
			expIsTruncated: []bool{false, false},
		},
		{
			name:  "One byte over limit should truncate",
			input: []string{"aaaaaaaaaaaaaaaaaaaaa", "remainder"},
			expected: []string{
				"aaaaaaaaaaaaaaaaaaaaa" + string(message.TruncatedFlag),
				string(message.TruncatedFlag) + "remainder",
			},
			expIsTruncated: []bool{true, true},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var processedMessage *message.Message
			h := NewSingleLineHandler(func(m *message.Message) { processedMessage = m }, 20)

			for idx, input := range scenario.input {
				msg := message.NewMessage([]byte(input), nil, "", time.Now().UnixNano())
				// For the "one byte over" case, the framer would have already set IsTruncated
				if len(input) > 20 {
					msg.ParsingExtra.IsTruncated = true
				}
				h.process(msg)
				assert.Equal(t, []byte(scenario.expected[idx]), processedMessage.GetContent(), "Unexpected message content for run %d", idx)
				assert.Equal(t, scenario.expIsTruncated[idx], processedMessage.ParsingExtra.IsTruncated, "Unexpected IsTruncated flag for run %d", idx)
			}
		})
	}
}

func TestSingleLineHandlerFramerIntegration(t *testing.T) {
	mockConfig := configmock.New(t)
	truncateTag := message.TruncatedReasonTag("single_line")
	tagTrunLogsFlag := mockConfig.GetBool("logs_config.tag_truncated_logs")
	defer mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", tagTrunLogsFlag)
	mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", true)

	t.Run("Frame 2 should get tag even though framer sets IsTruncated=false", func(t *testing.T) {
		// This simulates what the framer actually does for a 21-byte log (one over limit of 20):
		// Frame 1: 20 bytes (forced break), IsTruncated=true
		// Frame 2: 1 byte (newline found), IsTruncated=false
		// Frame 3: "next log" (new log line), IsTruncated=false

		var processedMessages []*message.Message
		h := NewSingleLineHandler(func(m *message.Message) {
			processedMessages = append(processedMessages, m)
		}, 20)

		// Frame 1: 20 'a's (forced break by framer)
		msg1 := message.NewMessage([]byte("aaaaaaaaaaaaaaaaaaaa"), nil, "", time.Now().UnixNano())
		msg1.ParsingExtra.IsTruncated = true // Framer sets this for forced break
		h.process(msg1)

		// Frame 2: 1 'a' (newline found by framer)
		msg2 := message.NewMessage([]byte("a"), nil, "", time.Now().UnixNano())
		msg2.ParsingExtra.IsTruncated = false // Framer sets FALSE because newline was found
		h.process(msg2)

		// Frame 3: "next log" (separate log line)
		msg3 := message.NewMessage([]byte("next log"), nil, "", time.Now().UnixNano())
		msg3.ParsingExtra.IsTruncated = false
		h.process(msg3)

		// Verify Frame 1
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaa"+string(message.TruncatedFlag), string(processedMessages[0].GetContent()))
		assert.True(t, processedMessages[0].ParsingExtra.IsTruncated)
		assert.Equal(t, []string{truncateTag}, processedMessages[0].ParsingExtra.Tags, "Frame 1 should have truncation tag")

		// Verify Frame 2 - CRITICAL: should have tag even though framer set IsTruncated=false
		assert.Equal(t, string(message.TruncatedFlag)+"a", string(processedMessages[1].GetContent()))
		assert.True(t, processedMessages[1].ParsingExtra.IsTruncated)
		assert.Equal(t, []string{truncateTag}, processedMessages[1].ParsingExtra.Tags, "Frame 2 should have truncation tag via handler state machine")

		// Verify Frame 3 - should NOT have markers or tag
		assert.Equal(t, "next log", string(processedMessages[2].GetContent()))
		assert.False(t, processedMessages[2].ParsingExtra.IsTruncated)
		assert.Nil(t, processedMessages[2].ParsingExtra.Tags, "Frame 3 (separate log) should NOT have truncation tag")
	})

	t.Run("Multiple continuation frames should all get tags", func(t *testing.T) {
		// This tests a 61-byte log (limit 20) which splits into 4 frames:
		// Frame 1: 20 bytes (forced break), IsTruncated=true
		// Frame 2: 20 bytes (forced break), IsTruncated=true
		// Frame 3: 20 bytes (forced break), IsTruncated=true
		// Frame 4: 1 byte (newline found), IsTruncated=false
		// Frame 5: "next log" (new log line), IsTruncated=false

		var processedMessages []*message.Message
		h := NewSingleLineHandler(func(m *message.Message) {
			processedMessages = append(processedMessages, m)
		}, 20)

		// Frame 1
		msg1 := message.NewMessage([]byte("aaaaaaaaaaaaaaaaaaaa"), nil, "", time.Now().UnixNano())
		msg1.ParsingExtra.IsTruncated = true
		h.process(msg1)

		// Frame 2
		msg2 := message.NewMessage([]byte("aaaaaaaaaaaaaaaaaaaa"), nil, "", time.Now().UnixNano())
		msg2.ParsingExtra.IsTruncated = true
		h.process(msg2)

		// Frame 3
		msg3 := message.NewMessage([]byte("aaaaaaaaaaaaaaaaaaaa"), nil, "", time.Now().UnixNano())
		msg3.ParsingExtra.IsTruncated = true
		h.process(msg3)

		// Frame 4 (last frame of truncated log)
		msg4 := message.NewMessage([]byte("a"), nil, "", time.Now().UnixNano())
		msg4.ParsingExtra.IsTruncated = false // Framer sets false because newline found
		h.process(msg4)

		// Frame 5 (separate log)
		msg5 := message.NewMessage([]byte("next log"), nil, "", time.Now().UnixNano())
		msg5.ParsingExtra.IsTruncated = false
		h.process(msg5)

		// Verify all frames of truncated log have tag
		assert.Equal(t, []string{truncateTag}, processedMessages[0].ParsingExtra.Tags)
		assert.Equal(t, []string{truncateTag}, processedMessages[1].ParsingExtra.Tags)
		assert.Equal(t, []string{truncateTag}, processedMessages[2].ParsingExtra.Tags)
		assert.Equal(t, []string{truncateTag}, processedMessages[3].ParsingExtra.Tags, "Frame 4 should have tag via state machine")

		// Verify separate log does NOT have tag
		assert.Nil(t, processedMessages[4].ParsingExtra.Tags, "Frame 5 (separate log) should NOT have tag")
	})
}
