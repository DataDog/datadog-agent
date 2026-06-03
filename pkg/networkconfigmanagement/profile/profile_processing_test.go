// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package profile

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func Test_extractMetadata(t *testing.T) {
	tests := []struct {
		name            string
		profile         *NCMProfile
		metadataRules   []MetadataRule
		configBytes     []byte
		expected        *ExtractedMetadata
		expectedErrMsg  string
		expectedLogMsgs []string
	}{
		// TODO: consolidate testing variables for ease of testing one thing at a time
		{
			name:        "extracting timestamp, config size success",
			profile:     newTestProfile(),
			configBytes: []byte(exampleConfig),
			expected: &ExtractedMetadata{
				Timestamp:  1755204807,
				ConfigSize: 3144,
			},
		},
		{
			name:        "extracting metadata error logs - cannot parse metadata from bad config",
			profile:     newTestProfile(),
			configBytes: []byte("huh"),
			expected:    &ExtractedMetadata{},
			expectedLogMsgs: []string{
				`could not parse timestamp for profile test`,
				`could not parse config size for profile test`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, log.DebugLvl)
			assert.NoError(t, err)
			log.SetupLogger(l, "debug")

			actual, _ := tt.profile.ExtractMetadata(tt.configBytes)
			w.Flush()

			if len(tt.expectedLogMsgs) > 0 {
				logOutput := b.String()
				for _, msg := range tt.expectedLogMsgs {
					assert.True(t, strings.Contains(logOutput, msg))
				}
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_applyRedactions(t *testing.T) {
	tests := []struct {
		name           string
		profile        *NCMProfile
		configBytes    []byte
		expected       []byte
		expectedErrMsg string
	}{
		{
			name:        "redacts config with rule set",
			profile:     newTestProfile(),
			configBytes: []byte(exampleConfig),
			expected:    []byte(expectedConfig),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := Redact(tt.configBytes, tt.profile.Redactions)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}
