// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func TestNativeBinarySymbolRetrieval(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "libmmap")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))

	existingSymbols := map[string]struct{}{"SSL_connect": {}}
	nonExistingSymbols := map[string]struct{}{"ThisFunctionDoesNotExistEver": {}}

	inspector := &NativeBinaryInspector{}

	t.Run("MandatoryAllExist", func(tt *testing.T) {
		result, compat, err := inspector.Inspect(lib, existingSymbols, nil)
		require.NoError(tt, err)
		require.True(tt, compat)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result))
	})

	t.Run("BestEffortAllExist", func(tt *testing.T) {
		result, compat, err := inspector.Inspect(lib, nil, existingSymbols)
		require.NoError(tt, err)
		require.True(tt, compat)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result))
	})

	t.Run("BestEffortDontExist", func(tt *testing.T) {
		result, compat, err := inspector.Inspect(lib, existingSymbols, nonExistingSymbols)
		require.NoError(tt, err)
		require.True(tt, compat)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result))
	})

	t.Run("SomeMandatoryDontExist", func(tt *testing.T) {
		_, _, err := inspector.Inspect(lib, nonExistingSymbols, nil)
		require.Error(tt, err, "should have failed to find mandatory symbols")
	})
}
