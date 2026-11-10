// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestRedirectLoggerLevels(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "warning",
			line:     "WARNING: 2026/11/10 01:22:24 [core] [Channel #1] grpc: addrConn.createTransport failed to connect\n",
			expected: "[WARN] [core] [Channel #1] grpc: addrConn.createTransport failed to connect",
		},
		{
			name:     "transport closing warning",
			line:     "WARNING: 2026/11/10 01:22:24 [core] [Server #1] grpc: Server.processRPC failed to write status: connection error: desc = \"transport is closing\"\n",
			expected: "[DEBUG] [core] [Server #1] grpc: Server.processRPC failed to write status: connection error: desc = \"transport is closing\"",
		},
		{
			name:     "transport closing error",
			line:     "ERROR: 2026/11/10 01:22:24 [core] [Server #1] grpc: transport is closing\n",
			expected: "[ERROR] [core] [Server #1] grpc: transport is closing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logs, log.DebugLvl)
			require.NoError(t, err)
			log.SetupLogger(logger, "debug")
			t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

			_, err = newRedirectLogger().Write([]byte(tt.line))
			require.NoError(t, err)
			log.Flush()

			assert.Equal(t, tt.expected, strings.TrimRight(logs.String(), "\n"))
		})
	}
}
