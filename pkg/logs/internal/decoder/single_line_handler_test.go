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
		name     string
		input    []string
		expected []string
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
