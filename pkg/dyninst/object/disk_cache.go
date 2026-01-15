// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall"

	"github.com/dustin/go-humanize"

	"github.com/DataDog/datadog-agent/pkg/dyninst/htlhash"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Loader abstracts the loading of object files.
type Loader interface {
	Load(path string) (FileWithDwarf, error)
}

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
		return errors.New("dirPath must not be empty")
	}
	if cfg.RequiredDiskSpacePercent < 0 || cfg.RequiredDiskSpacePercent > 100 {
		return fmt.Errorf(
			"requiredDiskPercent must be between 0 and 100, got %g",
			cfg.RequiredDiskSpacePercent,
		)
	}
	if cfg.MaxTotalBytes == 0 {
		return errors.New("maxTotalBytes must not be zero")
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
func (c *DiskCache) Load(path string) (FileWithDwarf, error) {
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
	mef, err := makeMMappingElfFile(f)
	if err != nil {
		return nil, err
	}
	ef, err := newElfFileWithDwarf(mef, &htlHashLoader{htlHash: htlHash, c: c})
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

// reserveSpace attempts to reserve the given number of bytes from the cache's
// capacity. The reservation counts against the cache size limit immediately.
//
// The method also enforces the disk space policy via spaceChecker.
func (c *DiskCache) reserveSpace(toAdd uint64) error {
	if toAdd == 0 {
		return nil
	}
	// Avoid holding the mutex across IO operations.
	if err := c.checker.check(toAdd); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	newTotal := c.mu.totalBytes + toAdd
	if newTotal > c.maxTotalBytes {
		return fmt.Errorf(
			"adding %s would exceed cache size limit of %s by %s",
			humanize.IBytes(toAdd),
			humanize.IBytes(c.maxTotalBytes),
			humanize.IBytes(newTotal-c.maxTotalBytes),
		)
	}
	c.mu.totalBytes = newTotal
	return nil
}

// releaseSpace releases a previously reserved number of bytes from the cache's
// capacity accounting. It is safe to call with zero.
func (c *DiskCache) releaseSpace(toRelease uint64) {
	if toRelease == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Infof("releaseSpace: releasing %s, total bytes: %s: %s", humanize.IBytes(toRelease), humanize.IBytes(c.mu.totalBytes), debug.Stack())
	if toRelease > c.mu.totalBytes {
		log.Errorf(
			"releaseSpace: invariant violation: total size underflow: %s > %s (delta: %s)",
			humanize.IBytes(toRelease),
			humanize.IBytes(c.mu.totalBytes),
			humanize.IBytes(toRelease-c.mu.totalBytes),
		)
		c.mu.totalBytes = 0
		return
	}
	c.mu.totalBytes -= toRelease
}

// htlHashLoader is a sectionDataLoader that loads sections based on the executable's
// htl hash.
type htlHashLoader struct {
	htlHash htlhash.Hash
	c       *DiskCache
}

func (h *htlHashLoader) load(
	cr compressedSectionMetadata, mef *MMappingElfFile,
) (SectionData, error) {
	return h.c.loadSection(h.htlHash, cr, mef)
}

// getSection returns a SectionData for the requested compressedFileRange. It
// either returns a cached section or performs the decompression, ensuring only
// one goroutine performs the decompression work at a time.
func (c *DiskCache) loadSection(
	htlHash htlhash.Hash,
	cr compressedSectionMetadata,
	mef *MMappingElfFile,
) (_ SectionData, retErr error) {
	key := cacheKey{
		htlHash:                   htlHash,
		compressedSectionMetadata: cr,
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

// DiskFile is a writable file that reserves space from the DiskCache. It
// enforces a maximum size and can be converted to a memory map.
type DiskFile struct {
	c             *DiskCache
	f             *os.File
	name          string
	reservedSpace uint64
	maxSpace      uint64
	used          uint64
	cleanup       runtime.Cleanup
	closed        bool
}

// NewFile creates a new writable file within the cache. The file starts with
// an initial reservation against the cache's capacity. The file is unlinked
// immediately, so it will be removed from the filesystem even if the process
// crashes; it remains accessible via its file descriptor until closed.
func (c *DiskCache) NewFile(name string, maxSize, initialSize uint64) (_ *DiskFile, retErr error) {
	if name == "" {
		return nil, errors.New("name must not be empty")
	}
	if initialSize > maxSize {
		return nil, errors.New("initialSize must be <= maxSize")
	}
	// If the disk doesn't have enough space, we can't create the file.
	if err := c.checker.check(maxSize); err != nil {
		return nil, err
	}
	if base := path.Base(name); base != name {
		return nil, errors.New("name must not contain path separators")
	}
	f, err := os.CreateTemp(c.dirPath, fmt.Sprintf("%s.%d", name, os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = f.Close()
		}
	}()

	// Unlink immediately to avoid leaking the file.
	if err := os.Remove(f.Name()); err != nil {
		return nil, fmt.Errorf("failed to remove temp file: %w", err)
	}
	if err := c.reserveSpace(uint64(initialSize)); err != nil {
		return nil, err
	}
	type spaceAndFile struct {
		space uint64
		file  *os.File
	}
	df := &DiskFile{
		c:             c,
		f:             f,
		name:          name,
		reservedSpace: initialSize,
		maxSpace:      maxSize,
	}
	df.cleanup = runtime.AddCleanup(df, func(s spaceAndFile) {
		_ = s.file.Close()
		c.releaseSpace(s.space)
	}, spaceAndFile{space: initialSize, file: f})
	return df, nil
}

// Write appends the given bytes to the file, growing the reservation if
// necessary. If the write would exceed the max size, it fails.
func (df *DiskFile) Write(p []byte) (int, error) {
	if df == nil || df.f == nil || df.closed {
		return 0, errors.New("write on closed DiskFile")
	}
	if len(p) == 0 {
		return 0, nil
	}
	// Check max size.
	neededTotal := df.used + uint64(len(p))
	if neededTotal > df.maxSpace {
		return 0, fmt.Errorf(
			"write exceeds max size: need %s, max %s (over by %s)",
			humanize.IBytes(neededTotal),
			humanize.IBytes(df.maxSpace),
			humanize.IBytes(neededTotal-df.maxSpace),
		)
	}

	// Ensure reservation is large enough for this write.
	if neededTotal > df.reservedSpace {
		needed := neededTotal - df.reservedSpace
		increment := max(needed, 2*df.reservedSpace)
		remaining := df.maxSpace - df.used
		increment = min(increment, remaining)
		if err := df.c.reserveSpace(increment); err != nil {
			return 0, err
		}
		df.reservedSpace += increment
	}

	n, err := df.f.Write(p)
	if n > 0 {
		df.used += uint64(n)
	}
	if err != nil {
		return n, fmt.Errorf("failed to write DiskFile: %w", err)
	}
	return n, nil
}

// Close aborts the DiskFile without converting it to a memory map, releasing
// its reservation and closing the underlying file.
func (df *DiskFile) Close() error {
	if df == nil || df.closed {
		return nil
	}
	defer runtime.KeepAlive(df)
	df.cleanup.Stop()
	defer func() { df.closed = true }()
	df.closed = true
	err := df.f.Close()
	df.f = nil
	if df.reservedSpace > 0 {
		df.c.releaseSpace(df.reservedSpace)
		df.reservedSpace = 0
	}
	df.used = 0
	return err
}

// IntoMMap converts the DiskFile into a SectionData backed by a memory map.
// The DiskFile is closed as part of this operation. The returned SectionData
// behaves like cachedSectionData and will release its accounting when closed
// or garbage collected.
func (df *DiskFile) IntoMMap(flags int) (_ SectionData, retErr error) {
	defer func() {
		if retErr != nil {
			_ = df.Close()
		}
	}()
	if df == nil || df.f == nil || df.closed {
		return nil, errors.New("IntoMMap on closed DiskFile")
	}
	if df.used == 0 {
		return nil, errors.New("cannot mmap empty DiskFile")
	}
	if err := df.f.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync DiskFile: %w", err)
	}
	// Ensure the length fits in an int for syscall.Mmap.
	maxInt := int(^uint(0) >> 1)
	if df.used > uint64(maxInt) {
		return nil, fmt.Errorf("mmap length overflow: %d", df.used)
	}

	// Map the file as read-only shared mapping.
	m, err := syscall.Mmap(int(df.f.Fd()), 0, int(df.used), flags, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap DiskFile: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = syscall.Munmap(m)
		}
	}()
	// Close the file descriptor after successful mmap.
	if err := df.f.Close(); err != nil {
		return nil, fmt.Errorf("failed to close DiskFile after mmap: %w", err)
	}
	df.f = nil

	// TODO: This use of the cachedSectionData is a hack. We should refactor
	// this package to have two layers of abstraction: one around storing disk
	// files and one around caching section data.

	// Prepare a unique cache key using random bytes for the htl hash.
	var randHash htlhash.Hash
	if _, err := io.ReadFull(rand.Reader, randHash[:]); err != nil {
		_ = syscall.Munmap(m)
		return nil, fmt.Errorf("failed to generate random hash: %w", err)
	}
	key := cacheKey{
		htlHash: randHash,
		compressedSectionMetadata: compressedSectionMetadata{
			name: df.name,
			compressedFileRange: compressedFileRange{
				format:             compressionFormatNone,
				offset:             0,
				compressedLength:   int64(df.used),
				uncompressedLength: int64(df.used),
			},
		},
	}

	// Create the cache entry and adjust accounting: replace reservation with
	// the actual used size.
	entry := &cacheEntry{
		cacheKey: key,
		cache:    df.c,
		deleted:  make(chan struct{}),
	}
	entry.decompress.data = m
	// Decompression never ran; but release() expects refCount to be set.
	df.c.mu.Lock()
	// Replace reservation with actual usage in totalBytes.
	if df.reservedSpace > df.c.mu.totalBytes {
		// Should not happen, but guard against underflow.
		log.Errorf(
			"IntoMMap: invariant violation: reservedSpace > totalBytes: %s > %s (delta: %s)",
			humanize.IBytes(df.reservedSpace),
			humanize.IBytes(df.c.mu.totalBytes),
			humanize.IBytes(df.reservedSpace-df.c.mu.totalBytes),
		)
		df.c.mu.totalBytes = 0
	} else {
		df.c.mu.totalBytes -= df.reservedSpace
	}
	df.c.mu.totalBytes += df.used
	entry.cacheMu.refCount = 1
	entry.cacheMu.deleting = false
	df.c.mu.entries[key] = entry
	df.c.mu.Unlock()

	// Finalize DiskFile state.
	df.reservedSpace = 0
	df.closed = true
	df.cleanup.Stop()
	runtime.KeepAlive(df)

	sd := new(cachedSectionData)
	sd.entry = entry
	sd.cleanup = runtime.AddCleanup(sd, cleanupCacheEntry, entry)
	return sd, nil
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
	// If the entry was never actually mmapped, we don't need to munmap.
	var munmapErr error
	if e.decompress.data != nil {
		if err := syscall.Munmap(e.decompress.data); err != nil {
			munmapErr = fmt.Errorf("failed to munmap decompressed data: %w", err)
		}
		e.decompress.data = nil
	}
	return munmapErr
}

type cacheKey struct {
	htlHash htlhash.Hash
	compressedSectionMetadata
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
