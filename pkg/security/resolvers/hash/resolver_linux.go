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
	ErrSizeLimitReached = fmt.Errorf("size limit reached")
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
	path        string
	containerID string
	inode       uint64
	pathID      uint32
}

// LRUCacheEntry is the structure used to cache hashes
type LRUCacheEntry struct {
	state  model.HashState
	hashes []string
}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
	opts           ResolverOpts
	statsdClient   statsd.ClientInterface
	limiter        *rate.Limiter
	cgroupResolver *cgroup.Resolver
	replace        map[string]string

	cache *lru.Cache[LRUCacheKey, *LRUCacheEntry]

	bufferPool *ddsync.TypedPool[[]byte]
}

// NewResolver returns a new instance of the hash resolver
func NewResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, cgroupResolver *cgroup.Resolver) (*Resolver, error) {
	if !c.HashResolverEnabled {
		return &Resolver{
			opts: ResolverOpts{
				Enabled: false,
			},
		}, nil
	}

	var cache *lru.Cache[LRUCacheKey, *LRUCacheEntry]
	if c.HashResolverCacheSize > 0 {
		var err error
		cache, err = lru.New[LRUCacheKey, *LRUCacheEntry](c.HashResolverCacheSize)
		if err != nil {
			return nil, fmt.Errorf("couldn't create hash resolver cache: %w", err)
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
			HashAlgorithms: c.HashResolverHashAlgorithms,
			EventTypes:     c.HashResolverEventTypes,
		},
		cgroupResolver: cgroupResolver,
		statsdClient:   statsdClient,
		limiter:        rate.NewLimiter(rate.Limit(c.HashResolverMaxHashRate), burst),
		cache:          cache,
		bufferPool:     ddsync.NewSlicePool[byte](copyBufferSize, copyBufferSize),
		replace:        c.HashResolverReplace,
	}

	return r, nil
}

// ComputeHashesFromEvent calls ComputeHashes using the provided event
func (resolver *Resolver) ComputeHashesFromEvent(event *model.Event, file *model.FileEvent) []string {
	if !resolver.opts.Enabled {
		return nil
	}

	// resolve FileEvent
	event.FieldHandlers.ResolveFilePath(event, file)

	process := event.ProcessContext.Process
	resolver.HashFileEvent(event.GetEventType(), process.ContainerID, process.Pid, file)

	return file.Hashes
}

// ComputeHashes computes the hashes of the provided file event.
// Disclaimer: This resolver considers that the FileEvent has already been resolved
func (resolver *Resolver) ComputeHashes(eventType model.EventType, process *model.Process, file *model.FileEvent) []string {
	if !resolver.opts.Enabled {
		return nil
	}

	resolver.HashFileEvent(eventType, process.ContainerID, process.Pid, file)

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

type fileUniqKey struct {
	dev   uint64
	inode uint64
}

func getFileInfo(path string) (fs.FileMode, int64, fileUniqKey, error) {
	stat, err := utils.UnixStat(path)
	if err != nil {
		return 0, 0, fileUniqKey{}, err
	}

	fkey := fileUniqKey{
		dev:   stat.Dev,
		inode: stat.Ino,
	}

	return utils.UnixStatModeToGoFileMode(stat.Mode), stat.Size, fkey, nil
}

// HashFileEvent hashes the provided file event
func (resolver *Resolver) HashFileEvent(eventType model.EventType, ctrID containerutils.ContainerID, pid uint32, file *model.FileEvent) {
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
		path:        file.PathnameStr,
		containerID: string(ctrID),
		inode:       file.Inode,
		pathID:      file.PathKey.PathID,
	}
	if resolver.cache != nil {
		cacheEntry, ok := resolver.cache.Get(fileKey)
		if ok {
			file.HashState = cacheEntry.state
			file.Hashes = cacheEntry.hashes
			hashResolverTelemetry.hashCacheHit.Inc(eventType.String())
			return
		}
	}

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
		w, ok := resolver.cgroupResolver.GetWorkload(ctrID)
		if ok {
			rootPIDs = w.GetPIDs()
		}
	}

	// open the target file
	var (
		lastErr     error
		f           *os.File
		mode        fs.FileMode
		size        int64
		fkey        fileUniqKey
		failedCache = make(map[fileUniqKey]struct{})
	)
	for _, pidCandidate := range rootPIDs {
		path := utils.ProcRootFilePath(pidCandidate, file.PathnameStr)
		if mode, size, fkey, lastErr = getFileInfo(path); !mode.IsRegular() {
			continue
		}

		if _, ok := failedCache[fkey]; ok {
			// we already tried to open this file and failed, no need to try again
			continue
		}

		f, lastErr = os.Open(path)
		if lastErr == nil {
			break
		}
		failedCache[fkey] = struct{}{}
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

	// is the file size above the configured limit
	if size > resolver.opts.MaxFileSize {
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

	var hashers []io.Writer
	for _, algorithm := range resolver.opts.HashAlgorithms {
		h := resolver.getHashFunction(algorithm)
		if h == nil {
			// shouldn't happen, ignore
			continue
		}
		hashers = append(hashers, h)
	}
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

	for i, algorithm := range resolver.opts.HashAlgorithms {
		var hashStr strings.Builder
		hashStr.WriteString(algorithm.String())
		if hashStr.Len() > 0 {
			hashStr.WriteByte(':')
		}
		digest := hashers[i].(hash.Hash).Sum(nil)
		if algorithm == model.SSDEEP {
			if len(digest) == 0 {
				// we failed to compute the digest
				hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
				continue
			}
			hashStr.Write(digest)
		} else {
			hencoder := hex.NewEncoder(&hashStr)
			if _, err := hencoder.Write(digest); err != nil {
				// we failed to compute the digest
				hashResolverTelemetry.hashMiss.Inc(eventType.String(), model.HashFailed.String())
				continue
			}
		}

		file.Hashes = append(file.Hashes, hashStr.String())
		hashResolverTelemetry.hashCount.Inc(eventType.String(), algorithm.String())
	}

	file.HashState = model.Done

	// cache entry
	if resolver.cache != nil {
		cacheEntry := &LRUCacheEntry{
			state:  model.Done,
			hashes: make([]string, len(file.Hashes)),
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
