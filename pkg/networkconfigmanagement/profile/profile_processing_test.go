// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
)

func Test_extractMetadata(t *testing.T) {
	tests := []struct {
		name            string
		profile         *NCMProfile
		commandType     CommandType
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
			commandType: Running,
			configBytes: []byte(exampleConfig),
			expected: &ExtractedMetadata{
				Timestamp:  1755204807,
				ConfigSize: 3144,
			},
		},
		{
			name:        "extracting metadata error logs - cannot parse metadata from bad config",
			profile:     testProfile,
			commandType: Running,
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

			actual, _ := tt.profile.extractMetadata(tt.commandType, tt.configBytes)
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

func Test_validateOutput(t *testing.T) {
	tests := []struct {
		name        string
		profile     *NCMProfile
		commandType CommandType
		configBytes []byte
		expected    error
	}{
		{
			name:        "valid output",
			profile:     newTestProfile(),
			commandType: Running,
			configBytes: []byte(exampleConfig),
			expected:    nil,
		},
		{
			name:        "invalid output - no metadata found for the command type",
			profile:     newTestProfile(),
			commandType: Startup,
			configBytes: []byte(exampleConfig),
			expected:    errors.New("no metadata found for command type startup in profile test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.ValidateOutput(tt.commandType, tt.configBytes)
			if tt.expected != nil {
				assert.Equal(t, tt.expected, err)
			}
		})
	}
}

func Test_applyRedactions(t *testing.T) {
	tests := []struct {
		name           string
		profile        *NCMProfile
		compileRules   bool
		commandType    CommandType
		configBytes    []byte
		expected       []byte
		expectedErrMsg string
	}{
		{
			name:         "redacts config with rule set",
			profile:      newTestProfile(),
			compileRules: true,
			commandType:  Running,
			configBytes:  []byte(exampleConfig),
			expected:     []byte(expectedConfig),
		},
		{
			name:           "cannot redact config if no rules set",
			profile:        newTestProfile(),
			compileRules:   false,
			commandType:    Startup,
			configBytes:    []byte(exampleConfig),
			expected:       []byte{},
			expectedErrMsg: "no metadata found for command type startup in profile test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.compileRules {
				tt.profile.initializeScrubbers()
			}
			actual, err := tt.profile.applyRedactions(tt.commandType, tt.configBytes)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_compileProcessingRules(t *testing.T) {
	tests := []struct {
		name        string
		profile     *NCMProfile
		commandType CommandType
		expected    *Commands
	}{
		{
			name:        "compile processing rules",
			profile:     testProfile,
			commandType: Running,
			expected:    runningCommandsWithCompiledRegex,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.profile.Commands[tt.commandType]
			cmd.initializeScrubber()
			assert.Equal(t, tt.expected, cmd)
		})
	}
}
