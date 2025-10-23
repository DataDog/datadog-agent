// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
)

func Test_extractMetadata(t *testing.T) {
	tests := []struct {
		name            string
		profile         *NCMProfile
		compileRules    bool
		commandType     CommandType
		metadataRules   []MetadataRule
		configBytes     []byte
		expected        *ExtractedMetadata
		expectedErrMsg  string
		expectedLogMsgs []string
	}{
		// TODO: consolidate testing variables for ease of testing one thing at a time
		{
			name:         "extracting timestamp, config size success",
			profile:      newTestProfile(),
			compileRules: true,
			commandType:  Running,
			configBytes:  []byte(exampleConfig),
			expected: &ExtractedMetadata{
				Timestamp:  1755204807,
				ConfigSize: 3144,
			},
		},
		{
			name:         "extracting metadata failure",
			profile:      newTestProfile(),
			compileRules: false,
			commandType:  Running,
			configBytes:  []byte("huh"),
			expected:     &ExtractedMetadata{},
			expectedLogMsgs: []string{
				`profile "test" does not have a regexp for metadata rule ! Last configuration change at (.*)`,
				`profile "test" does not have a regexp for metadata rule Current configuration : (?P<Size>\d+)`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %FuncShort: %Msg\n")
			assert.NoError(t, err)
			log.SetupLogger(l, "debug")

			if tt.compileRules {
				_ = tt.profile.compileProcessingRules()
			}
			actual, _ := tt.profile.extractMetadata(tt.commandType, tt.configBytes)
			w.Flush()

			if len(tt.expectedLogMsgs) > 0 {
				logOutput := b.String()
				for _, msg := range tt.expectedLogMsgs {
					fmt.Println(logOutput)
					fmt.Println(msg)
					fmt.Print(strings.Contains(logOutput, msg))
					assert.True(t, strings.Contains(logOutput, msg))
				}
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_validateOutput(t *testing.T) {
	tests := []struct {
		name         string
		profile      *NCMProfile
		compileRules bool
		commandType  CommandType
		configBytes  []byte
		expected     error
	}{
		{
			name:         "valid output",
			profile:      newTestProfile(),
			compileRules: true,
			commandType:  Running,
			configBytes:  []byte(exampleConfig),
			expected:     nil,
		},
		{
			name:         "invalid output - no metadata found for the command type",
			profile:      newTestProfile(),
			compileRules: false,
			commandType:  Startup,
			configBytes:  []byte(exampleConfig),
			expected:     fmt.Errorf("no metadata found for command type startup in profile test"),
		},
		{
			name:         "invalid output - rule violation",
			profile:      newTestProfile(),
			compileRules: false,
			commandType:  Running,
			configBytes:  []byte("example"),
			expected:     fmt.Errorf(`profile "test" does not have a regexp for validation rule: Building configuration... `),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.compileRules {
				_ = tt.profile.compileProcessingRules()
			}
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
				_ = tt.profile.compileProcessingRules()
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
			err := cmd.compileProcessingRules()
			if tt.expected != nil {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, cmd)
		})
	}
}
