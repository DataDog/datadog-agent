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
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/stretchr/testify/assert"
)

var testProfile = &NCMProfile{
	BaseProfile: BaseProfile{
		Name: "test",
	},
	Commands: map[CommandType]Commands{
		Running: {
			Values: []string{"show running-config"},
			ProcessingRules: ProcessingRules{
				metadataRules: []MetadataRule{
					{
						Type:   Timestamp,
						Regex:  `Last configuration change at (.+)`,
						Format: "15:04:05 MST Mon Jan 2 2006",
					},
					{
						Type:  ConfigSize,
						Regex: `Current configuration : (?P<Size>\d+)`,
					},
				},
				validationRules: []ValidationRule{
					{
						Pattern: "Building configuration...",
					},
				},
				redactionRules: []RedactionRule{
					{Type: SensitiveData, Regex: `(username .+ (password|secret) \d) .+`},
				},
			},
		},
	},
	Scrubber: scrubber.New(),
}

var example = `
Building configuration...


Current configuration : 3144 bytes
!
! Last configuration change at 20:53:27 UTC Thu Aug 14 2025
!
version 15.9
service timestamps debug datetime msec
service timestamps log datetime msec
no service password-encryption
!
hostname qa-device
!
boot-start-marker
boot-end-marker
!
ip domain name lab.local
ip cef    
no ipv6 cef
!         
multilink bundle-name authenticated
!         
!         
!         
!         
username cisco privilege 15 secret 9 $9$BMUEX2PiO0KhAv$N7lS6KlzzGds54nvZM5zmpPuLrKr9CZC3A1/jTwjHzA
!         
redundancy
!
`

var expected = `
Building configuration...


Current configuration : 3144 bytes
!
! Last configuration change at 20:53:27 UTC Thu Aug 14 2025
!
version 15.9
service timestamps debug datetime msec
service timestamps log datetime msec
no service password-encryption
!
hostname qa-device
!
boot-start-marker
boot-end-marker
!
ip domain name lab.local
ip cef    
no ipv6 cef
!         
multilink bundle-name authenticated
!         
!         
!         
!         
username cisco privilege 15 secret 9 "********"
!         
redundancy
!`

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
		{name: "extracting timestamp, config size success",
			profile:     testProfile,
			commandType: Running,
			configBytes: []byte(expected),
			expected: &ExtractedMetadata{
				timestamp:  1755204807,
				configSize: 3144,
			},
		},
		{
			name:        "extracting metadata failure",
			profile:     testProfile,
			commandType: Running,
			configBytes: []byte("huh"),
			expected:    &ExtractedMetadata{},
			expectedLogMsgs: []string{
				"could not parse timestamp for profile test",
				"could not parse config size for profile test",
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
		name        string
		profile     *NCMProfile
		commandType CommandType
		configBytes []byte
		expected    error
	}{
		{
			name:        "valid output",
			profile:     testProfile,
			commandType: Running,
			configBytes: []byte(example),
			expected:    nil,
		},
		{
			name:        "invalid output - no metadata found for the command type",
			profile:     testProfile,
			commandType: Startup,
			configBytes: []byte(example),
			expected:    fmt.Errorf("no metadata found for command type startup in profile test"),
		},
		{
			name:        "invalid output - rule violation",
			profile:     testProfile,
			commandType: Running,
			configBytes: []byte("example"),
			expected:    fmt.Errorf("invalid output (due to rule requiring: Building configuration...) for command type running in profile test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.validateOutput(tt.commandType, tt.configBytes)
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
		commandType    CommandType
		configBytes    []byte
		expected       []byte
		expectedErrMsg string
	}{
		{
			name:        "redacts config with rule set",
			profile:     testProfile,
			commandType: Running,
			configBytes: []byte(example),
			expected:    []byte(expected),
		},
		{
			name:           "cannot redact config if no rules set",
			profile:        testProfile,
			commandType:    Startup,
			configBytes:    []byte(example),
			expected:       []byte{},
			expectedErrMsg: "no metadata found for command type startup in profile test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := tt.profile.applyRedactions(tt.commandType, tt.configBytes)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}
