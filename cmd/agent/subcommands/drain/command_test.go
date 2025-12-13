// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package drain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	// Create a temporary file with sample log lines
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")

	// Write sample log lines to the file
	logContent := `2024-01-01 10:00:00 INFO Application started
2024-01-01 10:00:01 INFO Application started
2024-01-01 10:00:02 INFO Application started
2024-01-01 10:00:03 ERROR Database connection failed
2024-01-01 10:00:04 ERROR Database connection failed
2024-01-01 10:00:05 WARN Memory usage high: 85%
2024-01-01 10:00:06 INFO User logged in: user123
2024-01-01 10:00:07 INFO User logged in: user456
2024-01-01 10:00:08 INFO User logged in: user789
`
	err := os.WriteFile(tmpFile, []byte(logContent), 0644)
	require.NoError(t, err)

	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"drain", tmpFile, "--threshold", "2", "--hide-output"},
		runDrain,
		func(cliParams *CliParams, _ core.BundleParams) {
			require.Equal(t, tmpFile, cliParams.InputFilePath)
			require.Equal(t, 2, cliParams.Threshold)
			require.True(t, cliParams.HideOutput)
		})
}
