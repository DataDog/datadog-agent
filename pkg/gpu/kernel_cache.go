// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const fatbinFileCacheCleanupInterval = 5 * time.Minute

// kernelKey is a key to identify a kernel in the cache.
type kernelKey struct {
	pid       int
	address   uint64
	smVersion uint32
}

// kernelData is a structure that contains the kernel data and the error that occurred when loading it.
type kernelData struct {
	kernel *cuda.CubinKernel
	err    error
}

// symbolFileIdentifier is a key to identify a symbol file in the cache. Uses inode and file size to identify the file,
// as the file might be mmapped in different locations. Size is used to identify if the file has changed.
type symbolFileIdentifier struct {
	inode    int
	fileSize int64
}

// symbolsEntry wraps the cuda.Symbols and adds a last used time to the file to be able to clean up old files.
type symbolsEntry struct {
	*cuda.Symbols
	lastUsedTime time.Time
}

// updateLastUsedTime updates the last used time of the symbols entry.
func (e *symbolsEntry) updateLastUsedTime() {
	e.lastUsedTime = time.Now()
}

// kernelCache caches fatbin kernels and handles background loading of missing kernels
type kernelCache struct {
	// cache is a map of kernel key to its kernel data
	cache map[kernelKey]*kernelData

	// cacheMutex is a mutex to protect the cache
	cacheMutex sync.RWMutex

	// requests is a channel of kernel keys to be loaded in the background
	requests chan kernelKey

	// pidsToDelete is a channel of PIDs to delete from the cache. This will get processed in the processRequests goroutine
	// to ensure that only the processRequests touches the internal caches, to avoid race issues
	pidsToDelete chan int

	// cudaSymbols is a map of symbol file identifier to its symbols entry. Only accessed in the processRequests goroutine, so we don't need to lock it
	cudaSymbols map[symbolFileIdentifier]*symbolsEntry

	// pidMaps is a map of process ID to its memory maps. Only accessed in the processRequests goroutine, so we don't need to lock it
	pidMaps map[int][]*procfs.ProcMap

	// done is a channel of done signals for the processRequests goroutine
	done chan struct{}

	sysCtx    *systemContext
	telemetry *kernelCacheTelemetry
}

type kernelCacheTelemetry struct {
	symbolCacheSize telemetry.Gauge
	kernelCacheSize telemetry.Gauge
	readErrors      telemetry.Counter
	fatbinPayloads  telemetry.Counter
	kernelsPerFile  telemetry.Histogram
	kernelSizes     telemetry.Histogram
	activePIDs      telemetry.Gauge
}

func newKernelCacheTelemetry(tm telemetry.Component) *kernelCacheTelemetry {
	subsystem := gpuTelemetryModule + "__kernel_cache"

	return &kernelCacheTelemetry{
		symbolCacheSize: tm.NewGauge(subsystem, "symbol_cache_size", nil, "Number of CUDA symbols in the cache"),
		kernelCacheSize: tm.NewGauge(subsystem, "kernel_cache_size", nil, "Number of kernels in the cache"),
		readErrors:      tm.NewCounter(subsystem, "read_errors", nil, "Number of errors reading fatbin data"),
		fatbinPayloads:  tm.NewCounter(subsystem, "fatbin_payloads", []string{"compression"}, "Number of fatbin payloads read"),
		kernelsPerFile:  tm.NewHistogram(subsystem, "kernels_per_file", nil, "Number of kernels per fatbin file", []float64{5, 10, 50, 100, 500}),
		kernelSizes:     tm.NewHistogram(subsystem, "kernel_sizes", nil, "Size of kernels in bytes", []float64{100, 1000, 10000, 100000, 1000000, 10000000}),
		activePIDs:      tm.NewGauge(subsystem, "active_pids", nil, "Number of PIDs in the kernel cache"),
	}
}

// newKernelCache creates a new kernel cache with background processing
func newKernelCache(sysCtx *systemContext, tm telemetry.Component) *kernelCache {
	kc := &kernelCache{
		cache:        make(map[kernelKey]*kernelData),
		cudaSymbols:  make(map[symbolFileIdentifier]*symbolsEntry),
		pidMaps:      make(map[int][]*procfs.ProcMap),
		requests:     make(chan kernelKey, 100),
		pidsToDelete: make(chan int, 100),
		sysCtx:       sysCtx,
		telemetry:    newKernelCacheTelemetry(tm),
		done:         make(chan struct{}),
	}
	return kc
}

// Start starts the kernel cache background processing goroutine.
func (kc *kernelCache) Start() {
	go kc.processRequests()
}

// Stop stops the kernel cache background processing goroutine.
func (kc *kernelCache) Stop() {
	close(kc.done)
}

func buildSymbolFileIdentifier(path string) (symbolFileIdentifier, error) {
	stat, err := utils.UnixStat(path)
	if err != nil {
		return symbolFileIdentifier{}, fmt.Errorf("error getting file info: %w", err)
	}

	return symbolFileIdentifier{inode: int(stat.Ino), fileSize: stat.Size}, nil
}

// getCudaSymbols gets the CUDA symbols for a given path. Uses an internal cache to avoid reading the file multiple times.
// Uses internal caches, so should only be called in the processRequests goroutine, to avoid race issues.
func (kc *kernelCache) getCudaSymbols(path string) (*symbolsEntry, error) {
	fileIdent, err := buildSymbolFileIdentifier(path)
	if err != nil {
		return nil, fmt.Errorf("error building symbol file identifier: %w", err)
	}

	if data, ok := kc.cudaSymbols[fileIdent]; ok {
		data.updateLastUsedTime()
		return data, nil
	}

	data, err := cuda.GetSymbols(path, kc.sysCtx.smVersionSet)
	if err != nil {
		kc.telemetry.readErrors.Inc()
		return nil, fmt.Errorf("error getting file data: %w", err)
	}

	kc.telemetry.fatbinPayloads.Add(float64(data.Fatbin.CompressedPayloads), "compressed")
	kc.telemetry.fatbinPayloads.Add(float64(data.Fatbin.UncompressedPayloads), "uncompressed")
	kc.telemetry.kernelsPerFile.Observe(float64(data.Fatbin.NumKernels()))

	for kernel := range data.Fatbin.GetKernels() {
		kc.telemetry.kernelSizes.Observe(float64(kernel.KernelSize))
	}

	wrapper := &symbolsEntry{Symbols: data}
	wrapper.updateLastUsedTime()
	kc.cudaSymbols[fileIdent] = wrapper

	kc.telemetry.symbolCacheSize.Set(float64(len(kc.cudaSymbols)))

	return wrapper, nil
}

// errKernelNotProcessedYet is returned when the kernel is not processed yet, so it's not available in the cache
// but a request has been made to load it in the background.
var errKernelNotProcessedYet = errors.New("kernel not processed yet")

// GetKernelData attempts to get kernel data from cache or triggers background loading. If the kernel is not found, it returns nil.
// This function can return errKernelNotProcessedYet if the kernel is not processed yet, so it's not available in the cache
// but a request has been made to load it in the background. In that case, the caller should retry later.
func (kc *kernelCache) GetKernelData(pid int, addr uint64, smVersion uint32) (*cuda.CubinKernel, error) {
	key := kernelKey{pid: pid, address: addr, smVersion: smVersion}

	// Try to get from cache first
	data := kc.getExistingKernelData(key)
	if data != nil {
		return data.kernel, data.err
	}

	// Not in cache, trigger background load
	select {
	case kc.requests <- key:
		return nil, errKernelNotProcessedYet
	default:
		return nil, fmt.Errorf("kernel cache request channel full, cannot queue request for pid=%d addr=0x%x", pid, addr)
	}
}

// getExistingKernelData returns the kernel data for a given key if it exists.
func (kc *kernelCache) getExistingKernelData(key kernelKey) *kernelData {
	kc.cacheMutex.RLock()
	defer kc.cacheMutex.RUnlock()

	return kc.cache[key]
}

// loadKernelData loads the kernel data for a given key. This function uses some internal caches
// for symbols and CUDA files, so it should only be called in the processRequests goroutine, to avoid race issues.
func (kc *kernelCache) loadKernelData(key kernelKey) (*cuda.CubinKernel, error) {
	maps, err := kc.getProcessMemoryMaps(key.pid)
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	entry := findEntryInMaps(maps, uintptr(key.address))
	if entry == nil {
		return nil, fmt.Errorf("could not find memory maps entry for kernel address 0x%x", key.address)
	}

	offsetInFile := uint64(int64(key.address) - int64(entry.StartAddr) + entry.Offset)
	binaryPath := path.Join(kc.sysCtx.procRoot, strconv.Itoa(key.pid), "root", entry.Pathname)

	fileData, err := kc.getCudaSymbols(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("error getting file %s data: %w", binaryPath, err)
	}

	symbol, ok := fileData.SymbolTable[offsetInFile]
	if !ok {
		return nil, fmt.Errorf("could not find symbol for address 0x%x in file %s", key.address, binaryPath)
	}

	kern := fileData.Fatbin.GetKernel(symbol, key.smVersion)
	if kern == nil {
		return nil, fmt.Errorf("could not find kernel for symbol %s in file %s", symbol, binaryPath)
	}

	return kern, nil
}

// processRequests is a goroutine that processes the kernel requests in the background. It's also responsible for
// receiving PIDs to delete from the cache, and cleaning up the cache when the process exits. This ensures the internal
// caches are only accessed in the processRequests goroutine, to avoid race issues.
func (kc *kernelCache) processRequests() {
	fatbinCleanup := time.NewTicker(fatbinFileCacheCleanupInterval)
	defer fatbinCleanup.Stop()

	for {
		select {
		case key := <-kc.requests:
			// Load kernel data
			if kc.getExistingKernelData(key) != nil {
				// Kernel already loaded, skip
				// This can happen if we have received multiple requests for the same kernel
				// while we were processing the request.
				continue
			}

			kernel, err := kc.loadKernelData(key)

			// Update or store in cache
			kc.cacheMutex.Lock()
			kc.cache[key] = &kernelData{kernel: kernel, err: err}
			kc.telemetry.kernelCacheSize.Set(float64(len(kc.cache)))
			kc.cacheMutex.Unlock()
		case pid := <-kc.pidsToDelete:
			delete(kc.pidMaps, pid)
		case <-fatbinCleanup.C:
			kc.cleanupOldEntries()
		case <-kc.done:
			return
		}
	}
}

// getProcessMemoryMaps gets the memory maps for a process.
func (kc *kernelCache) getProcessMemoryMaps(pid int) ([]*procfs.ProcMap, error) {
	// pidMaps only gets accessed in the processRequests goroutine, so we don't need to lock it
	maps, ok := kc.pidMaps[pid]
	if ok {
		return maps, nil
	}

	proc, err := kc.sysCtx.procfsObj.Proc(pid)
	if err != nil {
		return nil, fmt.Errorf("error opening process %d: %w", pid, err)
	}

	maps, err = proc.ProcMaps()
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	kc.pidMaps[pid] = maps
	kc.telemetry.activePIDs.Set(float64(len(kc.pidMaps)))

	return maps, nil
}

func (kc *kernelCache) cleanDataForPid(pid int) {
	kc.cacheMutex.Lock()
	defer kc.cacheMutex.Unlock()

	for key := range kc.cache {
		if key.pid == pid {
			delete(kc.cache, key)
		}
	}

	kc.pidsToDelete <- pid

	kc.telemetry.kernelCacheSize.Set(float64(len(kc.cache)))
	kc.telemetry.activePIDs.Set(float64(len(kc.pidMaps)))
}

// cleanupOldEntries removes any old entries that have not been accessed in a while.
// Should only be called in the processRequests goroutine, to avoid race issues.
func (kc *kernelCache) cleanupOldEntries() {
	maxFatbinAge := 5 * time.Minute
	fatbinExpirationTime := time.Now().Add(-maxFatbinAge)

	for key, data := range kc.cudaSymbols {
		if data.lastUsedTime.Before(fatbinExpirationTime) {
			delete(kc.cudaSymbols, key)
		}
	}

	kc.telemetry.symbolCacheSize.Set(float64(len(kc.cudaSymbols)))
}
