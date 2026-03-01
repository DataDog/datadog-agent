// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

// Flags needed by statmount
const (
	LSMTRoot = ^uint64(0)

	StatmountSbBasic       = 0x00000001
	StatmountMntBasic      = 0x00000002
	StatmountPropagateFrom = 0x00000004
	StatmountMntRoot       = 0x00000008
	StatmountMntPoint      = 0x00000010
	StatmountFsType        = 0x00000020
)

type mntIDReq struct {
	Size  uint32
	Spare uint32
	MntID uint64
	Param uint64
}

type statmountFixed struct {
	Size           uint32
	Spare1         uint32
	Mask           uint64
	SbDevMajor     uint32
	SbDevMinor     uint32
	SbMagic        uint64
	SbFlags        uint32
	FsType         uint32
	MntID          uint64
	MntParentID    uint64
	MntIDOld       uint32
	MntParentIDOld uint32
	MntAttr        uint64
	MntPropagation uint64
	MntPeerGroup   uint64
	MntMaster      uint64
	PropagateFrom  uint64
	MntRoot        uint32
	MntPoint       uint32
	Spare2         [50]uint64
}

// Statmount represents the data obtained from the syscall statmount()
type Statmount struct {
	Mask           uint64
	SbDevMajor     uint32
	SbDevMinor     uint32
	SbMagic        uint64
	SbFlags        uint32
	FsType         string
	MntID          uint64
	MntParentID    uint64
	MntIDOld       uint32
	MntParentIDOld uint32
	MntAttr        uint64
	MntPropagation uint64
	MntPeerGroup   uint64
	MntMaster      uint64
	PropagateFrom  uint64
	MntRoot        string
	MntPoint       string
}

func ztToString(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func parseStatmount(buf []byte) Statmount {
	var hdr statmountFixed
	_ = binary.Read(bytes.NewReader(buf), binary.NativeEndian, &hdr)
	base := uint32(unsafe.Sizeof(statmountFixed{}))
	return Statmount{
		Mask:           hdr.Mask,
		SbDevMajor:     hdr.SbDevMajor,
		SbDevMinor:     hdr.SbDevMinor,
		SbMagic:        hdr.SbMagic,
		SbFlags:        hdr.SbFlags,
		FsType:         ztToString(buf[base+hdr.FsType:]),
		MntID:          hdr.MntID,
		MntParentID:    hdr.MntParentID,
		MntIDOld:       hdr.MntIDOld,
		MntParentIDOld: hdr.MntParentIDOld,
		MntAttr:        hdr.MntAttr,
		MntPropagation: hdr.MntPropagation,
		MntPeerGroup:   hdr.MntPeerGroup,
		MntMaster:      hdr.MntMaster,
		PropagateFrom:  hdr.PropagateFrom,
		MntPoint:       ztToString(buf[base+hdr.MntPoint:]),
		MntRoot:        ztToString(buf[base+hdr.MntRoot:]),
	}
}

// newMountFromStatmount - Creates a new Mount from parsed MountInfo data
func newMountFromStatmount(sm *Statmount) *model.Mount {
	root := sm.MntRoot

	if sm.FsType == "cgroup2" && strings.HasPrefix(root, "/..") {
		cfs := utils.DefaultCGroupFS()
		root = filepath.Join(cfs.GetRootCGroupPath(), root)
	}

	// create a Mount out of the parsed MountInfo
	return &model.Mount{
		MountID:       sm.MntIDOld,
		MountIDUnique: sm.MntID,
		Device:        utils.Mkdev(sm.SbDevMajor, sm.SbDevMinor),
		ParentPathKey: model.PathKey{
			MountID: sm.MntParentIDOld,
		},
		FSType:        sm.FsType,
		MountPointStr: sm.MntPoint,
		Path:          sm.MntPoint,
		RootStr:       root,
		Origin:        model.MountOriginListmount,
		Visible:       true,
		Detached:      false,
	}
}

func listmount(req *mntIDReq, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	r1, _, e := unix.RawSyscall6(
		unix.SYS_LISTMOUNT,
		uintptr(unsafe.Pointer(req)),
		uintptr(unsafe.Pointer(&ids[0])),
		uintptr(len(ids)),
		0, 0, 0,
	)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

func statmount(req *mntIDReq, buf []byte) error {
	_, _, e := unix.RawSyscall6(
		unix.SYS_STATMOUNT,
		uintptr(unsafe.Pointer(req)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0, 0, 0,
	)
	if e != 0 {
		return e
	}
	return nil
}

func collectUniqueMountNSFDs(procfs string) ([]int, error) {
	seen := make(map[uint64]struct{})
	var ret []int

	ents, err := os.ReadDir(procfs)
	if err != nil {
		return nil, err
	}

	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		p := filepath.Join(procfs, pid, "ns", "mnt")

		linkTarget, err := os.Readlink(p)
		if err != nil {
			continue
		}

		ino, err := getInodeNumFromLink(linkTarget)
		if err != nil {
			continue
		}
		if _, ok := seen[ino]; ok {
			continue
		}
		fd, err := unix.Open(p, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		seen[ino] = struct{}{}
		ret = append(ret, fd)
	}
	return ret, nil
}

// GetPidListmount does a listmount for the mount namespace of a specific pid and calls cb() for each entry found
func GetPidListmount(procfs string, pid uint32, cb func(*model.Mount)) error {
	path := filepath.Join(procfs, strconv.Itoa(int(pid)), "ns", "mnt")
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("error opening mount namespace: %w", err)
	}
	return getAllFromList([]int{fd}, cb)
}

func getFdListmount(nsfd int, ino uint64, cb func(*model.Mount)) error {
	ids := make([]uint64, 2048)
	buf := make([]byte, 4096)
	firstIteration := true
	lastMountID := uint64(0)

	mask := uint64(StatmountSbBasic |
		StatmountMntBasic |
		StatmountPropagateFrom |
		StatmountMntRoot |
		StatmountMntPoint |
		StatmountFsType)

	for {
		req := mntIDReq{
			Size:  uint32(unsafe.Sizeof(mntIDReq{})),
			Spare: 0,
			MntID: LSMTRoot,
			Param: 0,
		}
		if firstIteration {
			if err := unix.Setns(nsfd, unix.CLONE_NEWNS); err != nil {
				return fmt.Errorf("failed to setns: %v", err)
			}
		} else {
			req.Param = lastMountID
		}

		n, err := listmount(&req, ids)

		if err != nil || n < 0 {
			return fmt.Errorf("failed to listmount: %v", err)
		}

		for i := 0; i < n; i++ {
			req2 := mntIDReq{
				Size:  uint32(unsafe.Sizeof(mntIDReq{})),
				Spare: 0,
				MntID: ids[i],
				Param: mask,
			}
			// Ignore ENOENT, sometimes the mount might have been unmounted between listmount and this call
			if err := statmount(&req2, buf); err != nil && err != unix.ENOENT {
				return fmt.Errorf("failed to statmount: %v", err)
			}
			sm := parseStatmount(buf)
			mnt := newMountFromStatmount(&sm)
			mnt.NamespaceInode = uint32(ino)
			cb(mnt)
			lastMountID = req2.MntID
		}

		if n == len(ids) {
			// Not all mounts were obtained yet
			firstIteration = false
			continue
		}

		if n < len(ids) {
			// All mounts for this namespace were obtained
			break
		}
	}

	return nil
}

func getAllFromList(lst []int, cb func(*model.Mount)) error {
	defer func() {
		for _, fd := range lst {
			unix.Close(fd)
		}
	}()

	done := make(chan error, 1)

	go func() {
		runtime.LockOSThread()

		if err := unix.Unshare(unix.CLONE_FS); err != nil {
			done <- fmt.Errorf("unshare error: %w", err)
			return
		}

		for _, fd := range lst {
			err := getFdListmount(fd, 0, cb)
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	err := <-done
	return err
}

// GetAll Retrieves all the mountpoints from all the mount namespaces present in the procfs path and call
// a callback for each one
func GetAll(procfs string, cb func(*model.Mount)) error {
	nsFDs, err := collectUniqueMountNSFDs(procfs)

	if err != nil {
		return fmt.Errorf("error collecting unique mount namespace file descriptors: %w", err)
	}

	return getAllFromList(nsFDs, cb)
}

// HasListMount returns true if the kernel has the listmount() syscall, false otherwise
func HasListMount() bool {
	_, _, errno := unix.Syscall(unix.SYS_LISTMOUNT, 0, 0, 0)
	if errno == unix.ENOSYS {
		return false
	}
	_, _, errno = unix.Syscall(unix.SYS_STATMOUNT, 0, 0, 0)
	return errno != unix.ENOSYS

}
