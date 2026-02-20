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
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/glaslos/ssdeep"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

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
	path     string
	cgroupID string
}

// LRUCacheEntry is the structure used to cache hashes
// It includes file metadata (inode, mtime, size) to detect if the file has changed
type LRUCacheEntry struct {
	state  model.HashState
	hashes []string
	// File metadata to detect changes
	inode  uint64 // file inode
	pathID uint32 // internal path ID
	mtime  uint64 // modification time in nanoseconds
	size   int64  // file size in bytes
}

// SSDeepCacheKey is the key used for caching ssdeep hashes based on cheaper hashes
type SSDeepCacheKey struct {
	inode     uint64 // file inode
	size      int64  // file size in bytes
	cheapHash string // the cheapest hash used as cache key
}

// SSDeepCacheEntry stores the cached ssdeep hash
type SSDeepCacheEntry struct {
	ssdeepHash string
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
}

// NewResolver returns a new instance of the hash resolver
func NewResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, cgroupResolver *cgroup.Resolver) (*Resolver, error) {
	if !c.HashResolverEnabled {
		return &Resolver{}, nil
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
		if slices.Contains(c.HashResolverHashAlgorithms, model.SSDEEP) {
			ssdeepCache, err = lru.New[SSDeepCacheKey, *SSDeepCacheEntry](c.HashResolverCacheSize)
			if err != nil {
				return nil, fmt.Errorf("couldn't create ssdeep cache: %w", err)
			}
		}
	}

	burst := 1
	// if the rate limiter is disabled, set the burst to 0
	if c.HashResolverMaxHashRate == 0 {
		burst = 0
	}

	// size of the buffer used to copy data from the file to the hash functions
	const copyBufferSize = 32 * 1024

	r := &Resolver{
		opts: ResolverOpts{
			Enabled:        true,
			MaxFileSize:    c.HashResolverMaxFileSize,
			HashAlgorithms: sortAlgorithmsByCost(c.HashResolverHashAlgorithms),
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
		return ssdeep.New()
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

	// check state
	if file.HashState == model.Done {
		return
	}
	if file.HashState != model.NoHash && file.HashState != model.HashWasRateLimited {
		// this file was already processed and an error occurred, nothing else to do
		return
	}

	// check if the resolver is allowed to hash this event type
	if !slices.Contains(resolver.opts.EventTypes, eventType) {
		file.HashState = model.EventTypeNotConfigured
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.EventTypeNotConfigured.String())
		return
	}

	if !file.IsPathnameStrResolved || len(file.PathnameStr) == 0 {
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.PathnameResolutionError.String())
		file.HashState = model.PathnameResolutionError
		return
	}

	if hashStr, exists := resolver.replace[file.PathnameStr]; exists {
		file.Hashes = append(file.Hashes, hashStr)
		file.HashState = model.Done
		return
	}

	// check if the hash(es) of this file is in cache
	fileKey := LRUCacheKey{
		path:     file.PathnameStr,
		cgroupID: string(cgroupID),
	}

	// Note: we'll check after stat if the cached entry matches the file metadata
	// This is done later after we get the actual file stats

	// check the rate limiter
	rateReservation := resolver.limiter.Reserve()
	if !rateReservation.OK() {
		// better luck next time
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashWasRateLimited.String())
		file.HashState = model.HashWasRateLimited
		return
	}

	// add pid one for hash resolution outside of a container
	rootPIDs := []uint32{1, pid}
	if resolver.cgroupResolver != nil {
		if cacheEntry := resolver.cgroupResolver.GetCacheEntryByCgroupID(cgroupID); cacheEntry != nil {
			rootPIDs = cacheEntry.GetPIDs()
		}
	}

	// open the target file
	var (
		lastErr     error
		f           *os.File
		mode        fs.FileMode
		size        int64
		mtime       uint64
		fkey        fileUniqKey
		failedCache = make(map[fileUniqKey]struct{})
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
		rateReservation.Cancel()
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
		rateReservation.Cancel()
		file.HashState = model.FileNotFound
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileNotFound.String())
		return
	}
	defer f.Close()

	if maxFileSize <= 0 {
		maxFileSize = resolver.opts.MaxFileSize
	}

	// is the file size above the configured limit
	if size > maxFileSize {
		rateReservation.Cancel()
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileTooBig.String())
		file.HashState = model.FileTooBig
		return
	}

	// is the file empty ?
	if size == 0 {
		rateReservation.Cancel()
		hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileEmpty.String())
		file.HashState = model.FileEmpty
		return
	}

	// Now that we have the file stats, check if we have a cached entry
	// and if it matches the current file metadata
	if resolver.cache != nil {
		cacheEntry, ok := resolver.cache.Get(fileKey)
		if ok {
			// Check if the cached entry matches the current file metadata
			if cacheEntry.inode == file.Inode &&
				cacheEntry.pathID == file.PathKey.PathID &&
				cacheEntry.mtime == mtime &&
				cacheEntry.size == size {
				// Cache hit: file hasn't changed
				file.HashState = cacheEntry.state
				file.Hashes = cacheEntry.hashes
				hashResolverTelemetry.hashCacheHit.Inc(eventType.String())
				rateReservation.Cancel()
				return
			}
			// Cache entry exists but file has changed, we'll recompute and update the entry
		}
	}

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
		multiWriter := newSizeLimitedWriter(io.MultiWriter(hashers...), int(resolver.opts.MaxFileSize))

		buffer := resolver.bufferPool.Get()
		_, err := io.CopyBuffer(multiWriter, f, *buffer)
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
		var (
			cheapestHash  string
			ssdeepHashStr string
			foundInCache  bool
		)

		if len(computedHashes) > 0 {
			// Find the cheapest computed hash to use as cache key
			cheapestHash = computedHashes[0]

			// Check if we have a cached SSDEEP for this cheap hash
			ssdeepKey := SSDeepCacheKey{cheapHash: cheapestHash, inode: file.Inode, size: size}
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
				ssdeepHasher := ssdeep.New()
				limitedWriter := newSizeLimitedWriter(ssdeepHasher, int(resolver.opts.MaxFileSize))

				buffer := resolver.bufferPool.Get()
				_, err := io.CopyBuffer(limitedWriter, f, *buffer)
				resolver.bufferPool.Put(buffer)

				if err != nil {
					if !errors.Is(err, ErrSizeLimitReached) {
						hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.FileOpenError.String())
					}
				} else {
					digest := ssdeepHasher.Sum(nil)
					if len(digest) > 0 {
						var hashStr strings.Builder
						hashStr.WriteString("ssdeep:")
						hashStr.Write(digest)
						ssdeepHashStr = hashStr.String()

						// Cache the SSDEEP hash with the cheapest hash as key
						if cheapestHash != "" {
							ssdeepKey := SSDeepCacheKey{cheapHash: cheapestHash, inode: file.Inode, size: size}
							resolver.ssdeepCache.Add(ssdeepKey, &SSDeepCacheEntry{ssdeepHash: ssdeepHashStr})
						}

						hashResolverTelemetry.hashCount.Inc(eventType.String(), model.SSDEEP.String())
					} else {
						hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
					}
				}
			}
		} else {
			// Still count the cached ssdeep (but as a cache hit, already counted above)
			if ssdeepHashStr != "" {
				// Note: Already counted as cache hit, but also increment the hash count
				hashResolverTelemetry.hashCount.Inc(eventType.String(), model.SSDEEP.String())
			}
		}

		// Store SSDEEP in the map
		if ssdeepHashStr != "" {
			computedHashes = append(computedHashes, ssdeepHashStr)
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
			inode:  file.Inode,
			pathID: file.PathKey.PathID,
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
