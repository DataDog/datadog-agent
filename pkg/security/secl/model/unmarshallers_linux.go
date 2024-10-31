// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

func validateReadSize(size, read int) (int, error) {
	if size != read {
		return 0, fmt.Errorf("expected %d, read %d: %w", size, read, ErrIncorrectDataSize)
	}
	return read, nil
}

// BinaryUnmarshaler interface implemented by every event type
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) (int, error)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *CGroupContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8+16 {
		return 0, ErrNotEnoughData
	}

	e.CGroupFlags = containerutils.CGroupFlags(binary.NativeEndian.Uint64(data[:8]))

	n, err := e.CGroupFile.UnmarshalBinary(data[8:])
	if err != nil {
		return 0, err
	}

	return 8 + n, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ContainerContext) UnmarshalBinary(data []byte) (int, error) {
	id, err := UnmarshalString(data, ContainerIDLen)
	if err != nil {
		return 0, err
	}

	e.ContainerID = containerutils.ContainerID(id)

	return ContainerIDLen, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ChmodEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = binary.NativeEndian.Uint32(data[0:4])
	return n + 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ChownEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	// First convert to int32 to sign extend, then convert to int64
	e.UID = int64(int32(binary.NativeEndian.Uint32(data[0:4])))
	e.GID = int64(int32(binary.NativeEndian.Uint32(data[4:8])))
	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *Event) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	e.TimestampRaw = binary.NativeEndian.Uint64(data[0:8])
	e.Type = binary.NativeEndian.Uint32(data[8:12])
	e.Flags = binary.NativeEndian.Uint32(data[12:16])

	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SetuidEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.UID = binary.NativeEndian.Uint32(data[0:4])
	e.EUID = binary.NativeEndian.Uint32(data[4:8])
	e.FSUID = binary.NativeEndian.Uint32(data[8:12])
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SetgidEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.GID = binary.NativeEndian.Uint32(data[0:4])
	e.EGID = binary.NativeEndian.Uint32(data[4:8])
	e.FSGID = binary.NativeEndian.Uint32(data[8:12])
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *CapsetEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.CapEffective = binary.NativeEndian.Uint64(data[0:8])
	e.CapPermitted = binary.NativeEndian.Uint64(data[8:16])
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *Credentials) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 48 {
		return 0, ErrNotEnoughData
	}

	e.UID = binary.NativeEndian.Uint32(data[0:4])
	e.GID = binary.NativeEndian.Uint32(data[4:8])
	e.EUID = binary.NativeEndian.Uint32(data[8:12])
	e.EGID = binary.NativeEndian.Uint32(data[12:16])
	e.FSUID = binary.NativeEndian.Uint32(data[16:20])
	e.FSGID = binary.NativeEndian.Uint32(data[20:24])
	e.AUID = binary.NativeEndian.Uint32(data[24:28])
	if binary.NativeEndian.Uint32(data[28:32]) != 1 {
		e.AUID = AuditUIDUnset
	}
	e.CapEffective = binary.NativeEndian.Uint64(data[32:40])
	e.CapPermitted = binary.NativeEndian.Uint64(data[40:48])
	return 48, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *LoginUIDWriteEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.AUID = binary.NativeEndian.Uint32(data[0:4])
	return 4, nil
}

func unmarshalTime(data []byte) time.Time {
	if t := int64(binary.NativeEndian.Uint64(data)); t != 0 {
		return time.Unix(0, t)
	}
	return time.Time{}
}

// isValidTTYName uses a naive assumption as other tty driver may create tty with other prefix
func isValidTTYName(ttyName string) bool {
	return IsPrintableASCII(ttyName) && (strings.HasPrefix(ttyName, "tty") || strings.HasPrefix(ttyName, "pts"))
}

// UnmarshalProcEntryBinary unmarshalls process_entry_t from process.h
func (e *Process) UnmarshalProcEntryBinary(data []byte) (int, error) {
	const size = 160
	if len(data) < size {
		return 0, ErrNotEnoughData
	}

	read, err := UnmarshalBinary(data, &e.FileEvent)
	if err != nil {
		return 0, err
	}

	e.ExecTime = unmarshalTime(data[read : read+8])
	read += 8

	var ttyRaw [64]byte
	SliceToArray(data[read:read+64], ttyRaw[:])
	ttyName, err := UnmarshalString(ttyRaw[:], 64)
	if err != nil {
		return 0, err
	}
	if isValidTTYName(ttyName) {
		e.TTYName = ttyName
	}
	read += 64

	var commRaw [16]byte
	SliceToArray(data[read:read+16], commRaw[:])
	e.Comm, err = UnmarshalString(commRaw[:], 16)
	if err != nil {
		return 0, err
	}
	read += 16

	return validateReadSize(size, read)
}

// UnmarshalPidCacheBinary unmarshalls Unmarshal pid_cache_t
func (e *Process) UnmarshalPidCacheBinary(data []byte) (int, error) {
	const size = 88
	if len(data) < size {
		return 0, ErrNotEnoughData
	}

	var read int

	// Unmarshal pid_cache_t
	cookie := binary.NativeEndian.Uint64(data[0:SizeOfCookie])
	if cookie > 0 {
		e.Cookie = cookie
	}
	e.PPid = binary.NativeEndian.Uint32(data[8:12])

	// padding

	e.ForkTime = unmarshalTime(data[16:24])
	e.ExitTime = unmarshalTime(data[24:32])
	e.UserSession.ID = binary.NativeEndian.Uint64(data[32:40])

	// Unmarshal the credentials contained in pid_cache_t
	read, err := UnmarshalBinary(data[40:], &e.Credentials)
	if err != nil {
		return 0, err
	}
	read += 40

	return validateReadSize(size, read)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *Process) UnmarshalBinary(data []byte) (int, error) {
	const size = 288 // size of struct exec_event_t starting from process_entry_t, inclusive
	if len(data) < size {
		return 0, ErrNotEnoughData
	}
	var read int

	n, err := e.UnmarshalProcEntryBinary((data))
	if err != nil {
		return 0, err
	}
	read += n

	n, err = e.UnmarshalPidCacheBinary((data[read:]))
	if err != nil {
		return 0, err
	}
	read += n

	// interpreter part
	var pathKey PathKey

	n, err = pathKey.UnmarshalBinary(data[read:])
	if err != nil {
		return 0, err
	}
	read += n

	// TODO: Is there a better way to determine if there's no interpreter?
	if e.FileEvent.Inode != pathKey.Inode || e.FileEvent.MountID != pathKey.MountID {
		e.LinuxBinprm.FileEvent.PathKey = pathKey
	}

	if len(data[read:]) < 24 {
		return 0, ErrNotEnoughData
	}

	e.ArgsID = binary.NativeEndian.Uint64(data[read : read+8])
	e.EnvsID = binary.NativeEndian.Uint64(data[read+8 : read+16])
	read += 16

	e.ArgsTruncated = binary.NativeEndian.Uint32(data[read:read+4]) == 1
	e.EnvsTruncated = binary.NativeEndian.Uint32(data[read+4:read+8]) == 1
	read += 8

	return validateReadSize(size, read)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ExitEvent) UnmarshalBinary(data []byte) (int, error) {
	// Unmarshal exit code
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	exitStatus := binary.NativeEndian.Uint32(data[0:4])
	if exitStatus&0x7F == 0x00 { // process terminated normally
		e.Cause = uint32(ExitExited)
		e.Code = (exitStatus >> 8) & 0xFF
	} else if exitStatus&0x7F != 0x7F { // process terminated because of a signal
		if exitStatus&0x80 == 0x80 { // coredump signal
			e.Cause = uint32(ExitCoreDumped)
			e.Code = exitStatus & 0x7F
		} else { // other signals
			e.Cause = uint32(ExitSignaled)
			e.Code = exitStatus & 0x7F
		}
	}

	return 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *InvalidateDentryEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	e.Inode = binary.NativeEndian.Uint64(data[0:8])
	e.MountID = binary.NativeEndian.Uint32(data[8:12])
	// padding

	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ArgsEnvsEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, ErrNotEnoughData
	}

	e.ID = binary.NativeEndian.Uint64(data[0:8])
	e.Size = binary.NativeEndian.Uint32(data[8:12])
	if e.Size > MaxArgEnvSize {
		e.Size = MaxArgEnvSize
	}

	argsEnvSize := int(e.Size)
	data = data[12:]
	if len(data) < argsEnvSize {
		return 12, ErrNotEnoughData
	}

	for i := range e.ValuesRaw {
		e.ValuesRaw[i] = 0
	}

	SliceToArray(data[:argsEnvSize], e.ValuesRaw[:argsEnvSize])

	return 12 + argsEnvSize, nil
}

// UnmarshalBinary unmarshals the given content
func (p *PathKey) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	p.Inode = binary.NativeEndian.Uint64(data[0:8])
	p.MountID = binary.NativeEndian.Uint32(data[8:12])
	p.PathID = binary.NativeEndian.Uint32(data[12:16])

	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *FileFields) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 72 {
		return 0, ErrNotEnoughData
	}

	n, err := e.PathKey.UnmarshalBinary(data)
	if err != nil {
		return n, err
	}
	data = data[n:]

	e.Device = binary.NativeEndian.Uint32(data[0:4])

	e.Flags = int32(binary.NativeEndian.Uint32(data[4:8]))

	e.UID = binary.NativeEndian.Uint32(data[8:12])
	e.GID = binary.NativeEndian.Uint32(data[12:16])
	e.NLink = binary.NativeEndian.Uint32(data[16:20])
	e.Mode = binary.NativeEndian.Uint16(data[20:22])

	timeSec := binary.NativeEndian.Uint64(data[24:32])
	timeNsec := binary.NativeEndian.Uint64(data[32:40])
	e.CTime = uint64(time.Unix(int64(timeSec), int64(timeNsec)).UnixNano())

	timeSec = binary.NativeEndian.Uint64(data[40:48])
	timeNsec = binary.NativeEndian.Uint64(data[48:56])
	e.MTime = uint64(time.Unix(int64(timeSec), int64(timeNsec)).UnixNano())

	return 72, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *FileEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.FileFields)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *LinkEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.Source, &e.Target)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *MkdirEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = binary.NativeEndian.Uint32(data[0:4])
	return n + 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (m *Mount) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 56 {
		return 0, ErrNotEnoughData
	}

	n, err := m.RootPathKey.UnmarshalBinary(data)
	if err != nil {
		return 0, err
	}
	data = data[n:]

	n, err = m.ParentPathKey.UnmarshalBinary(data)
	if err != nil {
		return 0, err
	}
	data = data[n:]

	m.Device = binary.NativeEndian.Uint32(data[0:4])
	m.BindSrcMountID = binary.NativeEndian.Uint32(data[4:8])
	m.FSType, err = UnmarshalString(data[8:], 16)
	if err != nil {
		return 0, err
	}

	m.MountID = m.RootPathKey.MountID
	m.Origin = MountOriginEvent

	return 56, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *MountEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.Mount)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UnshareMountNSEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := e.Mount.UnmarshalBinary(data)
	if err != nil {
		return 0, err
	}
	e.Origin = MountOriginUnshare

	return n, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ChdirEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.File)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.Flags = binary.NativeEndian.Uint32(data[0:4])
	e.Mode = binary.NativeEndian.Uint32(data[4:8])
	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (s *SpanContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}

	s.SpanID = binary.NativeEndian.Uint64(data[0:8])
	s.TraceID.Lo = int64(binary.NativeEndian.Uint64(data[8:16]))
	s.TraceID.Hi = int64(binary.NativeEndian.Uint64(data[16:24]))
	return 24, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SELinuxEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.EventKind = SELinuxEventKind(binary.NativeEndian.Uint32(data[0:4]))

	switch e.EventKind {
	case SELinuxBoolChangeEventKind:
		boolValue := binary.NativeEndian.Uint32(data[4:8])
		if boolValue == ^uint32(0) {
			e.BoolChangeValue = "error"
		} else if boolValue > 0 {
			e.BoolChangeValue = "on"
		} else {
			e.BoolChangeValue = "off"
		}
	case SELinuxBoolCommitEventKind:
		boolValue := binary.NativeEndian.Uint32(data[4:8])
		e.BoolCommitValue = boolValue != 0
	case SELinuxStatusChangeEventKind:
		disableValue := binary.NativeEndian.Uint16(data[4:6]) != 0
		enforceValue := binary.NativeEndian.Uint16(data[6:8]) != 0
		if disableValue {
			e.EnforceStatus = "disabled"
		} else if enforceValue {
			e.EnforceStatus = "enforcing"
		} else {
			e.EnforceStatus = "permissive"
		}
	}

	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself, process_context_t kernel side
func (p *PIDContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}

	p.Pid = binary.NativeEndian.Uint32(data[0:4])
	p.Tid = binary.NativeEndian.Uint32(data[4:8])
	p.NetNS = binary.NativeEndian.Uint32(data[8:12])
	p.IsKworker = binary.NativeEndian.Uint32(data[12:16]) > 0
	p.ExecInode = binary.NativeEndian.Uint64(data[16:24])

	return 24, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.Old, &e.New)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.File)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SetXAttrEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 200 {
		return n, ErrNotEnoughData
	}
	SliceToArray(data[0:200], e.NameRaw[:])

	return n + 200, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SyscallEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.Retval = int64(binary.NativeEndian.Uint64(data[0:8]))

	return 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SyscallContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.ID = uint32(binary.NativeEndian.Uint32(data))

	// padding

	return 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UmountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.MountID = binary.NativeEndian.Uint32(data[0:4])

	return 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.Flags = binary.NativeEndian.Uint32(data[0:4])
	// padding

	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UtimesEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.SyscallContext, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 32 {
		return 0, ErrNotEnoughData
	}

	timeSec := binary.NativeEndian.Uint64(data[0:8])
	timeNsec := binary.NativeEndian.Uint64(data[8:16])
	e.Atime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = binary.NativeEndian.Uint64(data[16:24])
	timeNsec = binary.NativeEndian.Uint64(data[24:32])
	e.Mtime = time.Unix(int64(timeSec), int64(timeNsec))

	return n + 32, nil
}

// UnmarshalBinary calls a series of BinaryUnmarshaler
func UnmarshalBinary(data []byte, binaryUnmarshalers ...BinaryUnmarshaler) (int, error) {
	read := 0
	for _, marshaler := range binaryUnmarshalers {
		n, err := marshaler.UnmarshalBinary(data[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *MountReleasedEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.MountID = binary.NativeEndian.Uint32(data[0:4])

	return 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *BPFEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}
	cursor := read

	read, err = e.Map.UnmarshalBinary(data[cursor:])
	if err != nil {
		return 0, err
	}
	cursor += read
	read, err = e.Program.UnmarshalBinary(data[cursor:])
	if err != nil {
		return 0, err
	}
	cursor += read
	if len(data) < cursor+4 {
		return 0, ErrNotEnoughData
	}
	e.Cmd = binary.NativeEndian.Uint32(data[cursor : cursor+4])
	return cursor + 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (m *BPFMap) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}
	m.ID = binary.NativeEndian.Uint32(data[0:4])
	m.Type = binary.NativeEndian.Uint32(data[4:8])

	var err error
	m.Name, err = UnmarshalString(data[8:24], 16)
	if err != nil {
		return 0, err
	}
	return 24, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (p *BPFProgram) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 64 {
		return 0, ErrNotEnoughData
	}
	p.ID = binary.NativeEndian.Uint32(data[0:4])
	p.Type = binary.NativeEndian.Uint32(data[4:8])
	p.AttachType = binary.NativeEndian.Uint32(data[8:12])
	// padding
	helpers := []uint64{0, 0, 0}
	helpers[0] = binary.NativeEndian.Uint64(data[16:24])
	helpers[1] = binary.NativeEndian.Uint64(data[24:32])
	helpers[2] = binary.NativeEndian.Uint64(data[32:40])
	p.Helpers = parseHelpers(helpers)

	var err error
	p.Name, err = UnmarshalString(data[40:56], 16)
	if err != nil {
		return 0, err
	}
	p.Tag = parseSHA1Tag(data[56:64])
	return 64, nil
}

// parseSHA1Tag parses the short sha1 digest from the kernel event
func parseSHA1Tag(data []byte) string {
	if len(data) != 8 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(16)

	for _, b := range data {
		if _, err := fmt.Fprintf(&builder, "%02x", b); err != nil {
			// should really never happen when writing to a string.Builder
			return ""
		}
	}
	return builder.String()
}

func parseHelpers(helpers []uint64) []uint32 {
	if len(helpers) < 3 {
		return nil
	}

	var popcnt int
	for _, h := range helpers {
		popcnt += bits.OnesCount64(h)
	}
	rep := make([]uint32, 0, popcnt)

	for i := 0; i < 192; i++ {
		add := false
		if i < 64 {
			if helpers[0]&(1<<i) == (1 << i) {
				add = true
			}
		} else if i < 128 {
			if helpers[1]&(1<<(i-64)) == (1 << (i - 64)) {
				add = true
			}
		} else if i < 192 {
			if helpers[2]&(1<<(i-128)) == (1 << (i - 128)) {
				add = true
			}
		}

		if add {
			rep = append(rep, uint32(i))
		}
	}
	return rep
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *PTraceEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 16 {
		return 0, ErrNotEnoughData
	}

	e.Request = binary.NativeEndian.Uint32(data[read : read+4])
	e.PID = binary.NativeEndian.Uint32(data[read+4 : read+8])
	e.Address = binary.NativeEndian.Uint64(data[read+8 : read+16])
	return read + 16, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MMapEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 40 {
		return 0, ErrNotEnoughData
	}

	e.Addr = binary.NativeEndian.Uint64(data[read : read+8])
	e.Offset = binary.NativeEndian.Uint64(data[read+8 : read+16])
	e.Len = binary.NativeEndian.Uint64(data[read+16 : read+24])
	e.Protection = binary.NativeEndian.Uint64(data[read+24 : read+32])
	e.Flags = binary.NativeEndian.Uint64(data[read+32 : read+40])
	return read + 40, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MProtectEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 32 {
		return 0, ErrNotEnoughData
	}

	e.VMStart = binary.NativeEndian.Uint64(data[read : read+8])
	e.VMEnd = binary.NativeEndian.Uint64(data[read+8 : read+16])
	e.VMProtection = int(binary.NativeEndian.Uint32(data[read+16 : read+24]))
	e.ReqProtection = int(binary.NativeEndian.Uint32(data[read+24 : read+32]))
	return read + 32, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *LoadModuleEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 188 {
		return 0, ErrNotEnoughData
	}

	e.Name, err = UnmarshalString(data[read:read+56], 56)
	read += 56

	if err != nil {
		return 0, err
	}

	e.Args, err = UnmarshalString(data[read:read+128], 128)
	read += 128

	e.ArgsTruncated = binary.NativeEndian.Uint32(data[read:read+4]) == uint32(1)
	read += 4

	if err != nil {
		return 0, err
	}
	e.LoadedFromMemory = binary.NativeEndian.Uint32(data[read:read+4]) == uint32(1)
	read += 4

	return read, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UnloadModuleEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 56 {
		return 0, ErrNotEnoughData
	}

	e.Name, err = UnmarshalString(data[read:read+56], 56)
	if err != nil {
		return 0, err
	}
	return read + 56, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SignalEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 8 {
		return 0, ErrNotEnoughData
	}

	e.PID = binary.NativeEndian.Uint32(data[read : read+4])
	e.Type = binary.NativeEndian.Uint32(data[read+4 : read+8])
	return read + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SpliceEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 8 {
		return 0, ErrNotEnoughData
	}

	e.PipeEntryFlag = binary.NativeEndian.Uint32(data[read : read+4])
	e.PipeExitFlag = binary.NativeEndian.Uint32(data[read+4 : read+8])
	return read + 4, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *CgroupTracingEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.ContainerContext)
	if err != nil {
		return 0, err
	}
	cursor := read

	read, err = UnmarshalBinary(data, &e.CGroupContext)
	if err != nil {
		return 0, err
	}
	cursor += read

	read, err = e.Config.EventUnmarshalBinary(data[cursor:])
	if err != nil {
		return 0, err
	}
	cursor += read

	if len(data)-cursor < 8 {
		return 0, ErrNotEnoughData
	}

	e.ConfigCookie = binary.NativeEndian.Uint64(data[cursor : cursor+8])
	return cursor + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *CgroupWriteEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.File)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 8 {
		return 0, ErrNotEnoughData
	}

	e.Pid = binary.NativeEndian.Uint32(data[read : read+4])
	read += 4

	e.CGroupFlags = binary.NativeEndian.Uint32(data[read : read+4])
	read += 4

	return read, nil
}

// EventUnmarshalBinary unmarshals a binary representation of itself
func (adlc *ActivityDumpLoadConfig) EventUnmarshalBinary(data []byte) (int, error) {
	if len(data) < 48 {
		return 0, ErrNotEnoughData
	}

	eventMask := binary.NativeEndian.Uint64(data[0:8])
	for i := uint64(0); i < 64; i++ {
		if eventMask&(1<<i) == (1 << i) {
			adlc.TracedEventTypes = append(adlc.TracedEventTypes, EventType(i)+FirstDiscarderEventType)
		}
	}
	adlc.Timeout = time.Duration(binary.NativeEndian.Uint64(data[8:16]))
	adlc.WaitListTimestampRaw = binary.NativeEndian.Uint64(data[16:24])
	adlc.StartTimestampRaw = binary.NativeEndian.Uint64(data[24:32])
	adlc.EndTimestampRaw = binary.NativeEndian.Uint64(data[32:40])
	adlc.Rate = binary.NativeEndian.Uint32(data[40:44])
	adlc.Paused = binary.NativeEndian.Uint32(data[44:48])
	return 48, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (adlc *ActivityDumpLoadConfig) UnmarshalBinary(data []byte) error {
	_, err := adlc.EventUnmarshalBinary(data)
	return err
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *NetworkDeviceContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.NetNS = binary.NativeEndian.Uint32(data[0:4])
	e.IfIndex = binary.NativeEndian.Uint32(data[4:8])
	return 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *NetworkContext) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.Device)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 44 {
		return 0, ErrNotEnoughData
	}

	var srcIP, dstIP [16]byte
	SliceToArray(data[read:read+16], srcIP[:])
	SliceToArray(data[read+16:read+32], dstIP[:])
	e.Source.Port = binary.BigEndian.Uint16(data[read+32 : read+34])
	e.Destination.Port = binary.BigEndian.Uint16(data[read+34 : read+36])
	// padding 4 bytes

	e.Size = binary.NativeEndian.Uint32(data[read+40 : read+44])
	e.L3Protocol = binary.NativeEndian.Uint16(data[read+44 : read+46])
	e.L4Protocol = binary.NativeEndian.Uint16(data[read+46 : read+48])

	// readjust IP sizes depending on the protocol
	switch e.L3Protocol {
	case 0x800: // unix.ETH_P_IP
		e.Source.IPNet = *eval.IPNetFromIP(srcIP[0:4])
		e.Destination.IPNet = *eval.IPNetFromIP(dstIP[0:4])
	default:
		e.Source.IPNet = *eval.IPNetFromIP(srcIP[:])
		e.Destination.IPNet = *eval.IPNetFromIP(dstIP[:])
	}
	return read + 48, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *DNSEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 10 {
		return 0, ErrNotEnoughData
	}

	e.ID = binary.NativeEndian.Uint16(data[0:2])
	e.Count = binary.NativeEndian.Uint16(data[2:4])
	e.Type = binary.NativeEndian.Uint16(data[4:6])
	e.Class = binary.NativeEndian.Uint16(data[6:8])
	e.Size = binary.NativeEndian.Uint16(data[8:10])
	var err error
	e.Name, err = decodeDNSName(data[10:])
	if err != nil {
		return 0, fmt.Errorf("failed to decode %s (id: %d, count: %d, type:%d, size:%d)", data[10:], e.ID, e.Count, e.Type, e.Size)
	}
	if err = validateDNSName(e.Name); err != nil {
		return 0, err
	}
	return len(data), nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *IMDSEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 10 {
		return 0, ErrNotEnoughData
	}

	firstWord := strings.SplitN(string(data[0:10]), " ", 2)
	switch {
	case strings.HasPrefix(firstWord[0], "HTTP"):
		// this is an IMDS response
		e.Type = IMDSResponseType
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(data)), nil)
		if err != nil {
			return 0, fmt.Errorf("failed to parse IMDS response: %v", err)
		}
		e.fillFromIMDSHeader(resp.Header, "")

		// try to parse cloud provider specific data
		if e.CloudProvider == IMDSAWSCloudProvider {
			b := new(bytes.Buffer)
			_, err = io.Copy(b, resp.Body)
			if err == nil {
				_ = resp.Body.Close()
				// we don't care about errors, this unmarshalling will only work for token responses
				_ = e.AWS.SecurityCredentials.UnmarshalBinary(b.Bytes())
				if len(e.AWS.SecurityCredentials.ExpirationRaw) > 0 {
					e.AWS.SecurityCredentials.Expiration, _ = time.Parse(time.RFC3339, e.AWS.SecurityCredentials.ExpirationRaw)
				}
			}
		}
	case slices.Contains([]string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace,
	}, firstWord[0]):
		// this is an IMDS request
		e.Type = IMDSRequestType
		req, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(data)))
		if err != nil {
			return 0, fmt.Errorf("failed to parse IMDS request: %v", err)
		}
		e.URL = req.URL.String()
		e.fillFromIMDSHeader(req.Header, e.URL)
		e.Host = req.Host
		e.UserAgent = req.UserAgent()
	default:
		return 0, fmt.Errorf("invalid HTTP packet: unknown first word %s", firstWord[0])
	}

	return len(data), nil
}

func (e *IMDSEvent) fillFromIMDSHeader(header http.Header, url string) {
	if header != nil {
		e.Server = header.Get("Server")

		// guess the cloud provider from headers and the URL (this is a best effort resolution since some cloud provider
		// don't require any particular headers).
		if flavor := header.Get("Metadata-Flavor"); flavor == "Google" {
			e.CloudProvider = IMDSGCPCloudProvider
		} else if flavor == "ibm" {
			e.CloudProvider = IMDSIBMCloudProvider
		} else if authorization := header.Get("Authorization"); authorization == "Bearer Oracle" || strings.HasPrefix(url, "/opc") {
			e.CloudProvider = IMDSOracleCloudProvider
		} else if metadata := header.Get("Metadata"); metadata == "true" {
			e.CloudProvider = IMDSAzureCloudProvider
		} else {
			e.CloudProvider = IMDSAWSCloudProvider

			// check if this is an IMDSv2 request
			e.AWS.IsIMDSv2 = len(header.Get("x-aws-ec2-metadata-token-ttl-seconds")) > 0 ||
				len(header.Get("x-aws-ec2-metadata-token")) > 0
		}
	}
}

// UnmarshalBinary extract scrubbed data from an AWS IMDS security credentials response body
func (creds *AWSSecurityCredentials) UnmarshalBinary(body []byte) error {
	return json.Unmarshal(body, creds)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (d *NetDevice) UnmarshalBinary(data []byte) (int, error) {
	if len(data[:]) < 32 {
		return 0, ErrNotEnoughData
	}

	var err error
	d.Name, err = UnmarshalString(data[0:16], 16)
	if err != nil {
		return 0, err
	}
	d.NetNS = binary.NativeEndian.Uint32(data[16:20])
	d.IfIndex = binary.NativeEndian.Uint32(data[20:24])
	d.PeerNetNS = binary.NativeEndian.Uint32(data[24:28])
	d.PeerIfIndex = binary.NativeEndian.Uint32(data[28:32])
	return 32, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *NetDeviceEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}
	cursor := read

	read, err = e.Device.UnmarshalBinary(data[cursor:])
	if err != nil {
		return 0, err
	}
	cursor += read
	return cursor, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *VethPairEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}
	cursor := read

	read, err = e.HostDevice.UnmarshalBinary(data[cursor:])
	if err != nil {
		return 0, err
	}
	cursor += read

	read, err = e.PeerDevice.UnmarshalBinary(data[cursor:])
	if err != nil {
		return 0, err
	}
	cursor += read

	return cursor, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *BindEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 20 {
		return 0, ErrNotEnoughData
	}

	var ipRaw [16]byte
	SliceToArray(data[read:read+16], ipRaw[:])
	e.AddrFamily = binary.NativeEndian.Uint16(data[read+16 : read+18])
	e.Addr.Port = binary.BigEndian.Uint16(data[read+18 : read+20])

	// readjust IP size depending on the protocol
	switch e.AddrFamily {
	case 0x2: // unix.AF_INET
		e.Addr.IPNet = *eval.IPNetFromIP(ipRaw[0:4])
	case 0xa: // unix.AF_INET6
		e.Addr.IPNet = *eval.IPNetFromIP(ipRaw[:])
	}

	return read + 20, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ConnectEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 20 {
		return 0, ErrNotEnoughData
	}

	var ipRaw [16]byte
	SliceToArray(data[read:read+16], ipRaw[:])
	e.AddrFamily = binary.NativeEndian.Uint16(data[read+16 : read+18])
	e.Addr.Port = binary.BigEndian.Uint16(data[read+18 : read+20])

	// readjust IP size depending on the protocol
	switch e.AddrFamily {
	case 0x2: // unix.AF_INET
		e.Addr.IPNet = *eval.IPNetFromIP(ipRaw[0:4])
	case 0xa: // unix.AF_INET6
		e.Addr.IPNet = *eval.IPNetFromIP(ipRaw[:])
	}

	return read + 20, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SyscallsEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 72 {
		return 0, ErrNotEnoughData
	}

	e.EventReason = SyscallDriftEventReason(binary.NativeEndian.Uint64(data[0:8]))

	for i, b := range data[8:72] {
		// compute the ID of the syscall
		for j := 0; j < 8; j++ {
			if b&(1<<j) > 0 {
				e.Syscalls = append(e.Syscalls, Syscall(i*8+j))
			}
		}
	}
	return 72, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *OnDemandEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 260 {
		return 0, ErrNotEnoughData
	}

	e.ID = binary.NativeEndian.Uint32(data[0:4])
	SliceToArray(data[4:260], e.Data[:])
	return 260, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RawPacketEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := e.NetworkContext.Device.UnmarshalBinary(data)
	if err != nil {
		return 0, ErrNotEnoughData
	}
	data = data[read:]

	e.Size = binary.NativeEndian.Uint32(data)
	data = data[4:]
	e.Data = data
	e.CaptureInfo.InterfaceIndex = int(e.NetworkContext.Device.IfIndex)
	e.CaptureInfo.Length = int(e.NetworkContext.Size)
	e.CaptureInfo.CaptureLength = len(data)

	packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.DecodeOptions{NoCopy: true, Lazy: true, DecodeStreamsAsDatagrams: true})
	if layer := packet.Layer(layers.LayerTypeIPv4); layer != nil {
		if rl, ok := layer.(*layers.IPv4); ok {
			e.L3Protocol = unix.ETH_P_IP
			e.Source.IPNet = *eval.IPNetFromIP(rl.SrcIP)
			e.Destination.IPNet = *eval.IPNetFromIP(rl.DstIP)
		}
	} else if layer := packet.Layer(layers.LayerTypeIPv6); layer != nil {
		if rl, ok := layer.(*layers.IPv4); ok {
			e.L3Protocol = unix.ETH_P_IPV6
			e.Source.IPNet = *eval.IPNetFromIP(rl.SrcIP)
			e.Destination.IPNet = *eval.IPNetFromIP(rl.DstIP)
		}
	}

	if layer := packet.Layer(layers.LayerTypeUDP); layer != nil {
		if rl, ok := layer.(*layers.UDP); ok {
			e.L4Protocol = unix.IPPROTO_UDP
			e.Source.Port = uint16(rl.SrcPort)
			e.Destination.Port = uint16(rl.DstPort)
		}
	} else if layer := packet.Layer(layers.LayerTypeTCP); layer != nil {
		if rl, ok := layer.(*layers.TCP); ok {
			e.L4Protocol = unix.IPPROTO_TCP
			e.Source.Port = uint16(rl.SrcPort)
			e.Destination.Port = uint16(rl.DstPort)
		}
	}

	if layer := packet.Layer(layers.LayerTypeTLS); layer != nil {
		if rl, ok := layer.(*layers.TLS); ok {
			if len(rl.AppData) > 0 {
				e.TLSContext.Version = uint16(rl.AppData[0].Version)
			}
		}
	}

	return len(data), nil
}
