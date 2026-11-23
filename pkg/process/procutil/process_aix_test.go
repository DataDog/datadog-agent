// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package procutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func makePsinfo(fname, psargs string) *psinfo {
	psi := &psinfo{}
	copy(psi.Fname[:], fname)
	copy(psi.Psargs[:], psargs)
	return psi
}

func TestPsinfoToProcessASCIIArgs(t *testing.T) {
	proc := psinfoToProcess(makePsinfo("rmcd", "/opt/rsct/bin/rmcd -a IBM.LPCommands"), 42)

	require.Equal(t, "rmcd", proc.Name)
	require.Equal(t, "rmcd", proc.Comm)
	require.Equal(t, []string{"/opt/rsct/bin/rmcd", "-a", "IBM.LPCommands"}, proc.Cmdline)
}

func TestPsinfoToProcessNonASCIIArgs(t *testing.T) {
	// Readable argv can contain valid UTF-8 text; it must not be treated as
	// junk and replaced by the bracket fallback.
	proc := psinfoToProcess(makePsinfo("python", "python worker.py --label café"), 42)

	require.Equal(t, []string{"python", "worker.py", "--label", "café"}, proc.Cmdline)
}

func TestPsinfoToProcessUnreadableArgs(t *testing.T) {
	// pr_psargs holds arbitrary binary bytes when the kernel cannot read a
	// process's argv; the cmdline must fall back to the bracketed name.
	junk := []byte{0x89, 0xa3, 0xe7, 'h', 'e', 'l', 'l', 'o', 0x1b, '[', 0x00}
	psi := makePsinfo("IBM.Softdird", "")
	copy(psi.Psargs[:], junk)

	proc := psinfoToProcess(psi, 42)

	require.Equal(t, "IBM.Softdird", proc.Name)
	require.Equal(t, "IBM.Softdird", proc.Comm)
	require.Equal(t, []string{"[IBM.Softdird]"}, proc.Cmdline)
}

func TestPsinfoToProcessEmptyNameAndArgs(t *testing.T) {
	// e.g. the swapper (pid 0): nothing to fall back to.
	proc := psinfoToProcess(makePsinfo("", ""), 0)

	require.Empty(t, proc.Name)
	require.Empty(t, proc.Comm)
	require.Empty(t, proc.Cmdline)
}
