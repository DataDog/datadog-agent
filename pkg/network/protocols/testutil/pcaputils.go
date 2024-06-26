// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package testutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// GetShortTestName generates a suffix in the form
// "<protocol>-<subtest name>" to be passed to WithPCAP.
func GetShortTestName(proto, subtest string) string {
	subtest = strings.ReplaceAll(subtest, " ", "_")
	return fmt.Sprintf("%s-%s", proto, subtest)
}

// WithPCAP runs tcpdump for the duration of the test.
//
// It returns an `io.Writer` to be used as a KeyLogWriter in
// tls.Config to be able, to decrypt the TLS traffic in the resulting
// PCAP. The resulting PCAPs and keylog files will be saved in
// `/tmp/test_pcaps`
//
// Unless alwaysSave is true, WithPCAP will only save the resulting
// PCAP if the test fails.
func WithPCAP(t *testing.T, port string, suffix string, alwaysSave bool) io.Writer {
	t.Helper()

	tmpDest := os.TempDir()

	// Ensure destination directory exists
	if _, err := os.Stat(tmpDest); os.IsNotExist(err) {
		require.NoError(t, os.Mkdir(tmpDest, 0755))
	} else {
		require.NoError(t, err)
	}

	pcapFile := fmt.Sprintf("test-%s.pcap", suffix)
	pcapTempPath := filepath.Join(t.TempDir(), pcapFile)

	klwFile := fmt.Sprintf("test-%s.keylog", suffix)
	klwTempPath := filepath.Join(t.TempDir(), klwFile)

	klw, err := os.Create(klwTempPath)
	require.NoError(t, err, "could not create keylog writer")

	tcpdumpCmd := exec.Command("tcpdump", "-i", "any", "-w", pcapTempPath, "port", port)
	stderr, err := tcpdumpCmd.StderrPipe()
	require.NoError(t, err, "could not get tcpdump stderr pipe")
	require.NoError(t, tcpdumpCmd.Start())

	t.Cleanup(func() {
		require.NoError(t, tcpdumpCmd.Process.Signal(os.Interrupt), "could not send signal to tcpdump")
		out, err := io.ReadAll(stderr)
		require.NoError(t, err, "could not read stderr")
		require.NoError(t, tcpdumpCmd.Wait(), "error during tcpdump: "+string(out))
		klw.Close()

		if !alwaysSave && !t.Failed() {
			return
		}

		require.NoError(t, os.Rename(pcapTempPath, filepath.Join(tmpDest, pcapFile)))
		require.NoError(t, os.Rename(klwTempPath, filepath.Join(tmpDest, klwFile)))
	})

	return klw
}
