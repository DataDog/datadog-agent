// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"C"
	"fmt"
	"unsafe"

	lib "github.com/DataDog/ebpf"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

const (
	dentryPathKeyNotFound = "error: dentry path key not found"
	fakeInodeMSW          = 0xdeadc001
)

// DentryResolver resolves inode/mountID to full paths
type DentryResolver struct {
	probe     *Probe
	pathnames *lib.Map
	cache     map[uint32]*lru.Cache
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
	ebpf.ByteOrder.PutUint64(buffer[0:8], p.Inode)
	ebpf.ByteOrder.PutUint32(buffer[8:12], p.MountID)
	ebpf.ByteOrder.PutUint32(buffer[12:16], p.PathID)
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
	Name   [128]byte
}

// DelCacheEntry removes an entry from the cache
func (dr *DentryResolver) DelCacheEntry(mountID uint32, inode uint64) {
	if entries, exists := dr.cache[mountID]; exists {
		entries.Remove(inode)
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

func (dr *DentryResolver) getNameFromMap(mountID uint32, inode uint64, pathID uint32) (name string, err error) {
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
	if err != nil {
		name, _ = dr.getNameFromMap(mountID, inode, pathID)
	}
	return name
}

// ResolveFromCache resolve from the cache
func (dr *DentryResolver) ResolveFromCache(mountID uint32, inode uint64) (filename string, err error) {
	key := PathKey{MountID: mountID, Inode: inode}

	// Fetch path recursively
	for {
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

// ResolveFromMap resolves from kernel map
func (dr *DentryResolver) ResolveFromMap(mountID uint32, inode uint64, pathID uint32) (filename string, err error) {
	key := PathKey{MountID: mountID, Inode: inode, PathID: pathID}
	var path PathValue

	keyBuffer, err := key.MarshalBinary()
	if err != nil {
		return "", err
	}

	toAdd := make(map[PathKey]PathValue)

	// Fetch path recursively
	for {
		key.Write(keyBuffer)
		if err = dr.pathnames.Lookup(keyBuffer, &path); err != nil {
			filename = dentryPathKeyNotFound
			break
		}

		cacheKey := PathKey{MountID: key.MountID, Inode: key.Inode}
		toAdd[cacheKey] = path

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

	if err == nil {
		for k, v := range toAdd {
			// do not cache fake path keys in the case of rename events
			if k.Inode>>32 != fakeInodeMSW {
				dr.cacheInode(k.MountID, k.Inode, v)
			}
		}
	}

	return filename, err
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(mountID uint32, inode uint64, pathID uint32) string {
	path, err := dr.ResolveFromCache(mountID, inode)
	if err != nil {
		path, _ = dr.ResolveFromMap(mountID, inode, pathID)
	}
	return path
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

// NewDentryResolver returns a new dentry resolver
func NewDentryResolver(probe *Probe) (*DentryResolver, error) {
	return &DentryResolver{
		probe: probe,
		cache: make(map[uint32]*lru.Cache),
	}, nil
}
