package snmp

import (
	"bufio"
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
			name:         "scrub authenticationParameters",
			logLine:      []byte(`TEST: authenticationParameters abc`),
			expectedLogs: "[TRACE] Write: TEST: authenticationParameters ********",
		},
		{
			name:         "scrub community no quote",
			logLine:      []byte(`TEST: ContextName:cisco-nexus Community:abcd PDUType:162`),
			expectedLogs: "[TRACE] Write: TEST: ContextName:cisco-nexus Community:******** PDUType:162",
		},
		{
			name:         "scrub community quote",
			logLine:      []byte(`TEST: ContextName:"cisco-nexus", Community:"abcd", PDUType:0xa5`),
			expectedLogs: "[TRACE] Write: TEST: ContextName:\"cisco-nexus\", Community:\"********\", PDUType:0xa5",
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

			assert.Equal(t, tt.expectedLogs, logs)
		})
	}
}
