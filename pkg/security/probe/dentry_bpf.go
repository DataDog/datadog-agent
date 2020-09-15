// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"C"
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

// DentryResolver resolves inode/mountID to full paths
type DentryResolver struct {
	probe     *Probe
	pathnames *ebpf.Table
}

type pathKey struct {
	inode   uint64
	mountID uint32
}

func (p *pathKey) Write(buffer []byte) {
	byteOrder.PutUint64(buffer[0:8], p.inode)
	byteOrder.PutUint32(buffer[8:12], p.mountID)
	byteOrder.PutUint32(buffer[12:16], 0)
}

func (p *pathKey) Read(buffer []byte) {
	p.inode = byteOrder.Uint64(buffer[0:8])
	p.mountID = byteOrder.Uint32(buffer[8:12])
}

func (p *pathKey) IsNull() bool {
	return p.inode == 0 && p.mountID == 0
}

func (p *pathKey) String() string {
	return fmt.Sprintf("%x/%x", p.mountID, p.inode)
}

func (p *pathKey) Bytes() ([]byte, error) {
	if p.IsNull() {
		return nil, fmt.Errorf("invalid inode/mountID couple: %s", p.String())
	}

	b := make([]byte, 16)
	p.Write(b)

	return b, nil
}

type pathValue struct {
	parent pathKey
	name   [256]byte
}

func (dr *DentryResolver) getName(mountID uint32, inode uint64) (name string, err error) {
	key := pathKey{mountID: mountID, inode: inode}

	keyBuffer, err := key.Bytes()
	if err != nil {
		return "", err
	}

	pathRaw := []byte{}
	var nameRaw [256]byte

	if pathRaw, err = dr.pathnames.Get(keyBuffer); err != nil {
		return "", fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`", mountID, inode)
	}

	if err = binary.Read(bytes.NewBuffer(pathRaw[16:]), byteOrder, &nameRaw); err != nil {
		return "", errors.Wrap(err, "failed to decode received data (pathLeaf)")
	}

	return C.GoString((*C.char)(unsafe.Pointer(&nameRaw))), nil
}

// GetName resolves a couple of mountID/inode to a path
func (dr *DentryResolver) GetName(mountID uint32, inode uint64) string {
	name, _ := dr.getName(mountID, inode)
	return name
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) resolve(mountID uint32, inode uint64) (filename string, err error) {
	key := pathKey{mountID: mountID, inode: inode}

	keyBuffer, err := key.Bytes()
	if err != nil {
		return "", err
	}

	done := false
	pathRaw := []byte{}
	var path pathValue

	// Fetch path recursively
	for !done {
		if pathRaw, err = dr.pathnames.Get(keyBuffer); err != nil {
			filename = dentryPathKeyNotFound
			break
		}

		path.parent.Read(pathRaw)
		if err = binary.Read(bytes.NewBuffer(pathRaw[16:]), byteOrder, &path.name); err != nil {
			err = errors.Wrap(err, "failed to decode received data (pathLeaf)")
			break
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.name[0] != '\x00' && path.name[0] != '/' {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.name))) + filename
		}

		if path.parent.inode == 0 {
			break
		}

		// Prepare next key
		path.parent.Write(keyBuffer)
	}

	if len(filename) == 0 {
		filename = "/"
	}

	return
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(mountID uint32, inode uint64) string {
	path, _ := dr.resolve(mountID, inode)
	return path
}

// GetParent - Return the parent mount_id/inode
func (dr *DentryResolver) GetParent(mountID uint32, inode uint64) (uint32, uint64, error) {
	key := pathKey{mountID: mountID, inode: inode}

	keyBuffer, err := key.Bytes()
	if err != nil {
		return 0, 0, err
	}

	pathRaw, err := dr.pathnames.Get(keyBuffer)
	if err != nil {
		return 0, 0, err
	}

	var path pathValue
	path.parent.Read(pathRaw)

	return path.parent.mountID, path.parent.inode, nil
}

// Start the dentry resolver
func (dr *DentryResolver) Start() error {
	pathnames := dr.probe.Table("pathnames")
	if pathnames == nil {
		return errors.New("pathnames BPF_HASH table doesn't exist")
	}
	dr.pathnames = pathnames

	return nil
}
