// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package cuda

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func TestLazySectionReader(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libdir := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libdir, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))

	f, err := os.Open(lib)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	// Read using the regular ELF reader first
	elfFile, err := elf.NewFile(f)
	require.NoError(t, err)
	t.Cleanup(func() { elfFile.Close() })

	sectsByIndex := make(map[int]*elf.Section)
	for i, sect := range elfFile.Sections {
		sectsByIndex[i] = sect
	}

	// Read using the lazy reader now
	reader := newLazySectionReader(f)

	i := 0 // canot use the enumerator index as this is a range-over iterator, not a regular slice
	for sect := range reader.Iterate() {
		require.Greater(t, len(sectsByIndex), i)

		origSect := sectsByIndex[i]
		require.Equal(t, origSect.Offset, sect.Offset, "Offset mismatch in section number %d", i)
		require.Equal(t, origSect.Size, sect.Size, "Size mismatch in section number %d", i)
		require.Equal(t, origSect.Name, sect.Name(), "Name mismatch in section number %d", i)
		i++
	}
}
