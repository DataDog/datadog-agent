// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"structs"
	"unsafe"

	"github.com/cilium/ebpf"
)

// TypeID identifies a type in a BTF blob.
type TypeID uint32

// MapInfo is the kernel struct for map info
type MapInfo struct {
	_                     structs.HostLayout
	Type                  uint32
	ID                    ebpf.MapID
	KeySize               uint32
	ValueSize             uint32
	MaxEntries            uint32
	MapFlags              uint32
	Name                  ObjName
	Ifindex               uint32
	BtfVmlinuxValueTypeID TypeID
	NetnsDev              uint64
	NetnsIno              uint64
	BtfID                 uint32
	BtfKeyTypeID          TypeID
	BtfValueTypeID        TypeID
	BtfVmlinuxID          uint32
	MapExtra              uint64
}

// MapObjInfo retrieves information about a BPF Fd.
func MapObjInfo(fd uint32, info *MapInfo) error {
	err := ObjGetInfoByFd(&ObjGetInfoByFdAttr{
		BpfFd:   fd,
		InfoLen: uint32(unsafe.Sizeof(*info)),
		Info:    NewPointer(unsafe.Pointer(info)),
	})
	return err
}

func mapMemlock(fd uint32) (uint64, error) {
	fh, err := os.Open("/proc/self/fdinfo/" + strconv.FormatUint(uint64(fd), 10))
	if err != nil {
		return 0, err
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		key, rest, found := bytes.Cut(scanner.Bytes(), []byte(":"))
		if !found {
			// Line doesn't contain a colon, skip.
			continue
		}
		if string(key) != "memlock" {
			continue
		}
		// Cut the \t following the : as well as any potential trailing whitespace.
		rest = bytes.TrimSpace(rest)
		memlock, err := strconv.ParseUint(string(rest), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("can't parse field memlock: %v", err)
		}
		return memlock, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scanning fdinfo: %w", err)
	}
	return 0, ebpf.ErrNotSupported
}
