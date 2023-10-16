// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package path holds path related files
package path

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path"
	"strings"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"
)

// ResolverInterface defines the resolver interface
type ResolverInterface interface {
	ResolveBasename(e *model.FileFields) string
	ResolveFileFieldsPath(e *model.FileFields, pidCtx *model.PIDContext, ctrCtx *model.ContainerContext) (string, error)
	SetMountRoot(ev *model.Event, e *model.Mount) error
	ResolveMountRoot(ev *model.Event, e *model.Mount) (string, error)
	SetMountPoint(ev *model.Event, e *model.Mount) error
	ResolveMountPoint(ev *model.Event, e *model.Mount) (string, error)
	SendStats() error
	Start(*manager.Manager) error
	Close() error
}

// NoResolver returns an empty resolver
type NoResolver struct {
}

// ResolveBasename resolves an inode/mount ID pair to a file basename
func (n *NoResolver) ResolveBasename(e *model.FileFields) string {
	return ""
}

// ResolveFileFieldsPath resolves an inode/mount ID pair to a full path
func (n *NoResolver) ResolveFileFieldsPath(e *model.FileFields, pidCtx *model.PIDContext, ctrCtx *model.ContainerContext) (string, error) {
	return "", nil
}

// SetMountRoot set the mount point information
func (n *NoResolver) SetMountRoot(ev *model.Event, e *model.Mount) error {
	return nil
}

// ResolveMountRoot resolves the mountpoint to a full path
func (n *NoResolver) ResolveMountRoot(ev *model.Event, e *model.Mount) (string, error) {
	return "", nil
}

// SetMountPoint set the mount point information
func (n *NoResolver) SetMountPoint(ev *model.Event, e *model.Mount) error {
	return nil
}

// ResolveMountPoint resolves the mountpoint to a full path
func (n *NoResolver) ResolveMountPoint(ev *model.Event, e *model.Mount) (string, error) {
	return "", nil
}

// SendStats does nothing for NoResolver
func (n *NoResolver) SendStats() error {
	return nil
}

// Start does nothing for NoResolver
func (n *NoResolver) Start(m *manager.Manager) error {
	return nil
}

// Close does nothing for NoResolver
func (n *NoResolver) Close() error {
	return nil
}

const (
	// PathRingBuffersSize is the size of each core ringbuffer
	PathRingBuffersSize = uint32(524288)
	// WatermarkSize is the number of bytes of a watermark value
	WatermarkSize = uint32(8)
)

// ResolverOpts defines mount resolver options
type ResolverOpts struct {
	PathCacheSize  uint
	UseRingBuffers bool
	UseERPC        bool
}

// Resolver describes a resolvers for path and file names
type Resolver struct {
	opts          ResolverOpts
	mountResolver *mount.Resolver
	statsdClient  statsd.ClientInterface
	// RingBuffers
	pathRings       []byte
	numCPU          uint32
	watermarkBuffer *bytes.Buffer
	failureCounters [maxPathResolutionFailureCause]*atomic.Int64
	successCounter  *atomic.Int64
	// Cache
	pathCache        *simplelru.LRU[model.DentryKey, string]
	cacheMissCounter *atomic.Int64
	cacheHitCounter  *atomic.Int64
	// eRPC
	erpc          *erpc.ERPC
	erpcBuffer    []byte
	erpcChallenge uint32
	erpcRequest   erpc.Request
}

// NewResolver returns a new path resolver
func NewResolver(opts ResolverOpts, mountResolver *mount.Resolver, eRPC *erpc.ERPC, statsdClient statsd.ClientInterface) (*Resolver, error) {
	pr := &Resolver{
		opts:          opts,
		mountResolver: mountResolver,
		statsdClient:  statsdClient,
		erpc:          eRPC,
	}

	if pr.opts.UseRingBuffers {
		for i := 0; i < int(maxPathResolutionFailureCause); i++ {
			pr.failureCounters[i] = atomic.NewInt64(0)
		}
		pr.successCounter = atomic.NewInt64(0)
		pr.watermarkBuffer = bytes.NewBuffer(make([]byte, 0, WatermarkSize))
	}

	if opts.PathCacheSize > 0 {
		pathCache, err := simplelru.NewLRU[model.DentryKey, string](int(opts.PathCacheSize), nil)
		if err != nil {
			return nil, fmt.Errorf("couldn't create path resolver: %w", err)
		}
		pr.pathCache = pathCache
		pr.cacheMissCounter = atomic.NewInt64(0)
		pr.cacheHitCounter = atomic.NewInt64(0)
	}

	return pr, nil
}

func reversePathParts(pathStr string) string {
	if pathStr == "/" {
		return pathStr
	}

	pathStr = strings.TrimSuffix(pathStr, "/")
	parts := strings.Split(pathStr, "/")

	if len(parts) == 0 {
		return "/"
	}

	var builder strings.Builder

	// pre-allocation
	for _, part := range parts {
		builder.Grow(len(part) + 1)
	}

	// reverse iteration
	for i := 0; i < len(parts); i++ {
		j := len(parts) - 1 - i
		builder.WriteRune('/')
		builder.WriteString(parts[j])
	}

	return builder.String()
}

// resolvePathFromRingBuffers resolves the path of the given path ringbuffer reference using ringbuffers
func (r *Resolver) resolvePathFromRingBuffers(ref *model.PathRingBufferRef) (string, error) {
	if ref.Length > PathRingBuffersSize {
		errCode := math.MaxUint32 - ref.Length
		switch errCode {
		case uint32(drUnknown):
			r.failureCounters[drUnknown].Inc()
			return "", errDrUnknown
		case uint32(drInvalidInode):
			r.failureCounters[drInvalidInode].Inc()
			return "", errDrInvalidInode
		case uint32(drDentryDiscarded):
			r.failureCounters[drDentryDiscarded].Inc()
			return "", errDrDentryDiscarded
		case uint32(drDentryResolution):
			r.failureCounters[drDentryResolution].Inc()
			return "", errDrDentryResolution
		case uint32(drDentryBadName):
			r.failureCounters[drDentryBadName].Inc()
			return "", errDrDentryBadName
		case uint32(drDentryMaxTailCall):
			r.failureCounters[drDentryMaxTailCall].Inc()
			return "", errDrDentryMaxTailCall
		default:
			r.failureCounters[pathRefLengthTooBig].Inc()
			return "", errPathRefLengthTooBig
		}
	}

	if ref.Length == 0 {
		r.failureCounters[pathRefLengthZero].Inc()
		return "", errPathRefLengthZero
	}

	if ref.Length <= 2*WatermarkSize {
		r.failureCounters[pathRefLengthTooSmall].Inc()
		return "", errPathRefLengthTooSmall
	}

	if ref.ReadCursor > PathRingBuffersSize {
		r.failureCounters[pathRefReadCursorOOB].Inc()
		return "", errPathRefReadCursorOOB
	}

	if ref.CPU >= uint32(r.numCPU) {
		r.failureCounters[pathRefInvalidCPU].Inc()
		return "", errPathRefInvalidCPU
	}

	cpuOffset := ref.CPU * PathRingBuffersSize
	readOffset := ref.ReadCursor

	r.watermarkBuffer.Reset()
	if readOffset+WatermarkSize > PathRingBuffersSize {
		if _, err := r.watermarkBuffer.Write(r.pathRings[cpuOffset+readOffset : cpuOffset+PathRingBuffersSize]); err != nil {
			r.failureCounters[pathRingsReadOverflow].Inc()
			return "", errPathRingsReadOverflow
		}
		remaining := WatermarkSize - (PathRingBuffersSize - readOffset)
		if _, err := r.watermarkBuffer.Write(r.pathRings[cpuOffset : cpuOffset+remaining]); err != nil {
			r.failureCounters[pathRingsReadOverflow].Inc()
			return "", errPathRingsReadOverflow
		}
		readOffset = remaining
	} else {
		if _, err := r.watermarkBuffer.Write(r.pathRings[cpuOffset+readOffset : cpuOffset+readOffset+WatermarkSize]); err != nil {
			r.failureCounters[pathRingsReadOverflow].Inc()
			return "", errPathRingsReadOverflow
		}
		readOffset += WatermarkSize
	}

	if r.watermarkBuffer.Len() != int(WatermarkSize) {
		r.failureCounters[invalidFrontWatermarkSize].Inc()
		return "", errInvalidFrontWatermarkSize
	}

	frontWatermark := model.ByteOrder.Uint64(r.watermarkBuffer.Bytes())
	if frontWatermark != ref.Watermark {
		r.failureCounters[frontWatermarkValueMismatch].Inc()
		return "", errFrontWatermarkValueMismatch
	}

	var pathStr string
	segmentLen := ref.Length - (2 * WatermarkSize)

	if readOffset+segmentLen > PathRingBuffersSize {
		firstPart := model.NullTerminatedString(r.pathRings[cpuOffset+readOffset : cpuOffset+PathRingBuffersSize])
		remaining := segmentLen - (PathRingBuffersSize - readOffset)
		secondPart := model.NullTerminatedString(r.pathRings[cpuOffset : cpuOffset+remaining])
		pathStr = firstPart + secondPart
		readOffset = remaining
	} else {
		pathStr = model.NullTerminatedString(r.pathRings[cpuOffset+readOffset : cpuOffset+readOffset+segmentLen])
		readOffset += segmentLen
	}

	r.watermarkBuffer.Reset()
	if readOffset+WatermarkSize > PathRingBuffersSize {
		if _, err := r.watermarkBuffer.Write(r.pathRings[cpuOffset+readOffset : cpuOffset+PathRingBuffersSize]); err != nil {
			r.failureCounters[pathRingsReadOverflow].Inc()
			return "", errPathRingsReadOverflow
		}
		remaining := WatermarkSize - (PathRingBuffersSize - readOffset)
		if _, err := r.watermarkBuffer.Write(r.pathRings[cpuOffset : cpuOffset+remaining]); err != nil {
			r.failureCounters[pathRingsReadOverflow].Inc()
			return "", errPathRingsReadOverflow
		}
	} else {
		if _, err := r.watermarkBuffer.Write(r.pathRings[cpuOffset+readOffset : cpuOffset+readOffset+WatermarkSize]); err != nil {
			r.failureCounters[pathRingsReadOverflow].Inc()
			return "", errPathRingsReadOverflow
		}
	}

	if r.watermarkBuffer.Len() != int(WatermarkSize) {
		r.failureCounters[invalidBackWatermarkSize].Inc()
		return "", errInvalidBackWatermarkSize
	}

	backWatermark := model.ByteOrder.Uint64(r.watermarkBuffer.Bytes())
	if backWatermark != ref.Watermark {
		r.failureCounters[backWatermarkValueMismatch].Inc()
		return "", errBackWatermarkValueMismatch
	}

	r.successCounter.Inc()

	return reversePathParts(pathStr), nil
}

// preventSegmentMajorPageFault prepares the userspace memory area where the dentry resolver response is written. Used in kernel versions where BPF_F_MMAPABLE array maps are not yet available.
func (r *Resolver) preventBufferMajorPageFault() {
	// if we don't access the buffer, the eBPF program can't write to it ... (major page fault)
	for i := 0; i < len(r.erpcBuffer); i += os.Getpagesize() {
		r.erpcBuffer[i] = 0
	}
}

// resolvePathFromERPC resolves the path of the given path ringbuffer reference using an eRPC call
func (r *Resolver) resolvePathFromERPC(ref *model.PathRingBufferRef) (string, error) {
	if 4+2*WatermarkSize+ref.Length > uint32(len(r.erpcBuffer)) {
		return "", fmt.Errorf("path ref is too big: %d bytes", ref.Length)
	}

	challenge := r.erpcChallenge
	r.erpcChallenge++

	// create eRPC request
	r.erpcRequest.OP = erpc.ResolvePathSegmentOp
	// 0-8 and 8-12 already populated at start
	model.ByteOrder.PutUint32(r.erpcRequest.Data[12:16], ref.CPU)
	model.ByteOrder.PutUint32(r.erpcRequest.Data[16:20], ref.ReadCursor)
	model.ByteOrder.PutUint32(r.erpcRequest.Data[20:24], ref.Length)
	model.ByteOrder.PutUint32(r.erpcRequest.Data[24:28], challenge)

	r.preventBufferMajorPageFault()

	err := r.erpc.Request(&r.erpcRequest)
	if err != nil {
		return "", fmt.Errorf("unable to get path from ref %+v with eRPC: %w", ref, err)
	}

	segmentLen := ref.Length - (2 * WatermarkSize)

	ackChallenge := model.ByteOrder.Uint32(r.erpcBuffer[0:4])
	if challenge != ackChallenge {
		return "", fmt.Errorf("invalid challenge (expected %d, got %d, ref %+v)", challenge, ackChallenge, ref)
	}

	frontWatermark := model.ByteOrder.Uint64(r.erpcBuffer[4:12])
	if frontWatermark != ref.Watermark {
		return "", fmt.Errorf("invalid front watermark (expected %d, got %d, challenge %d, ref %+v)", ref.Watermark, frontWatermark, challenge, ref)
	}

	backWatermark := model.ByteOrder.Uint64(r.erpcBuffer[12+segmentLen : 12+segmentLen+8])
	if backWatermark != ref.Watermark {
		return "", fmt.Errorf("invalid back watermark (expected %d, got %d, challenge %d, ref %+v)", ref.Watermark, backWatermark, challenge, ref)
	}

	path := model.NullTerminatedString(r.erpcBuffer[12 : 12+segmentLen])
	if len(path) == 0 || len(path) > 0 && path[0] == 0 {
		return "", fmt.Errorf("couldn't resolve path (len: %d)", len(path))
	}

	return reversePathParts(path), nil
}

// resolvePathFromCache tries to resolve a cached path from the provided path key
func (r *Resolver) resolvePathFromCache(key *model.DentryKey) string {
	path, ok := r.pathCache.Get(*key)
	if !ok {
		r.cacheMissCounter.Inc()
		return ""
	}

	r.cacheHitCounter.Inc()
	return path
}

func (r *Resolver) resolvePath(ref *model.PathRingBufferRef, key *model.DentryKey, cache bool) (string, error) {
	var path string
	var err error

	if r.opts.UseRingBuffers {
		path, err = r.resolvePathFromRingBuffers(ref)
		if err == nil {
			if r.opts.PathCacheSize > 0 && cache {
				r.pathCache.Add(*key, path)
			}
			return path, nil
		}
	}

	if r.opts.PathCacheSize > 0 {
		path = r.resolvePathFromCache(key)
		if len(path) > 0 {
			return path, nil
		}
	}

	if r.opts.UseERPC {
		path, err = r.resolvePathFromERPC(ref)
		if err == nil {
			if r.opts.PathCacheSize > 0 && cache {
				r.pathCache.Add(*key, path)
			}
			return path, nil
		}
	}

	return path, err
}

// ResolveBasename resolves a path ringbuffer reference or an inode/mount ID pair to a file basename
func (r *Resolver) ResolveBasename(e *model.FileFields) string {
	resolvedPath, err := r.resolvePath(&e.PathRef, &e.DentryKey, !e.HasHardLinks())
	if err != nil {
		return ""
	}
	return path.Base(resolvedPath)
}

// ResolveFileFieldsPath resolves a path ringbuffer reference or an inode/mount ID pair to a full path
func (r *Resolver) ResolveFileFieldsPath(e *model.FileFields, pidCtx *model.PIDContext, ctrCtx *model.ContainerContext) (string, error) {
	pathStr, err := r.resolvePath(&e.PathRef, &e.DentryKey, !e.HasHardLinks())
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: err}
		}
		return pathStr, &ErrPathResolution{Err: err}
	}

	if e.IsFileless() {
		return pathStr, nil
	}

	mountPath, err := r.mountResolver.ResolveMountPath(e.MountID, e.Device, pidCtx.Pid, ctrCtx.ID)
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, &ErrPathResolution{Err: err}
	}

	rootPath, err := r.mountResolver.ResolveMountRoot(e.MountID, e.Device, pidCtx.Pid, ctrCtx.ID)
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, &ErrPathResolution{Err: err}
	}

	// This aims to handle bind mounts
	if strings.HasPrefix(pathStr, rootPath) && rootPath != "/" {
		pathStr = strings.Replace(pathStr, rootPath, "", 1)
	}

	if mountPath != "/" {
		pathStr = mountPath + pathStr
	}

	return pathStr, nil
}

// SetMountRoot sets the root path of the mountpoint
func (r *Resolver) SetMountRoot(ev *model.Event, e *model.Mount) error {
	var err error
	e.RootStr, err = r.resolvePath(&e.RootStrPathRef, &e.RootDentryKey, true)
	if err != nil {
		return &ErrPathResolutionNotCritical{Err: err}
	}
	return nil
}

// ResolveMountRoot resolves the root path of a mountpoint
func (r *Resolver) ResolveMountRoot(ev *model.Event, e *model.Mount) (string, error) {
	if len(e.RootStr) == 0 {
		if err := r.SetMountRoot(ev, e); err != nil {
			return "", err
		}
	}
	return e.RootStr, nil
}

// SetMountPoint set the mount point information
func (r *Resolver) SetMountPoint(ev *model.Event, e *model.Mount) error {
	var err error
	e.MountPointStr, err = r.resolvePath(&e.MountPointPathRef, &e.ParentDentryKey, true)
	if err != nil {
		return &ErrPathResolutionNotCritical{Err: err}
	}
	return nil
}

// ResolveMountPoint resolves the mountpoint to a full path
func (r *Resolver) ResolveMountPoint(ev *model.Event, e *model.Mount) (string, error) {
	if len(e.MountPointStr) == 0 {
		if err := r.SetMountPoint(ev, e); err != nil {
			return "", err
		}
	}
	return e.MountPointStr, nil
}

// Start starts the path resolver
func (r *Resolver) Start(m *manager.Manager) error {
	if r.pathRings != nil {
		return fmt.Errorf("path resolver already started")
	}

	numCPU, err := utils.NumCPU()
	if err != nil {
		return err
	}
	r.numCPU = uint32(numCPU)

	if r.opts.UseRingBuffers {
		pathRingsMap, err := managerhelper.Map(m, "dr_ringbufs")
		if err != nil {
			return err
		}

		pathRings, err := syscall.Mmap(pathRingsMap.FD(), 0, int(r.numCPU*PathRingBuffersSize), unix.PROT_READ, unix.MAP_SHARED)
		if err != nil || pathRings == nil {
			return fmt.Errorf("failed to mmap dr_ringbufs map: %w", err)
		}
		r.pathRings = pathRings
	}

	if r.opts.UseERPC {
		r.erpcBuffer = make([]byte, 7*os.Getpagesize())
		r.erpcChallenge = rand.Uint32()
		model.ByteOrder.PutUint64(r.erpcRequest.Data[0:8], uint64(uintptr(unsafe.Pointer(&r.erpcBuffer[0]))))
		model.ByteOrder.PutUint32(r.erpcRequest.Data[8:12], uint32(len(r.erpcBuffer)))
	}

	return nil
}

// Close unmaps the the ringbuffers map
func (r *Resolver) Close() error {
	if r.opts.UseRingBuffers {
		return unix.Munmap(r.pathRings)
	}
	return nil
}

// SendStats sends the path resolver metrics
func (r *Resolver) SendStats() error {
	if r.opts.UseRingBuffers {
		for cause, counter := range r.failureCounters {
			val := counter.Swap(0)
			if val > 0 {
				tags := []string{fmt.Sprintf("cause:%s", pathRingsResolutionFailureCause(cause).String())}
				_ = r.statsdClient.Count(metrics.MetricPathResolutionFailure, val, tags, 1.0)
			}
		}

		val := r.successCounter.Swap(0)
		if val > 0 {
			_ = r.statsdClient.Count(metrics.MetricPathResolutionSuccess, val, []string{}, 1.0)
		}
	}

	if r.opts.PathCacheSize > 0 {
		var val int64
		val = r.cacheMissCounter.Swap(0)
		if val > 0 {
			_ = r.statsdClient.Count(metrics.MetricPathResolutionCacheMiss, val, []string{}, 1.0)
		}
		val = r.cacheHitCounter.Swap(0)
		if val > 0 {
			_ = r.statsdClient.Count(metrics.MetricPathResolutionCacheHit, val, []string{}, 1.0)
		}
	}

	return nil
}
