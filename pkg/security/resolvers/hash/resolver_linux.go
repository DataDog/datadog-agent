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
	"os"

	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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

	cache *lru.Cache[model.PathKey, *LRUCacheEntry]

	// stats
	hashCount    map[model.EventType]map[model.HashAlgorithm]*atomic.Uint64
	hashMiss     map[model.EventType]map[model.HashState]*atomic.Uint64
	hashCacheHit map[model.EventType]*atomic.Uint64
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

	var cache *lru.Cache[model.PathKey, *LRUCacheEntry]
	if c.HashResolverCacheSize > 0 {
		var err error
		cache, err = lru.New[model.PathKey, *LRUCacheEntry](c.HashResolverCacheSize)
		if err != nil {
			return nil, fmt.Errorf("couldn't create hash resolver cache: %w", err)
		}
	}

	r := &Resolver{
		opts: ResolverOpts{
			Enabled:        true,
			MaxFileSize:    c.HashResolverMaxFileSize,
			HashAlgorithms: c.HashResolverHashAlgorithms,
			EventTypes:     c.HashResolverEventTypes,
		},
		cgroupResolver: cgroupResolver,
		statsdClient:   statsdClient,
		limiter:        rate.NewLimiter(rate.Limit(c.HashResolverMaxHashRate), c.HashResolverMaxHashBurst),
		cache:          cache,
		hashCount:      make(map[model.EventType]map[model.HashAlgorithm]*atomic.Uint64),
		hashMiss:       make(map[model.EventType]map[model.HashState]*atomic.Uint64),
		hashCacheHit:   make(map[model.EventType]*atomic.Uint64),
	}

	// generate counters
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		r.hashCount[i] = make(map[model.HashAlgorithm]*atomic.Uint64, model.MaxHashAlgorithm)
		for j := model.HashAlgorithm(0); j < model.MaxHashAlgorithm; j++ {
			r.hashCount[i][j] = atomic.NewUint64(0)
		}

		r.hashMiss[i] = make(map[model.HashState]*atomic.Uint64, model.MaxHashState)
		for j := model.HashState(0); j < model.MaxHashState; j++ {
			r.hashMiss[i][j] = atomic.NewUint64(0)
		}

		r.hashCacheHit[i] = atomic.NewUint64(0)
	}
	return r, nil
}

// ComputeHashesFromEvent calls ComputeHashes using the provided event
func (resolver *Resolver) ComputeHashesFromEvent(event *model.Event, file *model.FileEvent) []string {
	// resolve FileEvent
	event.FieldHandlers.ResolveFilePath(event, file)
	return resolver.ComputeHashes(event.GetEventType(), &event.ProcessContext.Process, file)
}

// ComputeHashes computes the hashes of the provided file event.
// Disclaimer: This resolver considers that the FileEvent has already been resolved
func (resolver *Resolver) ComputeHashes(eventType model.EventType, process *model.Process, file *model.FileEvent) []string {
	if !resolver.opts.Enabled {
		return nil
	}

	// check state
	if file.HashState == model.Done {
		return file.Hashes
	}
	if file.HashState != model.NoHash && file.HashState != model.HashWasRateLimited {
		// this file was already processed and an error occurred, nothing else to do
		return nil
	}

	// check if the resolver is allowed to hash this event type
	if !slices.Contains(resolver.opts.EventTypes, eventType) {
		file.HashState = model.EventTypeNotConfigured
		resolver.hashMiss[eventType][model.EventTypeNotConfigured].Inc()
		return nil
	}

	resolver.hash(eventType, process, file)
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
	default:
		return nil
	}
}

// hash hashes the provided file event
func (resolver *Resolver) hash(eventType model.EventType, process *model.Process, file *model.FileEvent) {
	if !file.IsPathnameStrResolved || len(file.PathnameStr) == 0 {
		resolver.hashMiss[eventType][model.PathnameResolutionError].Inc()
		file.HashState = model.PathnameResolutionError
		return
	}

	// check if the hash(es) of this file is in cache
	if resolver.cache != nil {
		cacheEntry, ok := resolver.cache.Get(file.PathKey)
		if ok {
			file.HashState = cacheEntry.state
			file.Hashes = cacheEntry.hashes
			resolver.hashCacheHit[eventType].Inc()
			return
		}
	}

	rootPIDs := []uint32{process.Pid}
	if resolver.cgroupResolver != nil {
		w, ok := resolver.cgroupResolver.GetWorkload(process.ContainerID)
		if ok {
			rootPIDs = w.GetPIDs()
		}
	}

	// open the target file
	var lastErr error
	var f *os.File
	for _, pidCandidate := range rootPIDs {
		f, lastErr = os.Open(utils.ProcRootFilePath(pidCandidate, file.PathnameStr))
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		if os.IsNotExist(lastErr) {
			file.HashState = model.FileNotFound
			resolver.hashMiss[eventType][model.FileNotFound].Inc()
			return
		}
		// We can't open this file, most likely because it isn't a regular file. Example seen in production:
		//  - open(/host/proc/2129077/root/tmp/plugin3037415914) => no such device or address
		//  - open(/host/proc/576833/root/run/containerd/runc/k8s.io/2b100...96104/runc.WUXTJB) => permission denied
		//  - open(/host/proc/313599/root/proc/10987/task/10988/status/10987/task) => not a directory
		//  - open(/host/proc/263082/root/usr/local/bin/runc) => no such process
		resolver.hashMiss[eventType][model.FileOpenError].Inc()
		file.HashState = model.FileOpenError
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		file.HashState = model.FileNotFound
		resolver.hashMiss[eventType][model.FileNotFound].Inc()
		return
	}

	// is this a regular file ?
	if !fi.Mode().IsRegular() {
		file.HashState = model.Done
		return
	}

	// is the file size above the configured limit
	if fi.Size() > resolver.opts.MaxFileSize {
		resolver.hashMiss[eventType][model.FileTooBig].Inc()
		file.HashState = model.FileTooBig
		return
	}

	// is the file empty ?
	if fi.Size() == 0 {
		resolver.hashMiss[eventType][model.FileEmpty].Inc()
		file.HashState = model.FileEmpty
		return
	}

	// check the rate limiter
	if !resolver.limiter.Allow() {
		// better luck next time
		resolver.hashMiss[eventType][model.HashWasRateLimited].Inc()
		file.HashState = model.HashWasRateLimited
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

	if _, err = io.Copy(multiWriter, f); err != nil {
		if errors.Is(err, ErrSizeLimitReached) {
			resolver.hashMiss[eventType][model.FileTooBig].Inc()
			file.HashState = model.FileTooBig
			return
		}
		// We can't read this file, most likely because it isn't a regular file (despite the check above). Example seen
		// in production:
		//  - read(/host/proc/2076/root/proc/1/fdinfo/64) => no such file or directory
		//  - read(/host/proc/2328/root/run/netns/a574a27c) => invalid argument
		resolver.hashMiss[eventType][model.FileOpenError].Inc()
		file.HashState = model.FileOpenError
		return
	}

	for i, algorithm := range resolver.opts.HashAlgorithms {
		var hashStr string
		if hashStr = algorithm.String(); len(hashStr) > 0 {
			hashStr += ":"
		}
		hashStr += hex.EncodeToString(hashers[i].(hash.Hash).Sum(nil))

		file.Hashes = append(file.Hashes, hashStr)
		resolver.hashCount[eventType][algorithm].Inc()
	}

	file.HashState = model.Done

	// cache entry
	if resolver.cache != nil {
		cacheEntry := &LRUCacheEntry{
			state:  model.Done,
			hashes: make([]string, len(file.Hashes)),
		}
		copy(cacheEntry.hashes, file.Hashes)
		resolver.cache.Add(file.PathKey, cacheEntry)
	}
}

// SendStats sends the resolver metrics
func (resolver *Resolver) SendStats() error {
	if !resolver.opts.Enabled {
		return nil
	}

	for evtType, hashCounts := range resolver.hashCount {
		for algorithm, count := range hashCounts {
			tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("hash:%s", algorithm)}
			if value := count.Swap(0); value > 0 {
				if err := resolver.statsdClient.Count(metrics.MetricHashResolverHashCount, int64(value), tags, 1.0); err != nil {
					return fmt.Errorf("couldn't send MetricHashResolverHashCount metric: %w", err)
				}
			}
		}
	}

	for evtType, hashMisses := range resolver.hashMiss {
		for reason, count := range hashMisses {
			tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("reason:%s", reason)}
			if value := count.Swap(0); value > 0 {
				if err := resolver.statsdClient.Count(metrics.MetricHashResolverHashMiss, int64(value), tags, 1.0); err != nil {
					return fmt.Errorf("couldn't send MetricHashResolverHashMiss metric: %w", err)
				}
			}
		}
	}

	for evtType, count := range resolver.hashCacheHit {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := count.Swap(0); value > 0 {
			if err := resolver.statsdClient.Count(metrics.MetricHashResolverHashCacheHit, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricHashResolverHashCacheHit metric: %w", err)
			}
		}
	}

	if resolver.cache != nil {
		if value := resolver.cache.Len(); value > 0 {
			if err := resolver.statsdClient.Gauge(metrics.MetricHashResolverHashCacheLen, float64(value), []string{}, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricHashResolverHashCacheLen metric: %w", err)
			}
		}
	}
	return nil
}
