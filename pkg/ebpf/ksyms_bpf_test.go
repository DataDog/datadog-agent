// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/require"
)

func TestGetKernelSymbolWtihKallsymsIterator(t *testing.T) {
	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	if kv < kernel.VersionCode(6, 1, 0) {
		t.Skip("BPF kallsyms iterator is not supported")
	}

	kaddrs, err := GetKernelSymbolsAddressesWithKallsymsIterator([]string{"_text", "_stext"}...)
	// only log errors because we are okay with missing some symbols
	if err != nil {
		t.Logf("GetKernelSymbolsAddressesWithKallsymsIterator error: %v", err)
	}

	require.True(t, len(kaddrs) > 0)
}
