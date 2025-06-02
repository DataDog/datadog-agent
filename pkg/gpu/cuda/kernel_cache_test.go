// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package cuda

import (
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/procfs"

	"github.com/stretchr/testify/require"

	cudatestutil "github.com/DataDog/datadog-agent/pkg/gpu/cuda/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestBuildSymbolFileIdentifier(t *testing.T) {
	// Create a file, then a symlink to it
	// and check that the identifier is the same
	// for both files.
	dir := t.TempDir()
	filePath := dir + "/file"
	copyPath := dir + "/copy"
	differentPath := dir + "/different"
	symlinkPath := dir + "/symlink"

	data := []byte("hello")
	// create the original file
	err := os.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	// create a symlink to the original file, which should have the same identifier
	err = os.Symlink(filePath, symlinkPath)
	require.NoError(t, err)

	// a copy is a different inode, so it should have a different identifier
	// even with the same size
	err = os.WriteFile(copyPath, data, 0644)
	require.NoError(t, err)

	// a different file with different content should have a different identifier
	// as it's different content and different inode
	err = os.WriteFile(differentPath, []byte("different"), 0644)
	require.NoError(t, err)

	origIdentifier, err := buildSymbolFileIdentifier(filePath)
	require.NoError(t, err)

	symlinkIdentifier, err := buildSymbolFileIdentifier(symlinkPath)
	require.NoError(t, err)

	copyIdentifier, err := buildSymbolFileIdentifier(copyPath)
	require.NoError(t, err)

	differentIdentifier, err := buildSymbolFileIdentifier(differentPath)
	require.NoError(t, err)

	require.Equal(t, origIdentifier, symlinkIdentifier)
	require.NotEqual(t, origIdentifier, copyIdentifier)
	require.NotEqual(t, origIdentifier, differentIdentifier)
	require.NotEqual(t, copyIdentifier, differentIdentifier)
}

func TestKernelCacheLoadKernelData(t *testing.T) {
	// Get sample CUDA binary path
	samplePath := cudatestutil.GetCudaSample(t, "sample")

	// Create a temporary directory that acts as a process root
	// and create the samplePath inside it to replicate the expected process tree
	pid := 1234
	procRoot := t.TempDir()
	procRootPath := path.Join(procRoot, strconv.Itoa(pid), "root")

	procRootSamplePath := path.Join(procRootPath, samplePath)
	err := os.MkdirAll(path.Dir(procRootSamplePath), 0755)
	require.NoError(t, err)

	// symlink samplePath to procRootSamplePath
	err = os.Symlink(samplePath, procRootSamplePath)
	require.NoError(t, err)

	// sanity check that the sample has a kernel for the SM we are going to parse
	require.Contains(t, cudatestutil.SampleSMVersions, testutil.DefaultSMVersion, "default SM version should be in sample file")

	// Because we're dealing with real files, we need to parse and get the symbol we want
	// to load from the fatbin file so we can use it in the test
	elffile, err := safeelf.Open(samplePath)
	require.NoError(t, err)

	syms, err := elffile.Symbols()
	require.NoError(t, err)

	kernelName := "_Z7kernel1Pfi" // kernel1(float*, int)
	var kernelAddress uint64
	for _, sym := range syms {
		if sym.Name == kernelName {
			kernelAddress = sym.Value
			break
		}
	}
	require.NotZero(t, kernelAddress, "kernel address should be found")

	kc, err := NewKernelCache(procRoot, cudatestutil.SamplesSMVersionSet(), testutil.GetTelemetryMock(t), 100)
	require.NoError(t, err)
	kc.Start()
	t.Cleanup(kc.Stop)

	// build the key we will request
	// caches are indexed by the address in the process, so we need to convert the address
	// to the address in the process
	baseAddr := uint64(0x1000)
	inProcessAddr := kernelAddress + baseAddr
	key := kernelKey{
		pid:       pid,
		address:   inProcessAddr,
		smVersion: testutil.DefaultSMVersion,
	}

	// Add memory map entry for the test
	procMap := &procfs.ProcMap{
		StartAddr: uintptr(baseAddr),
		EndAddr:   uintptr(inProcessAddr + 100), // Ensure the kernel is in the map
		Offset:    0,
		Pathname:  samplePath,
	}
	kc.pidMaps[key.pid] = []*procfs.ProcMap{procMap}

	// First call should return not processed yet
	kernel, err := kc.Get(key.pid, key.address, key.smVersion)
	require.ErrorIs(t, err, ErrKernelNotProcessedYet)
	require.Nil(t, kernel)

	// Wait for background processing
	require.Eventually(t, func() bool {
		return kc.fromCache(key) != nil
	}, 2000*time.Millisecond, 100*time.Millisecond)

	// Second call should return the kernel
	kernel, err = kc.Get(key.pid, key.address, key.smVersion)
	require.NoError(t, err)
	require.NotNil(t, kernel)
	require.Equal(t, kernelName, kernel.Name)
	require.Equal(t, uint64(0), kernel.SharedMem)

	// Verify cache entry exists
	kc.cacheMutex.RLock()
	cachedData, exists := kc.cache[key]
	kc.cacheMutex.RUnlock()
	require.True(t, exists)
	require.Equal(t, kernel, cachedData.kernel)
	require.NoError(t, cachedData.err)

	// Cleanup should remove the entry
	kc.CleanProcessData(key.pid)
	kc.cacheMutex.RLock()
	_, exists = kc.cache[key]
	kc.cacheMutex.RUnlock()
	require.False(t, exists)
}

func TestKernelCacheLoadKernelDataError(t *testing.T) {
	kc, err := NewKernelCache(kernel.ProcFSRoot(), cudatestutil.SamplesSMVersionSet(), testutil.GetTelemetryMock(t), 100)
	require.NoError(t, err)
	kc.Start()
	t.Cleanup(kc.Stop)

	// Load kernel data with invalid path
	key := kernelKey{
		pid:       1234,
		address:   0x1000,
		smVersion: testutil.DefaultSMVersion,
	}

	// Add memory map entry with invalid path
	kc.pidMaps[key.pid] = []*procfs.ProcMap{
		{
			StartAddr: 0x1000,
			EndAddr:   0x2000,
			Offset:    0,
			Pathname:  "/nonexistent/path",
		},
	}

	// First call should return not processed yet
	kernel, err := kc.Get(key.pid, key.address, key.smVersion)
	require.ErrorIs(t, err, ErrKernelNotProcessedYet)
	require.Nil(t, kernel)

	// Wait for background processing
	time.Sleep(100 * time.Millisecond)

	// Second call should return the error
	kernel, err = kc.Get(key.pid, key.address, key.smVersion)
	require.Error(t, err)
	require.Nil(t, kernel)

	// Verify cache entry exists with error
	kc.cacheMutex.RLock()
	cachedData, exists := kc.cache[key]
	kc.cacheMutex.RUnlock()
	require.True(t, exists)
	require.Nil(t, cachedData.kernel)
	require.Error(t, cachedData.err)
}
