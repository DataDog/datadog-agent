// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// BinaryUnmarshaler interface implemented by every event type
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) (int, error)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ContainerContext) UnmarshalBinary(data []byte) (int, error) {
	id, err := UnmarshalString(data, ContainerIDLen)
	if err != nil {
		return 0, err
	}
	e.ID = id

	return ContainerIDLen, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ChmodEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ChownEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	// First convert to int32 to sign extend, then convert to int64
	e.UID = int64(int32(ByteOrder.Uint32(data[0:4])))
	e.GID = int64(int32(ByteOrder.Uint32(data[4:8])))
	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *Event) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}

	e.TimestampRaw = ByteOrder.Uint64(data[8:16])
	e.Type = ByteOrder.Uint32(data[16:20])
	if data[20] != 0 {
		e.Async = true
	} else {
		e.Async = false
	}
	// 21-24: padding

	return 24, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SetuidEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.UID = ByteOrder.Uint32(data[0:4])
	e.EUID = ByteOrder.Uint32(data[4:8])
	e.FSUID = ByteOrder.Uint32(data[8:12])
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SetgidEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.GID = ByteOrder.Uint32(data[0:4])
	e.EGID = ByteOrder.Uint32(data[4:8])
	e.FSGID = ByteOrder.Uint32(data[8:12])
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *CapsetEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.CapEffective = ByteOrder.Uint64(data[0:8])
	e.CapPermitted = ByteOrder.Uint64(data[8:16])
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *Credentials) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 40 {
		return 0, ErrNotEnoughData
	}

	e.UID = ByteOrder.Uint32(data[0:4])
	e.GID = ByteOrder.Uint32(data[4:8])
	e.EUID = ByteOrder.Uint32(data[8:12])
	e.EGID = ByteOrder.Uint32(data[12:16])
	e.FSUID = ByteOrder.Uint32(data[16:20])
	e.FSGID = ByteOrder.Uint32(data[20:24])
	e.CapEffective = ByteOrder.Uint64(data[24:32])
	e.CapPermitted = ByteOrder.Uint64(data[32:40])
	return 40, nil
}

func unmarshalTime(data []byte) time.Time {
	if t := int64(ByteOrder.Uint64(data)); t != 0 {
		return time.Unix(0, t)
	}
	return time.Time{}
}

// isValidTTYName uses a naive assumption as other tty driver may create tty with other prefix
func isValidTTYName(ttyName string) bool {
	return IsPrintableASCII(ttyName) && (strings.HasPrefix(ttyName, "tty") || strings.HasPrefix(ttyName, "pts"))
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *Process) UnmarshalBinary(data []byte) (int, error) {
	// Unmarshal proc_cache_t
	read, err := UnmarshalBinary(data, &e.FileEvent)
	if err != nil {
		return 0, err
	}

	if len(data[read:]) < 112 {
		return 0, ErrNotEnoughData
	}

	e.ExecTime = unmarshalTime(data[read : read+8])
	read += 8

	var ttyRaw [64]byte
	SliceToArray(data[read:read+64], unsafe.Pointer(&ttyRaw))
	ttyName, err := UnmarshalString(ttyRaw[:], 64)
	if err != nil {
		return 0, err
	}
	if isValidTTYName(ttyName) {
		e.TTYName = ttyName
	}
	read += 64

	var commRaw [16]byte
	SliceToArray(data[read:read+16], unsafe.Pointer(&commRaw))
	e.Comm, err = UnmarshalString(commRaw[:], 16)
	if err != nil {
		return 0, err
	}
	read += 16

	// Unmarshal pid_cache_t
	cookie := ByteOrder.Uint32(data[read : read+4])
	if cookie > 0 {
		e.Cookie = cookie
	}
	e.PPid = ByteOrder.Uint32(data[read+4 : read+8])

	e.ForkTime = unmarshalTime(data[read+8 : read+16])
	e.ExitTime = unmarshalTime(data[read+16 : read+24])
	read += 24

	// Unmarshal the credentials contained in pid_cache_t
	n, err := UnmarshalBinary(data[read:], &e.Credentials)
	if err != nil {
		return 0, err
	}
	read += n

	if len(data[read:]) < 16 {
		return 0, ErrNotEnoughData
	}

	e.ArgsID = ByteOrder.Uint32(data[read : read+4])
	e.ArgsTruncated = ByteOrder.Uint32(data[read+4:read+8]) == 1
	read += 8

	e.EnvsID = ByteOrder.Uint32(data[read : read+4])
	e.EnvsTruncated = ByteOrder.Uint32(data[read+4:read+8]) == 1
	read += 8

	return read, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ExitEvent) UnmarshalBinary(data []byte) (int, error) {
	// Unmarshal exit code
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	exitStatus := ByteOrder.Uint32(data[0:4])
	if exitStatus&0x7F == 0x00 { // process terminated normally
		e.Cause = uint32(ExitExited)
		e.Code = uint32(exitStatus>>8) & 0xFF
	} else if exitStatus&0x7F != 0x7F { // process terminated because of a signal
		if exitStatus&0x80 == 0x80 { // coredump signal
			e.Cause = uint32(ExitCoreDumped)
			e.Code = uint32(exitStatus & 0x7F)
		} else { // other signals
			e.Cause = uint32(ExitSignaled)
			e.Code = uint32(exitStatus & 0x7F)
		}
	}

	return 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *InvalidateDentryEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	e.Inode = ByteOrder.Uint64(data[0:8])
	e.MountID = ByteOrder.Uint32(data[8:12])
	// padding

	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *ArgsEnvsEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < maxArgEnvSize+8 {
		return 0, ErrNotEnoughData
	}

	e.ID = ByteOrder.Uint32(data[0:4])
	e.Size = ByteOrder.Uint32(data[4:8])
	if e.Size > maxArgEnvSize {
		e.Size = maxArgEnvSize
	}
	SliceToArray(data[8:maxArgEnvSize+8], unsafe.Pointer(&e.ValuesRaw))

	return maxArgEnvSize + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *FileFields) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 72 {
		return 0, ErrNotEnoughData
	}
	e.Inode = ByteOrder.Uint64(data[0:8])
	e.MountID = ByteOrder.Uint32(data[8:12])
	e.PathID = ByteOrder.Uint32(data[12:16])
	e.Flags = int32(ByteOrder.Uint32(data[16:20]))

	// +4 for padding

	e.UID = ByteOrder.Uint32(data[24:28])
	e.GID = ByteOrder.Uint32(data[28:32])
	e.NLink = ByteOrder.Uint32(data[32:36])
	e.Mode = ByteOrder.Uint16(data[36:38])

	// +2 for padding

	timeSec := ByteOrder.Uint64(data[40:48])
	timeNsec := ByteOrder.Uint64(data[48:56])
	e.CTime = uint64(time.Unix(int64(timeSec), int64(timeNsec)).UnixNano())

	timeSec = ByteOrder.Uint64(data[56:64])
	timeNsec = ByteOrder.Uint64(data[64:72])
	e.MTime = uint64(time.Unix(int64(timeSec), int64(timeNsec)).UnixNano())

	return 72, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *FileEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.FileFields)
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *LinkEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.Source, &e.Target)
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

	e.Mode = ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *MountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 56 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ByteOrder.Uint32(data[0:4])
	e.GroupID = ByteOrder.Uint32(data[4:8])
	e.Device = ByteOrder.Uint32(data[8:12])
	e.ParentMountID = ByteOrder.Uint32(data[12:16])
	e.ParentInode = ByteOrder.Uint64(data[16:24])
	e.RootInode = ByteOrder.Uint64(data[24:32])
	e.RootMountID = ByteOrder.Uint32(data[32:36])

	// Notes: bytes 36 to 40 are used to pad the structure

	SliceToArray(data[40:56], unsafe.Pointer(&e.FSTypeRaw))
	e.FSType, err = UnmarshalString(e.FSTypeRaw[:], 16)
	if err != nil {
		return 0, err
	}

	return 56, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.Flags = ByteOrder.Uint32(data[0:4])
	e.Mode = ByteOrder.Uint32(data[4:8])
	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (s *SpanContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	s.SpanID = ByteOrder.Uint64(data[0:8])
	s.TraceID = ByteOrder.Uint64(data[8:16])

	return 16, nil
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

	e.EventKind = SELinuxEventKind(ByteOrder.Uint32(data[0:4]))

	switch e.EventKind {
	case SELinuxBoolChangeEventKind:
		boolValue := ByteOrder.Uint32(data[4:8])
		if boolValue == ^uint32(0) {
			e.BoolChangeValue = "error"
		} else if boolValue > 0 {
			e.BoolChangeValue = "on"
		} else {
			e.BoolChangeValue = "off"
		}
	case SELinuxBoolCommitEventKind:
		boolValue := ByteOrder.Uint32(data[4:8])
		e.BoolCommitValue = boolValue != 0
	case SELinuxStatusChangeEventKind:
		disableValue := ByteOrder.Uint16(data[4:6]) != 0
		enforceValue := ByteOrder.Uint16(data[6:8]) != 0
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

// UnmarshalBinary unmarshalls a binary representation of itself
func (p *PIDContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	p.Pid = ByteOrder.Uint32(data[0:4])
	p.Tid = ByteOrder.Uint32(data[4:8])
	p.NetNS = ByteOrder.Uint32(data[8:12])
	// padding (4 bytes)
	return 16, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.Old, &e.New)
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
	SliceToArray(data[0:200], unsafe.Pointer(&e.NameRaw))

	return n + 200, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *SyscallEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.Retval = int64(ByteOrder.Uint64(data[0:8]))
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

	e.MountID = ByteOrder.Uint32(data[0:4])

	return 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.Flags = ByteOrder.Uint32(data[0:4])
	// padding

	return n + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UtimesEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 32 {
		return 0, ErrNotEnoughData
	}

	timeSec := ByteOrder.Uint64(data[0:8])
	timeNsec := ByteOrder.Uint64(data[8:16])
	e.Atime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = ByteOrder.Uint64(data[16:24])
	timeNsec = ByteOrder.Uint64(data[24:32])
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
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ByteOrder.Uint32(data[0:4])
	e.DiscarderRevision = ByteOrder.Uint32(data[4:8])

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
	e.Cmd = ByteOrder.Uint32(data[cursor : cursor+4])
	return cursor + 4, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (m *BPFMap) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}
	m.ID = ByteOrder.Uint32(data[0:4])
	m.Type = ByteOrder.Uint32(data[4:8])

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
	p.ID = ByteOrder.Uint32(data[0:4])
	p.Type = ByteOrder.Uint32(data[4:8])
	p.AttachType = ByteOrder.Uint32(data[8:12])
	// padding
	helpers := []uint64{0, 0, 0}
	helpers[0] = ByteOrder.Uint64(data[16:24])
	helpers[1] = ByteOrder.Uint64(data[24:32])
	helpers[2] = ByteOrder.Uint64(data[32:40])
	p.Helpers = parseHelpers(helpers)

	var err error
	p.Name, err = UnmarshalString(data[40:56], 16)
	if err != nil {
		return 0, err
	}
	for _, b := range data[56:64] {
		p.Tag += fmt.Sprintf("%x", b)
	}
	return 64, nil
}

func parseHelpers(helpers []uint64) []uint32 {
	var rep []uint32
	var add bool

	if len(helpers) < 3 {
		return rep
	}

	for i := 0; i < 192; i++ {
		add = false
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

	e.Request = ByteOrder.Uint32(data[read : read+4])
	e.PID = ByteOrder.Uint32(data[read+4 : read+8])
	e.Address = ByteOrder.Uint64(data[read+8 : read+16])
	return read + 16, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MMapEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 28 {
		return 0, ErrNotEnoughData
	}

	e.Addr = ByteOrder.Uint64(data[read : read+8])
	e.Offset = ByteOrder.Uint64(data[read+8 : read+16])
	e.Len = ByteOrder.Uint32(data[read+16 : read+20])
	e.Protection = int(ByteOrder.Uint32(data[read+20 : read+24]))
	e.Flags = int(ByteOrder.Uint32(data[read+24 : read+28]))
	return read + 28, nil
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

	e.VMStart = ByteOrder.Uint64(data[read : read+8])
	e.VMEnd = ByteOrder.Uint64(data[read+8 : read+16])
	e.VMProtection = int(ByteOrder.Uint32(data[read+16 : read+24]))
	e.ReqProtection = int(ByteOrder.Uint32(data[read+24 : read+32]))
	return read + 32, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *LoadModuleEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 60 {
		return 0, ErrNotEnoughData
	}

	e.Name, err = UnmarshalString(data[read:read+56], 56)
	if err != nil {
		return 0, err
	}
	e.LoadedFromMemory = ByteOrder.Uint32(data[read+56:read+60]) == uint32(1)
	return read + 60, nil
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

	e.PID = ByteOrder.Uint32(data[read : read+4])
	e.Type = ByteOrder.Uint32(data[read+4 : read+8])
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

	e.PipeEntryFlag = ByteOrder.Uint32(data[read : read+4])
	e.PipeExitFlag = ByteOrder.Uint32(data[read+4 : read+8])
	return read + 4, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *CgroupTracingEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.ContainerContext)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 8 {
		return 0, ErrNotEnoughData
	}

	e.TimeoutRaw = ByteOrder.Uint64(data[read : read+8])
	return read + 8, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *NetworkDeviceContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.NetNS = ByteOrder.Uint32(data[0:4])
	e.IfIndex = ByteOrder.Uint32(data[4:8])
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

	srcIP, dstIP := data[read:read+16], data[read+16:read+32]
	e.Source.Port = binary.BigEndian.Uint16(data[read+32 : read+34])
	e.Destination.Port = binary.BigEndian.Uint16(data[read+34 : read+36])
	// padding 4 bytes

	e.Size = ByteOrder.Uint32(data[read+40 : read+44])
	e.L3Protocol = ByteOrder.Uint16(data[read+44 : read+46])
	e.L4Protocol = ByteOrder.Uint16(data[read+46 : read+48])

	// readjust IP sizes depending on the protocol
	switch e.L3Protocol {
	case 0x800: // unix.ETH_P_IP
		e.Source.IPNet = *eval.IPNetFromIP(srcIP[0:4])
		e.Destination.IPNet = *eval.IPNetFromIP(dstIP[0:4])
	default:
		e.Source.IPNet = *eval.IPNetFromIP(srcIP)
		e.Destination.IPNet = *eval.IPNetFromIP(dstIP)
	}
	return read + 48, nil
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *DNSEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 10 {
		return 0, ErrNotEnoughData
	}

	e.ID = ByteOrder.Uint16(data[0:2])
	e.Count = ByteOrder.Uint16(data[2:4])
	e.Type = ByteOrder.Uint16(data[4:6])
	e.Class = ByteOrder.Uint16(data[6:8])
	e.Size = ByteOrder.Uint16(data[8:10])
	e.Name = decodeDNS(data[10:])
	return len(data), nil
}

func decodeDNS(raw []byte) string {
	rawLen := len(raw)
	rep := ""
	i := 0
	for {
		// Parse label length
		if rawLen < i+1 {
			break
		}
		labelLen := int(raw[i])

		if rawLen-(i+1) < labelLen || labelLen == 0 {
			break
		}
		labelRaw := raw[i+1 : i+1+labelLen]

		if i == 0 {
			rep = string(labelRaw)
		} else {
			rep = rep + "." + string(labelRaw)
		}
		i += labelLen + 1
	}
	return rep
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
	d.NetNS = ByteOrder.Uint32(data[16:20])
	d.IfIndex = ByteOrder.Uint32(data[20:24])
	d.PeerNetNS = ByteOrder.Uint32(data[24:28])
	d.PeerIfIndex = ByteOrder.Uint32(data[28:32])
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

// UnmarshalBinary unmarshals a binary representation of itself
func (e *BindEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return 0, err
	}

	if len(data)-read < 20 {
		return 0, ErrNotEnoughData
	}

	ipRaw := data[read : read+16]
	e.AddrFamily = ByteOrder.Uint16(data[read+16 : read+18])
	e.Addr.Port = binary.BigEndian.Uint16(data[read+18 : read+20])

	// readjust IP size depending on the protocol
	switch e.AddrFamily {
	case 0x2: // unix.AF_INET
		e.Addr.IPNet = *eval.IPNetFromIP(ipRaw[0:4])
	case 0xa: // unix.AF_INET6
		e.Addr.IPNet = *eval.IPNetFromIP(ipRaw)
	}

	return read + 20, nil
}
