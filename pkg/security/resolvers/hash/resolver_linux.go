// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hash holds hash related files
package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/glaslos/ssdeep"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	// minSSDeepFileSize is the minimum file size (in bytes) required by ssdeep to produce a digest.
	// glaslos/ssdeep returns ErrFileTooSmall when totalSize <= 4096 and Force is false.
	minSSDeepFileSize    = 4096
	ssdeepRollingWindow  = 7
	ssdeepBlockMin       = 3
	ssdeepSpamSumLength  = 64
	ssdeepHalfSpamSumLen = ssdeepSpamSumLength / 2
	ssdeepNumBlockhashes = 31
	ssdeepMaxTotalSize   = ssdeepBlockMin << (ssdeepNumBlockhashes - 1) * ssdeepSpamSumLength
	ssdeepHashPrime      = 0x93
	ssdeepHashInit       = 0x27
	ssdeepB64String      = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

type fastSSDeepBlock struct {
	hashString         []byte
	blockSize          uint32
	tail1, tail2       byte
	hasTail1, hasTail2 bool
}

type fastSSDeepHasher struct {
	window       [ssdeepRollingWindow + 1]byte
	h1, h2, h3   uint32
	n            uint32
	iStart, iEnd int
	totalSize    uint64
	bsizeMask    uint32
	blockHash1   [ssdeepNumBlockhashes + 16]byte
	blockHash2   [ssdeepNumBlockhashes + 16]byte
	blocks       [ssdeepNumBlockhashes]fastSSDeepBlock
}

func stepSWAR8(w, b8 uint64) uint64 {
	return (((w * 3) + ((w & 0x0303030303030303) << 4)) & 0x3f3f3f3f3f3f3f3f) ^ b8
}

func (s *fastSSDeepHasher) loadSWAR() (w1Lo, w2Lo, w1Hi, w2Hi uint64) {
	iStart := s.iStart
	w1Lo = binary.LittleEndian.Uint64(s.blockHash1[iStart : iStart+8])
	w2Lo = binary.LittleEndian.Uint64(s.blockHash2[iStart : iStart+8])
	if s.iEnd-iStart > 8 {
		w1Hi = binary.LittleEndian.Uint64(s.blockHash1[iStart+8 : iStart+16])
		w2Hi = binary.LittleEndian.Uint64(s.blockHash2[iStart+8 : iStart+16])
	}
	return w1Lo, w2Lo, w1Hi, w2Hi
}

func (s *fastSSDeepHasher) storeSWAR(w1Lo, w2Lo, w1Hi, w2Hi uint64) {
	span := s.iEnd - s.iStart
	var tmp1, tmp2 [16]byte
	binary.LittleEndian.PutUint64(tmp1[:8], w1Lo)
	binary.LittleEndian.PutUint64(tmp2[:8], w2Lo)
	if span > 8 {
		binary.LittleEndian.PutUint64(tmp1[8:16], w1Hi)
		binary.LittleEndian.PutUint64(tmp2[8:16], w2Hi)
	}
	n := min(span, 16)
	copy(s.blockHash1[s.iStart:s.iStart+n], tmp1[:n])
	copy(s.blockHash2[s.iStart:s.iStart+n], tmp2[:n])
}

func newFastSSDeepHasher() *fastSSDeepHasher {
	s := &fastSSDeepHasher{
		iEnd: 1,
	}
	s.Reset()
	return s
}

func (s *fastSSDeepHasher) Reset() {
	*s = fastSSDeepHasher{
		iEnd: 1,
	}
	for i := range s.blocks {
		s.blocks[i].blockSize = ssdeepBlockMin << i
		s.blockHash1[i] = ssdeepHashInit
		s.blockHash2[i] = ssdeepHashInit
	}
}

func (s *fastSSDeepHasher) BlockSize() int {
	return ssdeepBlockMin
}

func (s *fastSSDeepHasher) Size() int {
	return ssdeepSpamSumLength
}

func (s *fastSSDeepHasher) Write(p []byte) (int, error) {
	s.totalSize += uint64(len(p))
	h1, h2, h3, n := s.h1, s.h2, s.h3, s.n
	window := s.window
	iStart, iEnd := s.iStart, s.iEnd
	span := iEnd - iStart
	bsizeMask := s.bsizeMask
	bh1 := &s.blockHash1
	bh2 := &s.blockHash2
	w1Lo, w2Lo, w1Hi, w2Hi := s.loadSWAR()

	for _, b := range p {
		b8 := uint64(b&63) * 0x0101010101010101
		w1Lo = stepSWAR8(w1Lo, b8)
		w2Lo = stepSWAR8(w2Lo, b8)
		if span > 8 {
			w1Hi = stepSWAR8(w1Hi, b8)
			w2Hi = stepSWAR8(w2Hi, b8)
			for i := iStart + 16; i < iEnd; i++ {
				bh1[i] = ((bh1[i] * ssdeepHashPrime) ^ b) & 63
				bh2[i] = ((bh2[i] * ssdeepHashPrime) ^ b) & 63
			}
		}

		bc := uint32(b)
		h2 -= h1
		h2 += ssdeepRollingWindow * bc
		h1 += bc
		h1 -= uint32(window[n])
		window[n] = b
		n++
		if n == ssdeepRollingWindow {
			n = 0
		}
		h3 = (h3 << 5) ^ bc

		rh := h1 + h2 + h3
		if (rh+1)&bsizeMask != 0 || (rh+1)%ssdeepBlockMin != 0 || rh == math.MaxUint32 {
			continue
		}
		s.storeSWAR(w1Lo, w2Lo, w1Hi, w2Hi)
		s.triggerBoundary(rh)
		iStart, iEnd, bsizeMask = s.iStart, s.iEnd, s.bsizeMask
		span = iEnd - iStart
		w1Lo, w2Lo, w1Hi, w2Hi = s.loadSWAR()
	}

	s.storeSWAR(w1Lo, w2Lo, w1Hi, w2Hi)
	s.h1, s.h2, s.h3, s.n = h1, h2, h3, n
	s.window = window
	return len(p), nil
}

func (s *fastSSDeepHasher) triggerBoundary(rh uint32) {
	for i := s.iStart; i < s.iEnd; i++ {
		block := &s.blocks[i]
		if rh%block.blockSize == (block.blockSize - 1) {
			if len(block.hashString) == 0 {
				if s.iEnd <= ssdeepNumBlockhashes-1 {
					s.blockHash1[s.iEnd] = s.blockHash1[s.iEnd-1]
					s.blockHash2[s.iEnd] = s.blockHash2[s.iEnd-1]
					s.iEnd++
				}
			}
			block.tail1 = s.blockHash1[i]
			block.tail2 = s.blockHash2[i]
			if len(block.hashString) < ssdeepSpamSumLength-1 {
				block.hashString = append(block.hashString, block.tail1)
				block.tail1 = 0
				block.hasTail1 = false
				s.blockHash1[i] = ssdeepHashInit
				if len(block.hashString) < ssdeepHalfSpamSumLen {
					s.blockHash2[i] = ssdeepHashInit
					block.tail2 = 0
					block.hasTail2 = false
				} else {
					block.hasTail2 = true
				}
			} else {
				block.hasTail1 = true
				block.hasTail2 = true
				if s.totalSize > uint64(s.blocks[s.iStart].blockSize*ssdeepSpamSumLength) &&
					len(s.blocks[s.iStart+1].hashString) >= ssdeepHalfSpamSumLen {
					s.iStart++
					s.bsizeMask = (s.bsizeMask << 1) + 1
				}
			}
		}
	}
}

func (s *fastSSDeepHasher) Sum(b []byte) []byte {
	return append(b, s.digest()...)
}

func (s *fastSSDeepHasher) digest() string {
	if !ssdeep.Force && s.totalSize <= minSSDeepFileSize {
		return ""
	}
	if s.totalSize > ssdeepMaxTotalSize {
		return ""
	}

	i := s.iStart
	for uint64(uint32(ssdeepBlockMin)<<i)*ssdeepSpamSumLength < s.totalSize {
		i++
	}

	if i >= s.iEnd {
		i = s.iEnd - 1
	}
	for i > s.iStart && len(s.blocks[i].hashString) < ssdeepHalfSpamSumLen {
		i--
	}
	bl1 := s.blocks[i]
	bl1Hash := s.blockHash1[i]
	bl2 := s.blocks[i+1]
	bl2Hash := s.blockHash2[i+1]
	if i >= s.iEnd-1 {
		bl2 = s.blocks[i]
		bl2.hashString = append([]byte{}, bl1.hashString...)
		bl2Hash = s.blockHash2[i]
	}
	rh := s.h1 + s.h2 + s.h3

	if len(bl2.hashString) > ssdeepHalfSpamSumLen-1 {
		bl2.hashString = bl2.hashString[:ssdeepHalfSpamSumLen-1]
	}

	if rh != 0 {
		bl1.hashString = append(bl1.hashString, bl1Hash)
		bl2.hashString = append(bl2.hashString, bl2Hash)
	} else {
		if bl1.hasTail1 {
			bl1.hashString = append(bl1.hashString, bl1.tail1)
		}
		if bl2.hasTail2 {
			bl2.hashString = append(bl2.hashString, bl2.tail2)
		}
	}
	for j := range bl1.hashString {
		bl1.hashString[j] = ssdeepB64String[bl1.hashString[j]]
	}
	for j := range bl2.hashString {
		bl2.hashString[j] = ssdeepB64String[bl2.hashString[j]]
	}

	return fmt.Sprintf("%d:%s:%s", bl1.blockSize, bl1.hashString, bl2.hashString)
}

var (
	// ErrSizeLimitReached indicates that the size limit was reached
	ErrSizeLimitReached = errors.New("size limit reached")
)

// SizeLimitedWriter implements io.Writer and returns an error if more than the configured amount of data is read
type SizeLimitedWriter struct {
	dst   io.Writer
	limit int
}

// newSizeLimitedWriter create a new SizeLimitedWriter that accepts at most 'limit' bytes.
func newSizeLimitedWriter(dst io.Writer, limit int) *SizeLimitedWriter {
	return &SizeLimitedWriter{
		dst:   dst,
		limit: limit,
	}
}

// Write attempts to write to the writer
func (l *SizeLimitedWriter) Write(p []byte) (int, error) {
	lp := len(p)
	if lp > l.limit {
		return 0, ErrSizeLimitReached
	}
	written, err := l.dst.Write(p)
	l.limit -= written
	return written, err
}

// ResolverOpts defines hash resolver options
type ResolverOpts struct {
	// Enabled defines if the hash resolver should be enabled
	Enabled bool
	// MaxFileSize defines the maximum size of the files that the hash resolver is allowed to hash
	MaxFileSize int64
	// HashAlgorithms defines the hashes that hash resolver needs to compute
	HashAlgorithms []model.HashAlgorithm
	// EventTypes defines the list of event types for which we may compute hashes. Warning: enabling a FIM event will
	// automatically make the hash resolver also hash process binary files.
	EventTypes []model.EventType
}

// LRUCacheKey is the structure used to access cached hashes
type LRUCacheKey struct {
	path  string
	inode uint64
}

// LRUCacheEntry is the structure used to cache hashes
// It includes file metadata (inode, mtime, size) to detect if the file has changed
type LRUCacheEntry struct {
	state  model.HashState
	hashes []string
	// File metadata to detect changes
	inode uint64 // file inode
	mtime uint64 // modification time in nanoseconds
	size  int64  // file size in bytes
}

// SSDeepCacheKey is the key used for caching ssdeep hashes based on cheaper hashes
type SSDeepCacheKey struct {
	size      int64  // file size in bytes
	cheapHash string // the cheapest hash used as cache key
}

// SSDeepCacheEntry stores the cached ssdeep hash
type SSDeepCacheEntry struct {
	ssdeepHash string
}

const defaultAsyncHashQueueSize = 4096

// AsyncHashCallback is invoked when an asynchronous or fast-path file hash computation finishes.
// async is false when resolved synchronously on the caller's goroutine and true when resolved on the background worker.
type AsyncHashCallback func(state model.HashState, hashes []string, async bool)

type asyncHashJob struct {
	eventType   model.EventType
	cgroupID    containerutils.CGroupID
	pid         uint32
	file        model.FileEvent
	maxFileSize int64
	onDone      AsyncHashCallback
	flushDone   chan struct{}
}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
	opts           ResolverOpts
	statsdClient   statsd.ClientInterface
	limiter        *rate.Limiter
	cgroupResolver *cgroup.Resolver
	replace        map[string]string

	cache       *lru.Cache[LRUCacheKey, *LRUCacheEntry]
	ssdeepCache *lru.Cache[SSDeepCacheKey, *SSDeepCacheEntry]

	bufferPool *ddsync.TypedPool[[]byte]

	asyncOnce  sync.Once
	asyncQueue chan asyncHashJob
	inFlight   atomic.Int64
}

// NewResolver returns a new instance of the hash resolver
func NewResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, cgroupResolver *cgroup.Resolver) (*Resolver, error) {
	if !c.HashResolverEnabled {
		return &Resolver{}, nil
	}

	hashAlgorithms := slices.Clone(c.HashResolverHashAlgorithms)
	if fips.BuiltForFIPS() {
		// sha1 is removed from the list because it's not a FIPS-approved hash algorithm
		hashAlgorithms = slices.DeleteFunc(hashAlgorithms, func(algorithm model.HashAlgorithm) bool {
			return algorithm == model.SHA1
		})
	}

	var cache *lru.Cache[LRUCacheKey, *LRUCacheEntry]
	var ssdeepCache *lru.Cache[SSDeepCacheKey, *SSDeepCacheEntry]
	if c.HashResolverCacheSize > 0 {
		var err error
		cache, err = lru.New[LRUCacheKey, *LRUCacheEntry](c.HashResolverCacheSize)
		if err != nil {
			return nil, fmt.Errorf("couldn't create hash resolver cache: %w", err)
		}
		// Create a separate cache for ssdeep hashes only if ssdeep algorithm is enabled
		if slices.Contains(hashAlgorithms, model.SSDEEP) {
			ssdeepCache, err = lru.New[SSDeepCacheKey, *SSDeepCacheEntry](c.HashResolverCacheSize)
			if err != nil {
				return nil, fmt.Errorf("couldn't create ssdeep cache: %w", err)
			}
		}
	}

	burst := max(c.HashResolverMaxHashRate, 0)

	// size of the buffer used to copy data from the file to the hash functions
	const copyBufferSize = 32 * 1024

	r := &Resolver{
		opts: ResolverOpts{
			Enabled:        true,
			MaxFileSize:    c.HashResolverMaxFileSize,
			HashAlgorithms: sortAlgorithmsByCost(hashAlgorithms),
			EventTypes:     c.HashResolverEventTypes,
		},
		cgroupResolver: cgroupResolver,
		statsdClient:   statsdClient,
		limiter:        rate.NewLimiter(rate.Limit(c.HashResolverMaxHashRate), burst),
		cache:          cache,
		ssdeepCache:    ssdeepCache,
		bufferPool:     ddsync.NewSlicePool[byte](copyBufferSize, copyBufferSize),
		replace:        c.HashResolverReplace,
	}

	return r, nil
}

// ComputeHashesFromEvent calls ComputeHashes using the provided event
func (resolver *Resolver) ComputeHashesFromEvent(event *model.Event, file *model.FileEvent, maxFileSize int64) []string {
	if !resolver.opts.Enabled {
		return nil
	}

	// resolve FileEvent
	event.FieldHandlers.ResolveFilePath(event, file)

	process := event.ProcessContext.Process
	resolver.HashFileEvent(event.GetEventType(), process.CGroup.CGroupID, process.Pid, file, maxFileSize)

	return file.Hashes
}

// ComputeHashes computes the hashes of the provided file event.
// Disclaimer: This resolver considers that the FileEvent has already been resolved
func (resolver *Resolver) ComputeHashes(eventType model.EventType, process *model.Process, file *model.FileEvent, maxFileSize int64) []string {
	if !resolver.opts.Enabled {
		return nil
	}

	resolver.HashFileEvent(eventType, process.CGroup.CGroupID, process.Pid, file, maxFileSize)

	return file.Hashes
}

// ComputeHashesFromEventAsync resolves the file path on the caller goroutine and schedules hash calculation
// on the background worker unless it can be satisfied immediately without filesystem I/O.
func (resolver *Resolver) ComputeHashesFromEventAsync(event *model.Event, file *model.FileEvent, maxFileSize int64, onDone AsyncHashCallback) {
	if resolver == nil || !resolver.opts.Enabled || event == nil || file == nil {
		return
	}

	event.FieldHandlers.ResolveFilePath(event, file)

	process := &event.ProcessContext.Process
	resolver.ComputeHashesAsync(event.GetEventType(), process, file, maxFileSize, onDone)
}

// ComputeHashesAsync schedules hash calculation for an already-resolved FileEvent on the background worker
// unless it can be satisfied immediately without filesystem I/O.
func (resolver *Resolver) ComputeHashesAsync(eventType model.EventType, process *model.Process, file *model.FileEvent, maxFileSize int64, onDone AsyncHashCallback) {
	if resolver == nil || !resolver.opts.Enabled || process == nil || file == nil {
		return
	}

	resolver.HashFileEventAsync(eventType, process.CGroup.CGroupID, process.Pid, file, maxFileSize, onDone)
}

// HashFileEventAsync performs non-blocking validation on the caller goroutine and offloads `/proc/<pid>/root`
// stat/open/read and cryptographic hashing to the background worker goroutine.
func (resolver *Resolver) HashFileEventAsync(eventType model.EventType, cgroupID containerutils.CGroupID, pid uint32, file *model.FileEvent, maxFileSize int64, onDone AsyncHashCallback) {
	if resolver == nil || !resolver.opts.Enabled || file == nil {
		return
	}

	fileCopy := *file
	if len(file.Hashes) > 0 {
		fileCopy.Hashes = slices.Clone(file.Hashes)
	}

	if resolver.tryFastHashFileEvent(eventType, &fileCopy, maxFileSize) {
		file.HashState = fileCopy.HashState
		file.Hashes = fileCopy.Hashes
		if onDone != nil {
			onDone(fileCopy.HashState, slices.Clone(fileCopy.Hashes), false)
		}
		return
	}

	job := asyncHashJob{
		eventType:   eventType,
		cgroupID:    cgroupID,
		pid:         pid,
		file:        fileCopy,
		maxFileSize: maxFileSize,
		onDone:      onDone,
	}
	if !resolver.enqueueAsyncHash(job) {
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashWasRateLimited.String())
		file.HashState = model.HashWasRateLimited
		if onDone != nil {
			onDone(model.HashWasRateLimited, nil, false)
		}
	}
}

func (resolver *Resolver) tryFastHashFileEvent(eventType model.EventType, file *model.FileEvent, maxFileSize int64) bool {
	if file.HashState == model.Done {
		return true
	}
	if file.HashState != model.NoHash && file.HashState != model.HashWasRateLimited {
		return true
	}
	if !slices.Contains(resolver.opts.EventTypes, eventType) {
		file.HashState = model.EventTypeNotConfigured
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.EventTypeNotConfigured.String())
		return true
	}
	if !file.IsPathnameStrResolved || len(file.PathnameStr) == 0 {
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.PathnameResolutionError.String())
		file.HashState = model.PathnameResolutionError
		return true
	}
	if hashStr, exists := resolver.replace[file.PathnameStr]; exists {
		file.Hashes = append(file.Hashes, hashStr)
		file.HashState = model.Done
		return true
	}
	if resolver.cache != nil && file.Inode != 0 && file.MTime != 0 {
		if maxFileSize <= 0 {
			maxFileSize = resolver.opts.MaxFileSize
		}
		fileKey := LRUCacheKey{
			path:  file.PathnameStr,
			inode: file.Inode,
		}
		if cacheEntry, ok := resolver.cache.Get(fileKey); ok {
			if cacheEntry.inode == file.Inode && cacheEntry.mtime == file.MTime && cacheEntry.size <= maxFileSize {
				file.HashState = cacheEntry.state
				file.Hashes = slices.Clone(cacheEntry.hashes)
				hashResolverTelemetry.hashCacheHit.Inc(eventType.String())
				return true
			}
		}
	}
	return false
}

func (resolver *Resolver) startAsyncWorker() {
	resolver.asyncQueue = make(chan asyncHashJob, defaultAsyncHashQueueSize)
	go resolver.runAsyncWorker()
}

func (resolver *Resolver) enqueueAsyncHash(job asyncHashJob) bool {
	resolver.asyncOnce.Do(resolver.startAsyncWorker)
	resolver.inFlight.Add(1)
	select {
	case resolver.asyncQueue <- job:
		return true
	default:
		resolver.inFlight.Add(-1)
		return false
	}
}

func (resolver *Resolver) runAsyncWorker() {
	for job := range resolver.asyncQueue {
		if job.flushDone != nil {
			close(job.flushDone)
			continue
		}
		resolver.HashFileEvent(job.eventType, job.cgroupID, job.pid, &job.file, job.maxFileSize)
		if job.onDone != nil {
			job.onDone(job.file.HashState, slices.Clone(job.file.Hashes), true)
		}
		resolver.inFlight.Add(-1)
	}
}

// Flush blocks until all queued and in-flight asynchronous hash computations have completed.
func (resolver *Resolver) Flush() {
	if resolver == nil || !resolver.opts.Enabled || resolver.inFlight.Load() == 0 {
		return
	}
	resolver.asyncOnce.Do(resolver.startAsyncWorker)
	for resolver.inFlight.Load() > 0 {
		done := make(chan struct{})
		resolver.asyncQueue <- asyncHashJob{flushDone: done}
		<-done
	}
}

// getHashFunction returns the hash function for the provided algorithm
func (resolver *Resolver) getHashFunction(algorithm model.HashAlgorithm) hash.Hash {
	switch algorithm {
	case model.SHA1:
		return sha1.New()
	case model.SHA256:
		return sha256.New()
	case model.MD5:
		return md5.New()
	case model.SSDEEP:
		return newFastSSDeepHasher()
	default:
		return nil
	}
}

// getHashCost returns the relative computational cost of a hash algorithm
// Lower values indicate cheaper algorithms
func getHashCost(algorithm model.HashAlgorithm) int {
	switch algorithm {
	case model.MD5:
		return 1
	case model.SHA1:
		return 2
	case model.SHA256:
		return 3
	case model.SSDEEP:
		return 100 // SSDEEP is significantly more expensive
	default:
		return 999
	}
}

// sortAlgorithmsByCost sorts hash algorithms from least costly to most costly
func sortAlgorithmsByCost(algorithms []model.HashAlgorithm) []model.HashAlgorithm {
	sorted := make([]model.HashAlgorithm, len(algorithms))
	copy(sorted, algorithms)
	slices.SortFunc(sorted, func(a, b model.HashAlgorithm) int {
		return getHashCost(a) - getHashCost(b)
	})
	return sorted
}

type fileUniqKey struct {
	dev   uint64
	inode uint64
}

func getFileInfo(path string) (fs.FileMode, int64, uint64, fileUniqKey, error) {
	stat, err := utils.UnixStat(path)
	if err != nil {
		return 0, 0, 0, fileUniqKey{}, err
	}

	fkey := fileUniqKey{
		dev:   stat.Dev,
		inode: stat.Ino,
	}

	// Convert mtime to nanoseconds
	mtime := uint64(stat.Mtim.Sec)*1e9 + uint64(stat.Mtim.Nsec)

	return utils.UnixStatModeToGoFileMode(stat.Mode), stat.Size, mtime, fkey, nil
}

// HashFileEvent hashes the provided file event
func (resolver *Resolver) HashFileEvent(eventType model.EventType, cgroupID containerutils.CGroupID, pid uint32, file *model.FileEvent, maxFileSize int64) {
	if !resolver.opts.Enabled {
		return
	}

	if resolver.tryFastHashFileEvent(eventType, file, maxFileSize) {
		return
	}

	if maxFileSize <= 0 {
		maxFileSize = resolver.opts.MaxFileSize
	}

	// add pid one for hash resolution outside of a container
	rootPIDs := []uint32{1, pid}
	if resolver.cgroupResolver != nil {
		if cacheEntry := resolver.cgroupResolver.GetCacheEntryByCgroupID(cgroupID); cacheEntry != nil {
			rootPIDs = cacheEntry.GetPIDs()
			if pid != 0 && !slices.Contains(rootPIDs, pid) {
				rootPIDs = append(rootPIDs, pid)
			}
		}
	}

	// open the target file
	var (
		lastErr         error
		f               *os.File
		mode            fs.FileMode
		size            int64
		mtime           uint64
		fkey            fileUniqKey
		entryInode      uint64
		fileKey         LRUCacheKey
		rateReservation *rate.Reservation
		failedCache     = make(map[fileUniqKey]struct{})
	)
	for _, pidCandidate := range rootPIDs {
		path := utils.ProcRootFilePath(pidCandidate, file.PathnameStr)
		mode, size, mtime, fkey, lastErr = getFileInfo(path)
		if lastErr != nil {
			continue
		}

		if !mode.IsRegular() {
			// the file is not regular, break out early and the error will be reported in the `if f == nil` check
			break
		}

		if size > maxFileSize {
			if rateReservation != nil {
				rateReservation.Cancel()
			}
			hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileTooBig.String())
			file.HashState = model.FileTooBig
			return
		}

		if size == 0 {
			if rateReservation != nil {
				rateReservation.Cancel()
			}
			hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileEmpty.String())
			file.HashState = model.FileEmpty
			return
		}

		entryInode = file.Inode
		if entryInode == 0 {
			entryInode = fkey.inode
		}
		fileKey = LRUCacheKey{
			path:  file.PathnameStr,
			inode: entryInode,
		}

		if resolver.cache != nil {
			if cacheEntry, ok := resolver.cache.Get(fileKey); ok {
				if cacheEntry.inode == entryInode &&
					cacheEntry.mtime == mtime &&
					cacheEntry.size == size {
					file.HashState = cacheEntry.state
					file.Hashes = slices.Clone(cacheEntry.hashes)
					hashResolverTelemetry.hashCacheHit.Inc(eventType.String())
					if rateReservation != nil {
						rateReservation.Cancel()
					}
					return
				}
			}
		}

		if rateReservation == nil {
			rateReservation = resolver.limiter.Reserve()
			if !rateReservation.OK() || rateReservation.Delay() > 0 {
				rateReservation.Cancel()
				hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashWasRateLimited.String())
				file.HashState = model.HashWasRateLimited
				return
			}
		}

		if _, ok := failedCache[fkey]; ok {
			// we already tried to open this file and failed, no need to try again
			continue
		}

		f, lastErr = os.Open(path)
		if lastErr != nil {
			failedCache[fkey] = struct{}{}
			continue
		}

		// we manage to open the file
		break
	}
	if lastErr != nil {
		if rateReservation != nil {
			rateReservation.Cancel()
		}
		if os.IsNotExist(lastErr) {
			file.HashState = model.FileNotFound
			hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileNotFound.String())
			return
		}
		// We can't open this file, most likely because it isn't a regular file. Example seen in production:
		//  - open(/host/proc/2129077/root/tmp/plugin3037415914) => no such device or address
		//  - open(/host/proc/576833/root/run/containerd/runc/k8s.io/2b100...96104/runc.WUXTJB) => permission denied
		//  - open(/host/proc/313599/root/proc/10987/task/10988/status/10987/task) => not a directory
		//  - open(/host/proc/263082/root/usr/local/bin/runc) => no such process
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileOpenError.String())
		file.HashState = model.FileOpenError
		return
	}

	if f == nil {
		if rateReservation != nil {
			rateReservation.Cancel()
		}
		file.HashState = model.FileNotFound
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileNotFound.String())
		return
	}
	defer f.Close()

	fileReader := struct{ io.Reader }{f}

	// Map to store computed hashes by algorithm (to preserve original order)
	var computedHashes []string

	// Step 1: Compute all non-SSDEEP hashes in a single pass (ordered by cost)
	var hashers []io.Writer
	var hasherAlgorithms []model.HashAlgorithm
	for _, algorithm := range resolver.opts.HashAlgorithms {
		// SSDEEP is handled separately
		if algorithm == model.SSDEEP {
			continue
		}

		h := resolver.getHashFunction(algorithm)
		if h == nil {
			// shouldn't happen, ignore
			continue
		}
		hashers = append(hashers, h)
		hasherAlgorithms = append(hasherAlgorithms, algorithm)
	}

	if len(hashers) > 0 {
		multiWriter := newSizeLimitedWriter(io.MultiWriter(hashers...), int(maxFileSize))

		buffer := resolver.bufferPool.Get()
		_, err := io.CopyBuffer(multiWriter, fileReader, *buffer)
		resolver.bufferPool.Put(buffer)
		if err != nil {
			if errors.Is(err, ErrSizeLimitReached) {
				hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileTooBig.String())
				file.HashState = model.FileTooBig
				return
			}
			// We can't read this file, most likely because it isn't a regular file (despite the check above). Example seen
			// in production:
			//  - read(/host/proc/2076/root/proc/1/fdinfo/64) => no such file or directory
			//  - read(/host/proc/2328/root/run/netns/a574a27c) => invalid argument
			hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileOpenError.String())
			file.HashState = model.FileOpenError
			return
		}

		// Store computed hashes in map
		for i, algorithm := range hasherAlgorithms {
			var hashStr strings.Builder
			hashStr.WriteString(algorithm.String())
			if hashStr.Len() > 0 {
				hashStr.WriteByte(':')
			}
			digest := hashers[i].(hash.Hash).Sum(nil)
			hencoder := hex.NewEncoder(&hashStr)
			if _, err := hencoder.Write(digest); err != nil {
				// we failed to compute the digest
				hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
				continue
			}

			computedHashes = append(computedHashes, hashStr.String())
			hashResolverTelemetry.hashCount.Inc(eventType.String(), algorithm.String())
		}
	}

	// Step 2: Handle SSDEEP separately with caching based on cheapest hash
	if resolver.ssdeepCache != nil {
		if !ssdeep.Force && size <= minSSDeepFileSize {
			hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
		} else {
			var (
				cheapestHash  string
				ssdeepHashStr string
				foundInCache  bool
			)

			if len(computedHashes) > 0 {
				// Find the cheapest computed hash to use as cache key
				cheapestHash = computedHashes[0]

				// Check if we have a cached SSDEEP for this cheap hash
				ssdeepKey := SSDeepCacheKey{cheapHash: cheapestHash, size: size}
				if cached, ok := resolver.ssdeepCache.Get(ssdeepKey); ok {
					ssdeepHashStr = cached.ssdeepHash
					foundInCache = true
					hashResolverTelemetry.hashCacheHit.Inc(eventType.String())
				}
			}

			// Compute SSDEEP if not found in cache
			if !foundInCache {
				// Seek back to the beginning of the file
				if _, err := f.Seek(0, 0); err != nil {
					hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileOpenError.String())
				} else {
					ssdeepHasher := newFastSSDeepHasher()
					limitedWriter := newSizeLimitedWriter(ssdeepHasher, int(maxFileSize))

					buffer := resolver.bufferPool.Get()
					_, err := io.CopyBuffer(limitedWriter, fileReader, *buffer)
					resolver.bufferPool.Put(buffer)

					if err != nil {
						if !errors.Is(err, ErrSizeLimitReached) {
							hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileOpenError.String())
						}
					} else {
						digest := ssdeepHasher.digest()
						if len(digest) > 0 {
							ssdeepHashStr = "ssdeep:" + digest
							hashResolverTelemetry.hashCount.Inc(eventType.String(), model.SSDEEP.String())
						} else {
							hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
						}
						if cheapestHash != "" {
							ssdeepKey := SSDeepCacheKey{cheapHash: cheapestHash, size: size}
							resolver.ssdeepCache.Add(ssdeepKey, &SSDeepCacheEntry{ssdeepHash: ssdeepHashStr})
						}
					}
				}
			} else {
				// Still count the cached ssdeep (but as a cache hit, already counted above)
				if ssdeepHashStr != "" {
					// Note: Already counted as cache hit, but also increment the hash count
					hashResolverTelemetry.hashCount.Inc(eventType.String(), model.SSDEEP.String())
				} else {
					hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
				}
			}

			// Store SSDEEP in the map
			if ssdeepHashStr != "" {
				computedHashes = append(computedHashes, ssdeepHashStr)
			}
		}
	}

	file.Hashes = computedHashes
	file.HashState = model.Done

	// cache entry with file metadata
	// This will either create a new entry or update an existing one for the same path
	if resolver.cache != nil {
		cacheEntry := &LRUCacheEntry{
			state:  model.Done,
			hashes: make([]string, len(file.Hashes)),
			inode:  entryInode,
			mtime:  mtime,
			size:   size,
		}
		copy(cacheEntry.hashes, file.Hashes)
		resolver.cache.Add(fileKey, cacheEntry)
	}
}

var hashResolverTelemetry = struct {
	hashCount    telemetry.Counter
	hashMiss     telemetry.Counter
	hashCacheHit telemetry.Counter
	cacheLen     telemetry.Gauge
}{
	hashCount:    metrics.NewITCounter(metrics.MetricHashResolverHashCount, []string{"event_type", "hash"}, "Number of hashes computed by the hash resolver"),
	hashMiss:     metrics.NewITCounter(metrics.MetricHashResolverHashMiss, []string{"event_type", "reason"}, "Number of hash misses by the hash resolver"),
	hashCacheHit: metrics.NewITCounter(metrics.MetricHashResolverHashCacheHit, []string{"event_type"}, "Number of hash cache hits by the hash resolver"),
	cacheLen:     metrics.NewITGauge(metrics.MetricHashResolverHashCacheLen, []string{}, "Number of entries in the hash resolver cache"),
}

// SendStats sends the resolver metrics
func (resolver *Resolver) SendStats() error {
	if !resolver.opts.Enabled {
		return nil
	}

	hashResolverTelemetry.cacheLen.Set(float64(resolver.cache.Len()))

	return nil
}
