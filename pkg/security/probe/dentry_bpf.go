// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"C"
	"fmt"
	"unsafe"

	lib "github.com/DataDog/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

// DentryResolver resolves inode/mountID to full paths
type DentryResolver struct {
	probe     *Probe
	pathnames *lib.Map
}

// ErrInvalidKeyPath is returned when inode or mountid are not valid
type ErrInvalidKeyPath struct {
	Inode   uint64
	MountID uint32
}

func (e *ErrInvalidKeyPath) Error() string {
	return fmt.Sprintf("invalid inode/mountID couple: %d/%d", e.Inode, e.MountID)
}

type PathKey struct {
	Inode   uint64
	MountID uint32
	Padding uint32
}

func (p *PathKey) Write(buffer []byte) {
	ebpf.ByteOrder.PutUint64(buffer[0:8], p.Inode)
	ebpf.ByteOrder.PutUint32(buffer[8:12], p.MountID)
	ebpf.ByteOrder.PutUint32(buffer[12:16], 0)
}

func (p *PathKey) IsNull() bool {
	return p.Inode == 0 && p.MountID == 0
}

func (p *PathKey) String() string {
	return fmt.Sprintf("%x/%x", p.MountID, p.Inode)
}

func (p *PathKey) MarshalBinary() ([]byte, error) {
	if p.IsNull() {
		return nil, &ErrInvalidKeyPath{Inode: p.Inode, MountID: p.MountID}
	}

	b := make([]byte, 16)
	p.Write(b)

	return b, nil
}

type PathValue struct {
	Parent PathKey
	Name   [128]byte
}

func (dr *DentryResolver) getName(mountID uint32, inode uint64) (name string, err error) {
	key := PathKey{MountID: mountID, Inode: inode}
	var path PathValue

	if err := dr.pathnames.Lookup(key, &path); err != nil {
		return "", fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`", mountID, inode)
	}

	return C.GoString((*C.char)(unsafe.Pointer(&path.Name))), nil
}

// GetName resolves a couple of mountID/inode to a path
func (dr *DentryResolver) GetName(mountID uint32, inode uint64) string {
	name, _ := dr.getName(mountID, inode)
	return name
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) resolve(mountID uint32, inode uint64) (string, error) {
	var done bool
	var path PathValue
	var filename string
	key := PathKey{MountID: mountID, Inode: inode}

	keyBuffer, err := key.MarshalBinary()
	if err != nil {
		return "", err
	}

	// Fetch path recursively
	for !done {
		if err = dr.pathnames.Lookup(keyBuffer, &path); err != nil {
			filename = dentryPathKeyNotFound
			break
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] != '\x00' && path.Name[0] != '/' {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.Name))) + filename
		}

		if path.Parent.Inode == 0 {
			break
		}

		// Prepare next key
		path.Parent.Write(keyBuffer)
	}

	if len(filename) == 0 {
		filename = "/"
	}

	return filename, err
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(mountID uint32, inode uint64) string {
	path, _ := dr.resolve(mountID, inode)
	return path
}

// GetParent - Return the parent mount_id/inode
func (dr *DentryResolver) GetParent(mountID uint32, inode uint64) (uint32, uint64, error) {
	key := PathKey{MountID: mountID, Inode: inode}
	var path PathValue

	if err := dr.pathnames.Lookup(key, &path); err != nil {
		return 0, 0, err
	}

	return path.Parent.MountID, path.Parent.Inode, nil
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
