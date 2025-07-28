// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package cuda

import (
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/prometheus/procfs"

	"github.com/stretchr/testify/require"
)

// AddKernelCacheProcMap adds a proc map to the kernel cache. Useful for testing, to stop
// the kernel cache from trying to inspect the (fake) PID maps and causing a failure
func AddKernelCacheProcMap(kc *KernelCache, pid int, entries []*procfs.ProcMap) {
	kc.pidMaps[pid] = entries
}

// AddKernelCacheEntry adds a kernel to the kernel cache. Useful for testing, to stop
func AddKernelCacheEntry(t *testing.T, kc *KernelCache, pid int, address uint64, smVersion uint32, binPath string, kernel *CubinKernel) {
	startAddr := uintptr(0)
	if address > 1000 {
		startAddr = uintptr(address - 1000)
	}
	endAddr := uintptr(address + 1000)

	AddKernelCacheProcMap(kc, pid, []*procfs.ProcMap{
		{StartAddr: startAddr, EndAddr: endAddr, Offset: 0, Pathname: binPath},
	})

	fatbin := NewFatbin()
	kernKey := CubinKernelKey{Name: kernel.Name, SmVersion: smVersion}
	fatbin.AddKernel(kernKey, kernel)

	procBinPath := path.Join(kc.procRoot, fmt.Sprintf("%d/root/%s", pid, binPath))
	procBinIdent, err := buildSymbolFileIdentifier(procBinPath)
	require.NoError(t, err)

	kc.cudaSymbols[procBinIdent] = &symbolsEntry{
		Symbols: &Symbols{
			SymbolTable: map[uint64]string{address: kernel.Name},
			Fatbin:      fatbin,
		},
	}
}

// WaitForKernelCacheEntry waits for a kernel cache entry to be added to the kernel cache.
func WaitForKernelCacheEntry(t *testing.T, kc *KernelCache, pid int, address uint64, smVersion uint32) {
	cacheKey := kernelKey{
		pid:       int(pid),
		address:   address,
		smVersion: smVersion,
	}
	require.Eventually(t, func() bool {
		return kc.fromCache(cacheKey) != nil
	}, 10000*time.Millisecond, 10*time.Millisecond)

	// Ensure the kernel has been loaded correctly
	require.NoError(t, kc.cache[cacheKey].err)
	require.NotNil(t, kc.cache[cacheKey].kernel)
}
