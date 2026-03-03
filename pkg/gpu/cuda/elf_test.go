// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Only _test file is tagged for linux, as it causes problems when running in windows builds. This is a
// temporary fix as part of #incident-35081. This should be removed once the issue is resolved.

package cuda

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestLazySectionReader(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libdir := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libdir, "libssl.so."+runtime.GOARCH)

	f, err := os.Open(lib)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	// Read using the regular ELF reader first
	elfFile, err := safeelf.NewFile(f)
	require.NoError(t, err)
	t.Cleanup(func() { elfFile.Close() })

	// Read using the lazy reader now
	reader := newLazySectionReader(f)

	i := 0 // canot use the enumerator index as this is a range-over iterator, not a regular slice
	for sect := range reader.Iterate() {
		require.Greater(t, len(elfFile.Sections), i)

		origSect := elfFile.Sections[i]
		require.Equal(t, origSect.Offset, sect.Offset, "Offset mismatch in section number %d", i)
		require.Equal(t, origSect.Size, sect.Size, "Size mismatch in section number %d", i)
		require.Equal(t, origSect.Name, sect.Name(), "Name mismatch in section number %d", i)
		i++
	}

	require.Equal(t, len(elfFile.Sections), i, "Mismatch in number of sections")
}
