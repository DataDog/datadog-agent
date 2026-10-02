// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadProcNet(t *testing.T) {
	tests := [...]struct {
		input    string
		expected []uint16
	}{
		{
			input: `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0200007F:B600 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 61632 1 ffff88003cc20780 100 0 0 10 0
   1: 00000000:A160 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 16753 1 ffff880034e46d00 100 0 0 10 0
   7: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 16529 1 ffff880034e45e00 100 0 0 10 0
   8: 0F02000A:0016 0202000A:C121 01 00000000:00000000 02:00091FA3 00000000     0        0 20179 3 ffff88003cc20000 20 4 1 10 -1`,
			expected: []uint16{46592, 41312, 22},
		},
		{
			input: `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:ADA0 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 16755 1 ffff88003b34b180 100 0 0 10 0
   6: 00000000000000000000000000000000:B696 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 16511 1 ffff88003b349080 100 0 0 10 0
   7: 00000000000000000000000001000000:EBCE 00000000000000000000000001000000:303A 06 00000000:00000000 03:00000AA4 00000000     0        0 0 3 ffff880035387000
   8: 00000000000000000000000001000000:EBD0 00000000000000000000000001000000:303A 06 00000000:00000000 03:00000B06 00000000     0        0 0 3 ffff880035387118`,
			expected: []uint16{44448, 46742},
		},
	}

	for _, tt := range tests {
		file, err := writeTestFile(tt.input)
		require.NoError(t, err)
		//noinspection GoDeferInLoop
		defer func() { _ = os.Remove(file.Name()) }()

		ports, err := readProcNetListeners(file.Name())
		require.NoError(t, err)

		require.Len(t, ports, len(tt.expected))
		require.ElementsMatch(t, ports, tt.expected)
	}
}

func writeTestFile(content string) (f *os.File, err error) {
	tmpfile, err := os.CreateTemp("", "test-proc-net")

	if err != nil {
		return nil, err
	}

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		return nil, err
	}

	if err := tmpfile.Close(); err != nil {
		return nil, err
	}

	return tmpfile, nil
}
