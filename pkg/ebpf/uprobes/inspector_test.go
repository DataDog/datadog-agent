// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func TestNativeBinarySymbolRetrieval(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))
	fpath := utils.FilePath{HostPath: lib}

	allMandatoryExisting := []SymbolRequest{{Name: "SSL_connect"}}
	allBestEffortExisting := []SymbolRequest{{Name: "SSL_connect", BestEffort: true}}
	mandatoryExistBestEffortDont := []SymbolRequest{{Name: "SSL_connect"}, {Name: "ThisFunctionDoesNotExistEver", BestEffort: true}}
	mandatoryNonExisting := []SymbolRequest{{Name: "ThisFunctionDoesNotExistEver"}}

	inspector := &NativeBinaryInspector{}

	t.Run("MandatoryAllExist", func(tt *testing.T) {
		result, compat, err := inspector.Inspect(fpath, allMandatoryExisting)
		require.NoError(tt, err)
		require.True(tt, compat)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result))
	})

	t.Run("BestEffortAllExist", func(tt *testing.T) {
		result, compat, err := inspector.Inspect(fpath, allBestEffortExisting)
		require.NoError(tt, err)
		require.True(tt, compat)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result))
	})

	t.Run("BestEffortDontExist", func(tt *testing.T) {
		result, compat, err := inspector.Inspect(fpath, mandatoryExistBestEffortDont)
		require.NoError(tt, err)
		require.True(tt, compat)
		require.ElementsMatch(tt, []string{"SSL_connect"}, maps.Keys(result))
	})

	t.Run("SomeMandatoryDontExist", func(tt *testing.T) {
		_, _, err := inspector.Inspect(fpath, mandatoryNonExisting)
		require.Error(tt, err, "should have failed to find mandatory symbols")
	})
}
