// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestTraceLevelLogWriter_Write(t *testing.T) {
	tests := []struct {
		name         string
		logLine      []byte
		assertLen    bool
		expectedLogs string
		expectedLen  int
	}{
		{
			name:         "log line",
			logLine:      []byte(`SEND INIT NEGOTIATE SECURITY PARAMS`),
			expectedLogs: "[TRACE] Write: SEND INIT NEGOTIATE SECURITY PARAMS",
			assertLen:    true,
			expectedLen:  len("SEND INIT NEGOTIATE SECURITY PARAMS"),
		},
		{
			name:         "scrub SECURITY PARAMETERS",
			logLine:      []byte(`SECURITY PARAMETERS:&{mu:{state:1 sema:0} localAESSalt:0 localDESSalt:3998515827 AuthoritativeEngineID:<80>^@O<B8>^E11fba59a65bc^@^A<8C><F8> AuthoritativeEngineBoots:2 AuthoritativeEngineTime:549 UserName:datadogSHADES AuthenticationParameters: PrivacyParameters:[0 0 0 2 238 84 130 117] AuthenticationProtocol:SHA PrivacyProtocol:DES AuthenticationPassphrase:doggiepass PrivacyPassphrase:doggiePRIVkey SecretKey:[28 100 233 65 80 138 148 44 105 136 172 255 141 53 176 170 203 101 70 154] PrivacyKey:[169 188 43 128 146 34 23 114 59 181 206 241 3 227 175 9 102 110 230 20] Logger:0xc00004e190}`),
			expectedLogs: "[TRACE] Write: SECURITY PARAMETERS: ********",
		},
		{
			name:         "scrub Parsed privacyParameters",
			logLine:      []byte("Parsed privacyParameters \x01\x02abc"),
			expectedLogs: "[TRACE] Write: Parsed privacyParameters ********",
		},
		{
			name:         "scrub Parsed contextEngineID",
			logLine:      []byte("Parsed contextEngineID \x01\x02abc"),
			expectedLogs: "[TRACE] Write: Parsed contextEngineID ********",
		},
		{
			name:         "scrub AuthenticationPassphrase",
			logLine:      []byte(`TEST: AuthenticationPassphrase:doggiepass`),
			expectedLogs: "[TRACE] Write: TEST: AuthenticationPassphrase: ********",
		},
		{
			name:         "scrub PrivacyPassphrase",
			logLine:      []byte(`TEST: PrivacyPassphrase:doggiepass`),
			expectedLogs: "[TRACE] Write: TEST: PrivacyPassphrase: ********",
		},
		{
			name:         "scrub SecretKey",
			logLine:      []byte(`TEST: SecretKey:doggiepass`),
			expectedLogs: "[TRACE] Write: TEST: SecretKey: ********",
		},
		{
			name:         "scrub PrivacyKey",
			logLine:      []byte(`TEST: PrivacyKey:doggiepass`),
			expectedLogs: "[TRACE] Write: TEST: PrivacyKey: ********",
		},
		{
			name:         "scrub community no quote",
			logLine:      []byte(`TEST: ContextName:cisco-nexus Community:abcd PDUType:162`),
			expectedLogs: "[TRACE] Write: TEST: ContextName:cisco-nexus Community:******** PDUType:162",
		},
		{
			name:         "scrub community quote",
			logLine:      []byte(`TEST: ContextName:"cisco-nexus", Community:"abcd", PDUType:0xa5`),
			expectedLogs: "[TRACE] Write: TEST: ContextName:\"cisco-nexus\", Community:******** PDUType:0xa5",
		},
		{
			name:         "scrub ContextEngineID no quote",
			logLine:      []byte("TEST: SecurityParameters:0xc0009f4820 ContextEngineID:\x80\x00O\xb8\x0511fba59a65bc\x00\x01,\xf8 ContextName:cisco-nexus"),
			expectedLogs: "[TRACE] Write: TEST: SecurityParameters:0xc0009f4820 ContextEngineID:******** ContextName:cisco-nexus",
		},
		{
			name:         "scrub ContextEngineID quote",
			logLine:      []byte(`TEST: SecurityParameters:(*gosnmp.UsmSecurityParameters)(0xc0004b6c30), ContextEngineID:"\x80\x00O\xb8\x0511fba59a65bc\x00\x01,\xf8", ContextName:"cisco-nexus"`),
			expectedLogs: "[TRACE] Write: TEST: SecurityParameters:(*gosnmp.UsmSecurityParameters)(0xc0004b6c30), ContextEngineID:******** ContextName:\"cisco-nexus\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
			require.NoError(t, err)
			log.SetupLogger(l, "trace")

			sw := &TraceLevelLogWriter{}
			lineLen, err := sw.Write(tt.logLine)
			assert.NoError(t, err)

			if tt.assertLen {
				assert.Equal(t, tt.expectedLen, lineLen)
			}

			w.Flush()
			logs := b.String()

			assert.Contains(t, logs, tt.expectedLogs)
		})
	}
}
