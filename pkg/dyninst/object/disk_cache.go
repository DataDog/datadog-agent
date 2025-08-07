// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"

	"github.com/dustin/go-humanize"

	"github.com/DataDog/datadog-agent/pkg/dyninst/htlhash"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiskCacheConfig is the configuration for a DiskCache.
type DiskCacheConfig struct {
	// DirPath is the directory to store cached sections.
	DirPath string
	// RequiredDiskSpaceBytes is the minimum number of bytes that must remain
	// available on the underlying filesystem *after* writing a decompressed
	// section. This is enforced in addition to the percentage-based limit.
	//
	// Set to 0 to disable the bytes-based limit.
	RequiredDiskSpaceBytes uint64
	// RequiredDiskSpacePercent is the minimum percentage of total disk capacity
	// that must remain free *after* writing a decompressed section. This is
	// enforced in addition to the bytes-based limit.
	//
	// Set to 0 to disable the percentage-based limit.
	RequiredDiskSpacePercent float64
	// MaxTotalBytes is the maximum aggregate size of all sections that can be
	// cached, in bytes.
	MaxTotalBytes uint64
}

func (cfg *DiskCacheConfig) validate() error {
	if cfg.DirPath == "" {
		return fmt.Errorf("dirPath must not be empty")
	}
	if cfg.RequiredDiskSpacePercent < 0 || cfg.RequiredDiskSpacePercent > 100 {
		return fmt.Errorf(
			"requiredDiskPercent must be between 0 and 100, got %g",
			cfg.RequiredDiskSpacePercent,
		)
	}
	if cfg.MaxTotalBytes == 0 {
		return fmt.Errorf("maxTotalBytes must not be zero")
	}
	return nil
}

// DiskCache enables loading object files and storing decompressed sections on
// disk.
type DiskCache struct {
	dirPath string
	checker spaceChecker

	// maxTotalBytes is the maximum aggregate size of all sections currently
	// held in the cache (open or in-flight).
	maxTotalBytes uint64

	mu struct {
		sync.Mutex

		// The total number of bytes of all sections currently held in the cache
		// (open or in-flight).
		totalBytes uint64
		entries    map[cacheKey]*cacheEntry
	}
}

// NewDiskCache creates a new disk cache rooted at dirPath.
func NewDiskCache(
	cfg DiskCacheConfig,
) (*DiskCache, error) {
	return newDiskCache(cfg, newDirectoryDiskUsageReader(cfg.DirPath))
}

func newDiskCache(cfg DiskCacheConfig, diskUsageReader diskUsageReader) (*DiskCache, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DirPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	c := &DiskCache{
		dirPath:       cfg.DirPath,
		maxTotalBytes: cfg.MaxTotalBytes,
		checker: spaceChecker{
			disk:                     diskUsageReader,
			requiredDiskSpaceBytes:   cfg.RequiredDiskSpaceBytes,
			requiredDiskSpacePercent: cfg.RequiredDiskSpacePercent,
		},
	}
	c.mu.entries = make(map[cacheKey]*cacheEntry)
	return c, nil
}

// Load loads the object file at path and returns an ElfFile. The underlying
// compressed sections made available by the DebugSections are backed by files
// in the cache's directory. The files are tracked by the cache and are
// automatically removed when the resulting ElfFile is closed and no other
// references to the compressed sections exist.
func (c *DiskCache) Load(path string) (*ElfFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	htlHash, err := htlhash.Compute(f)
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			err = fmt.Errorf("%w: (failed to close file: %w)", err, closeErr)
		}
		return nil, err
	}
	mef, err := newMMappingElfFile(f)
	if err != nil {
		return nil, err
	}
	ef, err := newElfObject(mef, &htlHashLoader{htlHash: htlHash, c: c})
	if err != nil {
		return nil, err
	}

	return ef, nil
}

// SpaceInUse returns the total size of all cached sections in bytes.
func (c *DiskCache) SpaceInUse() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mu.totalBytes
}

// htlHashLoader is a sectionDataLoader that loads sections based on the executable's
// htl hash.
type htlHashLoader struct {
	htlHash htlhash.Hash
	c       *DiskCache
}

func (h *htlHashLoader) load(
	cr compressedSection, mef *MMappingElfFile,
) (SectionData, error) {
	return h.c.loadSection(h.htlHash, cr, mef)
}

// getSection returns a SectionData for the requested compressedFileRange. It
// either returns a cached section or performs the decompression, ensuring only
// one goroutine performs the decompression work at a time.
func (c *DiskCache) loadSection(
	htlHash htlhash.Hash,
	cr compressedSection,
	mef *MMappingElfFile,
) (_ SectionData, retErr error) {
	key := cacheKey{
		htlHash:           htlHash,
		compressedSection: cr,
	}
	entry, err := c.getOrCreateEntry(key)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			cleanupCacheEntry(entry)
		}
	}()

	// Ensure decompression (only first goroutine will actually perform work).
	entry.decompress.Do(func() {
		outputPath := filepath.Join(c.dirPath, key.String())
		entry.decompress.data, entry.decompress.err = decompressToDisk(
			&c.checker, outputPath, cr.compressedFileRange, mef,
		)
	})
	if entry.decompress.err != nil {
		return nil, entry.decompress.err
	}
	sd := new(cachedSectionData)
	sd.entry = entry
	sd.cleanup = runtime.AddCleanup(sd, cleanupCacheEntry, entry)
	return sd, nil
}

// getOrCreateEntry returns the cache entry for the given key, creating it if
// it does not exist.
func (c *DiskCache) getOrCreateEntry(key cacheKey) (*cacheEntry, error) {
	// Note that the complexity in this function stems from the goal of
	// avoiding holding a mutex across io operations. It's possible that
	// the corresponding entry for the key is in the process of being deleted,
	// in which case we need to wait for the deletion to complete before
	// creating a new entry.
	getOrCreateEntry := func() (*cacheEntry, <-chan struct{}, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, exists := c.mu.entries[key]
		if !exists {
			length := uint64(key.uncompressedLength)
			newTotal := c.mu.totalBytes + length
			if newTotal > c.maxTotalBytes {
				return nil, nil, fmt.Errorf(
					"adding %s would exceed cache size limit of %s by %s",
					humanize.IBytes(length),
					humanize.IBytes(c.maxTotalBytes),
					humanize.IBytes(newTotal-c.maxTotalBytes),
				)
			}
			c.mu.totalBytes = newTotal
			entry = &cacheEntry{
				cacheKey: key,
				cache:    c,
				deleted:  make(chan struct{}),
			}
			c.mu.entries[key] = entry
		}
		if entry.cacheMu.deleting {
			return nil, entry.deleted, nil
		}
		entry.cacheMu.refCount++
		return entry, nil, nil
	}
	for {
		entry, deleted, err := getOrCreateEntry()
		if err != nil {
			return nil, err
		}
		if entry != nil {
			return entry, nil
		}
		<-deleted // wait for entry to be deleted
	}
}

// decompressToDisk performs the actual decompression and writes the
// uncompressed data to the outputPath.
func decompressToDisk(
	checker *spaceChecker,
	outputPath string,
	cr compressedFileRange,
	mef *MMappingElfFile,
) (_ []byte, retErr error) {
	if err := checker.check(uint64(cr.uncompressedLength)); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(outputPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close() // in case of a failure, we'll clean up the file
	// Early remove the file to avoid leaking it to the system if we crash or
	// are killed.
	if err := os.Remove(outputPath); err != nil {
		return nil, fmt.Errorf("failed to remove decompressed data file: %w", err)
	}
	if cr.format != compressionFormatZlib {
		return nil, fmt.Errorf("unsupported compression format: %d", cr.format)
	}
	md, zrd, err := mmapCompressedSection(cr, mef)
	if err != nil {
		return nil, err
	}
	defer md.Close()
	n, err := io.Copy(f, zrd)
	if err != nil {
		_ = zrd.Close()
		return nil, fmt.Errorf("failed to write decompressed data: %w", err)
	}
	if err := zrd.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zlib reader: %w", err)
	}
	if n != int64(cr.uncompressedLength) {
		return nil, fmt.Errorf(
			"decompressed section size mismatch: %d != %d",
			n, cr.uncompressedLength,
		)
	}
	if err := f.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync temp file: %w", err)
	}
	m, err := syscall.Mmap(int(f.Fd()), 0, int(n), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap cached section: %w", err)
	}
	defer func() {
		if retErr == nil {
			return
		}
		if munmapErr := syscall.Munmap(m); munmapErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf(
				"failed to munmap decompressed data: %w", munmapErr,
			))
		}
	}()
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("failed to close decompressed data file: %w", err)
	}
	return m, nil
}

// cachedSectionData is a SectionData implementation backed by a file on disk
// managed by DecompressedSectionDiskCache.
type cachedSectionData struct {
	entry   *cacheEntry
	cleanup runtime.Cleanup
}

// Data returns the decompressed data for the section.
//
// It will panic if the section is closed.
func (c *cachedSectionData) Data() []byte { return c.entry.decompress.data }

// Close closes the section.
func (c *cachedSectionData) Close() error {
	if c.entry == nil {
		return nil
	}
	c.cleanup.Stop()
	err := c.entry.release()
	c.entry = nil
	return err
}

type spaceChecker struct {
	// The filesystem must have at least this many bytes free *after* placing
	// a section on disk.
	requiredDiskSpaceBytes uint64
	// requiredDiskSpacePercent is the minimum percentage (0–100) of total disk
	// capacity that must remain free *after* placing a section on disk.
	requiredDiskSpacePercent float64

	disk diskUsageReader
}

func (s *spaceChecker) check(toAdd uint64) error {
	du, err := s.disk.ReadDiskUsage()
	if err != nil {
		return fmt.Errorf("failed to query disk space: %w", err)
	}
	// Ensure we have enough space to write the section (and avoid underflow).
	if du.Available < toAdd {
		return fmt.Errorf(
			"insufficient disk space: need %s, have only %s (%s short)",
			humanize.IBytes(toAdd),
			humanize.IBytes(du.Available),
			humanize.IBytes(toAdd-du.Available),
		)
	}
	availAfter := du.Available - toAdd
	// Make sure we do not violate the policy based on an absolute amount.
	{
		minAvailBytes := s.requiredDiskSpaceBytes
		if minAvailBytes > 0 && availAfter < minAvailBytes {
			return fmt.Errorf(
				"disk space limits reached: need %s free, after write of %s "+
					"will have only %s (%s short)",
				humanize.IBytes(minAvailBytes),
				humanize.IBytes(toAdd),
				humanize.IBytes(availAfter),
				humanize.IBytes(minAvailBytes-availAfter),
			)
		}
	}
	// Make sure we do not violate the policy based on a percentage.
	minAvailFromPercent := uint64(math.Ceil(float64(du.Total) * (s.requiredDiskSpacePercent / 100)))
	if availAfter < minAvailFromPercent {
		return fmt.Errorf(
			"disk space limits reached: need %s free (%s%% of %v), after "+
				"write of %s will have %s (%s short)",
			humanize.IBytes(minAvailFromPercent),
			strconv.FormatFloat(s.requiredDiskSpacePercent, 'f', -1, 64),
			humanize.IBytes(du.Total),
			humanize.IBytes(toAdd),
			humanize.IBytes(availAfter),
			humanize.IBytes(minAvailFromPercent-availAfter),
		)
	}
	return nil
}

type cacheEntry struct {
	cacheKey
	cache   *DiskCache
	cacheMu struct { // fields accessed only under c.mu
		refCount int
		deleting bool // set to true when the entry is being deleted
	}
	decompress struct { // synchronizes decompression
		sync.Once
		data []byte
		err  error
	}
	deleted chan struct{} // closed when the entry is being deleted
}

func cleanupCacheEntry(e *cacheEntry) {
	if err := e.release(); err != nil {
		log.Errorf("failed to cleanup cache entry: %v", err)
	}
}

// release decrements the reference count for the entry, and if the entry is
// no longer referenced, it removes the entry from the cache.
func (e *cacheEntry) release() error {
	// Note that complexity here is mirrored in getOrCreateEntry: we avoid
	// holding a mutex across io operations.
	decrementRefCount := func() (shouldDelete bool, err error) {
		e.cache.mu.Lock()
		defer e.cache.mu.Unlock()
		if e.cacheMu.deleting {
			return false, fmt.Errorf("release: entry %s is being deleted", e.cacheKey)
		}
		if e.cacheMu.refCount <= 0 {
			return false, fmt.Errorf(
				"release: entry %s would have negative ref count %d",
				e.cacheKey, e.cacheMu.refCount,
			)
		}
		e.cacheMu.refCount--
		e.cacheMu.deleting = e.cacheMu.refCount == 0
		return e.cacheMu.deleting, nil
	}

	shouldDelete, err := decrementRefCount()
	if err != nil {
		log.Errorf("release invariant violation: %v", err)
		return err
	}
	if !shouldDelete {
		return nil
	}
	defer close(e.deleted)
	defer func() {
		e.cache.mu.Lock()
		defer e.cache.mu.Unlock()
		delete(e.cache.mu.entries, e.cacheKey)
		length := uint64(e.cacheKey.uncompressedLength)
		// Pedantically check for underflow.
		if length > e.cache.mu.totalBytes {
			log.Errorf(
				"release: invariant violation: total size underflow: %s > %s (delta: %s)",
				humanize.IBytes(length),
				humanize.IBytes(e.cache.mu.totalBytes),
				humanize.IBytes(length-e.cache.mu.totalBytes),
			)
			e.cache.mu.totalBytes = 0
		} else {
			e.cache.mu.totalBytes -= length
		}
	}()
	var munmapErr error
	if err := syscall.Munmap(e.decompress.data); err != nil {
		munmapErr = fmt.Errorf("failed to munmap decompressed data: %w", err)
	}
	e.decompress.data = nil
	return munmapErr
}

type cacheKey struct {
	htlHash htlhash.Hash
	compressedSection
}

func (c cacheKey) String() string {
	return fmt.Sprintf(
		"%s:%s:%d-%d",
		c.htlHash, c.name, c.offset, c.offset+c.compressedLength,
	)
}

// diskUsageReader reads the available disk space on the filesystem.
type diskUsageReader interface {
	// ReadDiskUsage reads the available disk space on the filesystem.
	ReadDiskUsage() (filesystem.DiskUsage, error)
}

// directoryDiskUsageReader reads the available disk space on the filesystem
// from a directory.
type directoryDiskUsageReader struct {
	dirPath string
	disk    filesystem.Disk
}

func newDirectoryDiskUsageReader(dirPath string) *directoryDiskUsageReader {
	return &directoryDiskUsageReader{
		dirPath: dirPath,
		disk:    filesystem.NewDisk(),
	}
}

// ReadDiskUsage reads the available disk space on the filesystem from a
// directory.
func (r *directoryDiskUsageReader) ReadDiskUsage() (filesystem.DiskUsage, error) {
	du, err := r.disk.GetUsage(r.dirPath)
	if err != nil {
		return filesystem.DiskUsage{}, err
	}
	return *du, nil
}
