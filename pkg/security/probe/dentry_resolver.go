// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"C"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"

	lib "github.com/DataDog/ebpf"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)
import "github.com/DataDog/datadog-agent/pkg/security/model"

const (
	dentryPathKeyNotFound = "error: dentry path key not found"
	fakeInodeMSW          = 0xdeadc001
)

// DentryResolver resolves inode/mountID to full paths
type DentryResolver struct {
	probe           *Probe
	pathnames       *lib.Map
	cache           map[uint32]*lru.Cache
	erpc            *ERPC
	erpcSegment     []byte
	erpcSegmentSize int
	erpcRequest     ERPCRequest
	erpcEnabled     bool
	mapEnabled      bool
}

// ErrInvalidKeyPath is returned when inode or mountid are not valid
type ErrInvalidKeyPath struct {
	Inode   uint64
	MountID uint32
}

func (e *ErrInvalidKeyPath) Error() string {
	return fmt.Sprintf("invalid inode/mountID couple: %d/%d", e.Inode, e.MountID)
}

// ErrEntryNotFound is thrown when a path key was not found in the cache
var ErrEntryNotFound = errors.New("entry not found")

// PathKey identifies an entry in the dentry cache
type PathKey struct {
	Inode   uint64
	MountID uint32
	PathID  uint32
}

func (p *PathKey) Write(buffer []byte) {
	model.ByteOrder.PutUint64(buffer[0:8], p.Inode)
	model.ByteOrder.PutUint32(buffer[8:12], p.MountID)
	model.ByteOrder.PutUint32(buffer[12:16], p.PathID)
}

// IsNull returns true if a key is invalid
func (p *PathKey) IsNull() bool {
	return p.Inode == 0 && p.MountID == 0
}

func (p *PathKey) String() string {
	return fmt.Sprintf("%x/%x", p.MountID, p.Inode)
}

// MarshalBinary returns the binary representation of a path key
func (p *PathKey) MarshalBinary() ([]byte, error) {
	if p.IsNull() {
		return nil, &ErrInvalidKeyPath{Inode: p.Inode, MountID: p.MountID}
	}

	return make([]byte, 16), nil
}

// PathValue describes a value of an entry of the cache
type PathValue struct {
	Parent PathKey
	Name   [model.MaxSegmentLength + 1]byte
	Len    uint16
}

// DelCacheEntry removes an entry from the cache
func (dr *DentryResolver) DelCacheEntry(mountID uint32, inode uint64) {
	if entries, exists := dr.cache[mountID]; exists {
		key := PathKey{Inode: inode}

		// Delete path recursively
		for {
			path, exists := entries.Get(key.Inode)
			if !exists {
				break
			}
			entries.Remove(key.Inode)

			parent := path.(PathValue).Parent
			if parent.Inode == 0 {
				break
			}

			// Prepare next key
			key = parent
		}
	}
}

// DelCacheEntries removes all the entries belonging to a mountID
func (dr *DentryResolver) DelCacheEntries(mountID uint32) {
	delete(dr.cache, mountID)
}

func (dr *DentryResolver) lookupInode(mountID uint32, inode uint64) (pathValue PathValue, err error) {
	entries, exists := dr.cache[mountID]
	if !exists {
		return pathValue, ErrEntryNotFound
	}

	entry, exists := entries.Get(inode)
	if !exists {
		return pathValue, ErrEntryNotFound
	}

	return entry.(PathValue), nil
}

func (dr *DentryResolver) cacheInode(mountID uint32, inode uint64, pathValue PathValue) error {
	entries, exists := dr.cache[mountID]
	if !exists {
		var err error

		entries, err = lru.New(128)
		if err != nil {
			return err
		}
		dr.cache[mountID] = entries
	}

	entries.Add(inode, pathValue)

	return nil
}

func (dr *DentryResolver) getNameFromCache(mountID uint32, inode uint64) (name string, err error) {
	path, err := dr.lookupInode(mountID, inode)
	if err != nil {
		return "", err
	}

	return C.GoString((*C.char)(unsafe.Pointer(&path.Name))), nil
}

// GetNameFromMap resolves the name of the provided inode
func (dr *DentryResolver) GetNameFromMap(mountID uint32, inode uint64, pathID uint32) (name string, err error) {
	key := PathKey{MountID: mountID, Inode: inode, PathID: pathID}
	var path PathValue

	if err := dr.pathnames.Lookup(key, &path); err != nil {
		return "", fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`", mountID, inode)
	}

	return C.GoString((*C.char)(unsafe.Pointer(&path.Name))), nil
}

// GetName resolves a couple of mountID/inode to a path
func (dr *DentryResolver) GetName(mountID uint32, inode uint64, pathID uint32) string {
	name, err := dr.getNameFromCache(mountID, inode)
	if err != nil && dr.erpcEnabled {
		name, err = dr.GetNameFromERPC(mountID, inode, pathID)
	}
	if err != nil && dr.mapEnabled {
		name, _ = dr.GetNameFromMap(mountID, inode, pathID)
	}
	return name
}

// ResolveFromCache resolve from the cache
func (dr *DentryResolver) ResolveFromCache(mountID uint32, inode uint64) (filename string, err error) {
	key := PathKey{MountID: mountID, Inode: inode}

	// Fetch path recursively
	for i := 0; i <= model.MaxPathDepth; i++ {
		path, err := dr.lookupInode(key.MountID, key.Inode)
		if err != nil {
			return "", err
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] != '\x00' && path.Name[0] != '/' {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.Name))) + filename
		}

		if path.Parent.Inode == 0 {
			break
		}

		// Prepare next key
		key = path.Parent
	}

	if len(filename) == 0 {
		filename = "/"
	}

	return
}

// ResolveFromMap resolves the path of the provided inode / mount id / path id
func (dr *DentryResolver) ResolveFromMap(mountID uint32, inode uint64, pathID uint32) (string, error) {
	key := PathKey{MountID: mountID, Inode: inode, PathID: pathID}
	var path PathValue
	var filename, segment string
	var err, resolutionErr error

	keyBuffer, err := key.MarshalBinary()
	if err != nil {
		return "", err
	}

	toAdd := make(map[PathKey]PathValue)

	// Fetch path recursively
	for i := 0; i <= model.MaxPathDepth; i++ {
		key.Write(keyBuffer)
		if err = dr.pathnames.Lookup(keyBuffer, &path); err != nil {
			filename = dentryPathKeyNotFound
			break
		}

		cacheKey := PathKey{MountID: key.MountID, Inode: key.Inode}
		toAdd[cacheKey] = path

		if path.Name[0] == '\x00' {
			resolutionErr = errTruncatedParents
			break
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] != '/' {
			segment = C.GoString((*C.char)(unsafe.Pointer(&path.Name)))
			filename = "/" + segment + filename
		}

		if path.Parent.Inode == 0 {
			break
		}

		// Prepare next key
		key = path.Parent
	}

	// resolution errors are more important than regular map lookup errors
	if resolutionErr != nil {
		err = resolutionErr
	}

	if len(filename) == 0 {
		filename = "/"
	}

	if err == nil {
		for k, v := range toAdd {
			// do not cache fake path keys in the case of rename events
			if k.Inode>>32 != fakeInodeMSW {
				_ = dr.cacheInode(k.MountID, k.Inode, v)
			}
		}
	}

	return filename, err
}

func (dr *DentryResolver) preventSegmentMajorPageFault() {
	// if we don't access the segment, the eBPF program can't write to it ... (major page fault)
	dr.erpcSegment[0] = 0
	dr.erpcSegment[os.Getpagesize()] = 0
	dr.erpcSegment[2*os.Getpagesize()] = 0
	dr.erpcSegment[3*os.Getpagesize()] = 0
	dr.erpcSegment[4*os.Getpagesize()] = 0
	dr.erpcSegment[5*os.Getpagesize()] = 0
	dr.erpcSegment[6*os.Getpagesize()] = 0
}

// GetNameFromERPC resolves the name of the provided inode / mount id / path id
func (dr *DentryResolver) GetNameFromERPC(mountID uint32, inode uint64, pathID uint32) (name string, err error) {
	// create eRPC request
	dr.erpcRequest.OP = ResolveSegmentOp
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[0:8], inode)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[8:12], mountID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[12:16], pathID)
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[16:24], uint64(uintptr(unsafe.Pointer(&dr.erpcSegment[0]))))
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[24:28], uint32(dr.erpcSegmentSize))

	// if we don't try to access the segment, the eBPF program can't write to it ... (major page fault)
	dr.preventSegmentMajorPageFault()

	if err = dr.erpc.Request(&dr.erpcRequest); err != nil {
		return "", errors.Wrapf(err, "unable to get filename for mountID `%d` and inode `%d` with eRPC", mountID, inode)
	}

	seg := C.GoString((*C.char)(unsafe.Pointer(&dr.erpcSegment[16])))
	if len(seg) == 0 || len(seg) > 0 && seg[0] == 0 {
		return "", errors.Errorf("couldn't resolve segment (len: %d)", len(seg))
	}
	return seg, nil
}

// ResolveFromERPC resolves the path of the provided inode / mount id / path id
func (dr *DentryResolver) ResolveFromERPC(mountID uint32, inode uint64, pathID uint32) (string, error) {
	var filename, segment string
	var err, resolutionErr error
	var key PathKey
	var val PathValue

	// create eRPC request
	dr.erpcRequest.OP = ResolvePathOp
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[0:8], inode)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[8:12], mountID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[12:16], pathID)
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[16:24], uint64(uintptr(unsafe.Pointer(&dr.erpcSegment[0]))))
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[24:28], uint32(dr.erpcSegmentSize))

	// if we don't try to access the segment, the eBPF program can't write to it ... (major page fault)
	dr.preventSegmentMajorPageFault()

	if err = dr.erpc.Request(&dr.erpcRequest); err != nil {
		return "", err
	}

	var keys []PathKey
	var segments []string

	i := 0
	depth := 0
	// make sure that we keep room for at least one pathID + character + \0 => (sizeof(pathID) + 1 = 17)
	for i < dr.erpcSegmentSize-17 {
		depth++

		// parse the path_key_t structure
		key.Inode = model.ByteOrder.Uint64(dr.erpcSegment[i : i+8])
		key.MountID = model.ByteOrder.Uint32(dr.erpcSegment[i+8 : i+12])
		// skip pathID
		i += 16

		if dr.erpcSegment[i] == 0 {
			if depth >= model.MaxPathDepth {
				resolutionErr = errTruncatedParents
			} else {
				resolutionErr = errERPCResolution
			}
			break
		}

		if dr.erpcSegment[i] != '/' {
			segment = C.GoString((*C.char)(unsafe.Pointer(&dr.erpcSegment[i])))
			filename = "/" + segment + filename
			i += len(segment) + 1
		} else {
			break
		}

		keys = append(keys, key)
		segments = append(segments, segment)
	}

	if len(filename) == 0 {
		filename = "/"
	}

	if resolutionErr == nil {
		var seg string
		for i, k := range keys {
			if k.Inode>>32 == fakeInodeMSW {
				continue
			}

			if len(keys) > i+1 {
				val.Parent = keys[i+1]
			} else {
				val.Parent = PathKey{}
			}
			seg = segments[i]
			copy(val.Name[:], seg[:])
			val.Name[len(seg)] = 0

			_ = dr.cacheInode(k.MountID, k.Inode, val)
		}
	}

	if filename[0] == 0 {
		return "", errors.Errorf("couldn't resolve path (len: %d)", len(filename))
	}

	return filename, resolutionErr
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(mountID uint32, inode uint64, pathID uint32) (string, error) {
	path, err := dr.ResolveFromCache(mountID, inode)
	if err != nil && dr.erpcEnabled {
		path, err = dr.ResolveFromERPC(mountID, inode, pathID)
	}
	if err != nil && err != errTruncatedParents && dr.mapEnabled {
		path, err = dr.ResolveFromMap(mountID, inode, pathID)
	}
	return path, err
}

func (dr *DentryResolver) getParentFromCache(mountID uint32, inode uint64) (uint32, uint64, error) {
	path, err := dr.lookupInode(mountID, inode)
	if err != nil {
		return 0, 0, ErrEntryNotFound
	}

	return path.Parent.MountID, path.Parent.Inode, nil
}

func (dr *DentryResolver) getParentFromMap(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	key := PathKey{MountID: mountID, Inode: inode, PathID: pathID}
	var path PathValue

	if err := dr.pathnames.Lookup(key, &path); err != nil {
		return 0, 0, err
	}

	return path.Parent.MountID, path.Parent.Inode, nil
}

// GetParent - Return the parent mount_id/inode
func (dr *DentryResolver) GetParent(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	parentMountID, parentInode, err := dr.getParentFromCache(mountID, inode)
	if err != nil {
		parentMountID, parentInode, err = dr.getParentFromMap(mountID, inode, pathID)
	}
	return parentMountID, parentInode, err
}

// Start the dentry resolver
func (dr *DentryResolver) Start() error {
	pathnames, ok, err := dr.probe.manager.GetMap("pathnames")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("map pathnames not found")
	}
	dr.pathnames = pathnames

	return nil
}

// Close cleans up the eRPC segment
func (dr *DentryResolver) Close() error {
	return errors.Wrap(unix.Munmap(dr.erpcSegment), "couldn't cleanup eRPC memory segment")
}

// ErrTruncatedParents is used to notify that some parents of the path are missing
type ErrTruncatedParents struct{}

func (err ErrTruncatedParents) Error() string {
	return "truncated_parents"
}

var errTruncatedParents ErrTruncatedParents

// ErrERPCResolution is used to notify that the eRPC resolution failed
type ErrERPCResolution struct{}

func (err ErrERPCResolution) Error() string {
	return "erpc_resolution"
}

var errERPCResolution ErrERPCResolution

// NewDentryResolver returns a new dentry resolver
func NewDentryResolver(probe *Probe) (*DentryResolver, error) {
	// We need at least 7 memory pages for the eRPC segment method to work.
	// For each segment of a path, we write 16 bytes to store (inode, mount_id, path_id), and then at least 2 bytes to
	// store the smallest possible path (segment of size 1 + trailing 0). 18 * 1500 = 27 000.
	// Then, 27k + 256 / page_size < 7.
	segment, err := unix.Mmap(0, 0, 7*os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to mmap memory segment")
	}

	return &DentryResolver{
		probe:           probe,
		cache:           make(map[uint32]*lru.Cache),
		erpc:            probe.erpc,
		erpcSegment:     segment,
		erpcSegmentSize: len(segment),
		erpcRequest:     ERPCRequest{},
		erpcEnabled:     probe.config.ERPCDentryResolutionEnabled,
		mapEnabled:      probe.config.MapDentryResolutionEnabled,
	}, nil
}
