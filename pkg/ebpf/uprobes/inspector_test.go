// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func TestNativeBinarySymbolRetrieval(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libmmap, "libssl.so."+runtime.GOARCH)
	fpath := utils.FilePath{HostPath: lib}

	setID := 0
	allMandatoryExisting := map[int][]SymbolRequest{setID: {{Name: "SSL_connect"}}}
	allBestEffortExisting := map[int][]SymbolRequest{setID: {{Name: "SSL_connect", BestEffort: true}}}
	mandatoryExistBestEffortDont := map[int][]SymbolRequest{setID: {{Name: "SSL_connect"}, {Name: "ThisFunctionDoesNotExistEver", BestEffort: true}}}
	mandatoryNonExisting := map[int][]SymbolRequest{setID: {{Name: "ThisFunctionDoesNotExistEver"}}}

	inspector := &NativeBinaryInspector{}

	t.Run("MandatoryAllExist", func(tt *testing.T) {
		result, err := inspector.Inspect(fpath, allMandatoryExisting)
		require.NoError(tt, err)
		require.Contains(tt, result, setID)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result[setID].SymbolMap))
		require.NoError(tt, result[setID].Error)
	})

	t.Run("BestEffortAllExist", func(tt *testing.T) {
		result, err := inspector.Inspect(fpath, allBestEffortExisting)
		require.NoError(tt, err)
		require.Contains(tt, result, setID)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result[setID].SymbolMap))
		require.NoError(tt, result[setID].Error)
	})

	t.Run("BestEffortDontExist", func(tt *testing.T) {
		result, err := inspector.Inspect(fpath, mandatoryExistBestEffortDont)
		require.NoError(tt, err)
		require.Contains(tt, result, setID)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result[setID].SymbolMap))
		require.NoError(tt, result[setID].Error)
	})

	t.Run("SomeMandatoryDontExist", func(tt *testing.T) {
		result, err := inspector.Inspect(fpath, mandatoryNonExisting)
		require.NoError(tt, err) // no global error here as we're not looking for mandatory symbols
		require.Contains(tt, result, setID)
		require.Error(tt, result[setID].Error, "should have failed to find mandatory symbols")
	})
}
