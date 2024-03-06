// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
)

const (
	idNameCreate       = uint16(10)
	idNameDelete       = uint16(11)
	idCreate           = uint16(12)
	idCleanup          = uint16(13)
	idClose            = uint16(14)
	idRead             = uint16(15)
	idWrite            = uint16(16)
	idSetInformation   = uint16(17)
	idSetDelete        = uint16(18)
	idRename           = uint16(19)
	idDirEnum          = uint16(20)
	idFlush            = uint16(21)
	idQueryInformation = uint16(22)
	idFSCTL            = uint16(23)
	idOperationEnd     = uint16(24)
	idDirNotify        = uint16(25)
	idDeletePath       = uint16(26)
	idRenamePath       = uint16(27)
	idSetLinkPath      = uint16(28)
	idRename29         = uint16(29)
	idCreateNewFile    = uint16(30)
)

type fileObjectPointer uint64

var (
	filePathResolver = make(map[fileObjectPointer]string, 0)
)

/*
		<template tid="CreateArgs">
	      <data name="Irp" inType="win:Pointer"/>
	      <data name="ThreadId" inType="win:Pointer"/>
	      <data name="FileObject" inType="win:Pointer"/>
	      <data name="CreateOptions" inType="win:UInt32"/>
	      <data name="CreateAttributes" inType="win:UInt32"/>
	      <data name="ShareAccess" inType="win:UInt32"/>
	      <data name="FileName" inType="win:UnicodeString"/>
	    </template>
*/
/*
 	<data name="Irp" inType="win:Pointer"/>
      <data name="FileObject" inType="win:Pointer"/>
      <data name="IssuingThreadId" inType="win:UInt32"/>
      <data name="CreateOptions" inType="win:UInt32"/>
      <data name="CreateAttributes" inType="win:UInt32"/>
      <data name="ShareAccess" inType="win:UInt32"/>
      <data name="FileName" inType="win:UnicodeString"/>
*/
type createHandleArgs struct {
	etw.DDEventHeader
	irp              uint64            // actually a pointer
	fileObject       fileObjectPointer // pointer
	threadID         uint64            // actually a pointer
	createOptions    uint32
	createAttributes uint32
	shareAccess      uint32
	fileName         string
}

/*
 * these constants are defined in the windows driver kit (wdm.h).  Copied
 * here because the correspond to the createOptions field
 */
const (
	kernelDisposition_FILE_SUPERSEDE           = uint32(0x00000000) // nolint:unused,revive
	kernelDisposition_FILE_OPEN                = uint32(0x00000001) // nolint:unused,revive
	kernelDisposition_FILE_CREATE              = uint32(0x00000002) // nolint:unused,revive
	kernelDisposition_FILE_OPEN_IF             = uint32(0x00000003) // nolint:unused,revive
	kernelDisposition_FILE_OVERWRITE           = uint32(0x00000004) // nolint:unused,revive
	kernelDisposition_FILE_OVERWRITE_IF        = uint32(0x00000005) // nolint:unused,revive
	kernelDisposition_FILE_MAXIMUM_DISPOSITION = uint32(0x00000005) // nolint:unused,revive
)

const (
	kernelCreateOpts_FILE_DIRECTORY_FILE            = uint32(0x00000001) // nolint:unused,revive
	kernelCreateOpts_FILE_WRITE_THROUGH             = uint32(0x00000002) // nolint:unused,revive
	kernelCreateOpts_FILE_SEQUENTIAL_ONLY           = uint32(0x00000004) // nolint:unused,revive
	kernelCreateOpts_FILE_NO_INTERMEDIATE_BUFFERING = uint32(0x00000008) // nolint:unused,revive

	kernelCreateOpts_FILE_SYNCHRONOUS_IO_ALERT    = uint32(0x00000010) // nolint:unused,revive
	kernelCreateOpts_FILE_SYNCHRONOUS_IO_NONALERT = uint32(0x00000020) // nolint:unused,revive
	kernelCreateOpts_FILE_NON_DIRECTORY_FILE      = uint32(0x00000040) // nolint:unused,revive
	kernelCreateOpts_FILE_CREATE_TREE_CONNECTION  = uint32(0x00000080) // nolint:unused,revive

	kernelCreateOpts_FILE_COMPLETE_IF_OPLOCKED = uint32(0x00000100) // nolint:unused,revive
	kernelCreateOpts_FILE_NO_EA_KNOWLEDGE      = uint32(0x00000200) // nolint:unused,revive
	kernelCreateOpts_FILE_OPEN_REMOTE_INSTANCE = uint32(0x00000400) // nolint:unused,revive
	kernelCreateOpts_FILE_RANDOM_ACCESS        = uint32(0x00000800) // nolint:unused,revive

	kernelCreateOpts_FILE_DELETE_ON_CLOSE        = uint32(0x00001000) // nolint:unused,revive
	kernelCreateOpts_FILE_OPEN_BY_FILE_ID        = uint32(0x00002000) // nolint:unused,revive
	kernelCreateOpts_FILE_OPEN_FOR_BACKUP_INTENT = uint32(0x00004000) // nolint:unused,revive
	kernelCreateOpts_FILE_NO_COMPRESSION         = uint32(0x00008000) // nolint:unused,revive
)

type createNewFileArgs createHandleArgs

/*
The Parameters.Create.Options member is a ULONG value that describes the options that are used

	when opening the handle. The high 8 bits correspond to the value of the CreateDisposition parameter
	of ZwCreateFile, and the low 24 bits correspond to the value of the CreateOptions parameter of ZwCreateFile.

The Parameters.Create.ShareAccess member is a USHORT value that describes the type of share access.
This value corresponds to the value of the ShareAccess parameter of ZwCreateFile.

The Parameters.Create.FileAttributes and Parameters.Create.EaLength members are reserved for use

	by file systems and file system filter drivers. For more information, see the IRP_MJ_CREATE topic in
	the Installable File System (IFS) documentation.
*/
func parseCreateHandleArgs(e *etw.DDEventRecord) (*createHandleArgs, error) {
	ca := &createHandleArgs{
		DDEventHeader: e.EventHeader,
	}
	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {
		ca.irp = data.GetUint64(0)
		ca.threadID = data.GetUint64(8)
		ca.fileObject = fileObjectPointer(data.GetUint64(16))
		ca.createOptions = data.GetUint32(24)
		ca.createAttributes = data.GetUint32(28)
		ca.shareAccess = data.GetUint32(32)

		ca.fileName, _, _, _ = data.ParseUnicodeString(36)
	} else if e.EventHeader.EventDescriptor.Version == 1 {

		ca.irp = data.GetUint64(0)
		ca.fileObject = fileObjectPointer(data.GetUint64(8))
		ca.threadID = uint64(data.GetUint32(16))
		ca.createOptions = data.GetUint32(20)
		ca.createAttributes = data.GetUint32(24)
		ca.shareAccess = data.GetUint32(28)

		ca.fileName, _, _, _ = data.ParseUnicodeString(32)
	} else {
		return nil, fmt.Errorf("unknown version %v", e.EventHeader.EventDescriptor.Version)
	}

	filePathResolver[ca.fileObject] = ca.fileName
	return ca, nil
}

func parseCreateNewFileArgs(e *etw.DDEventRecord) (*createNewFileArgs, error) {
	ca, err := parseCreateHandleArgs(e)
	if err != nil {
		return nil, err
	}
	return (*createNewFileArgs)(ca), nil
}

// nolint: unused
func (ca *createHandleArgs) String() string {
	var output strings.Builder

	output.WriteString("  Create PID: " + strconv.Itoa(int(ca.ProcessID)) + "\n")
	output.WriteString("         Name: " + ca.fileName + "\n")
	output.WriteString("         Opts: " + strconv.FormatUint(uint64(ca.createOptions), 16) + " Share: " + strconv.FormatUint(uint64(ca.shareAccess), 16) + "\n")
	output.WriteString("         OBJ:  " + strconv.FormatUint(uint64(ca.fileObject), 16) + "\n")
	return output.String()
}

// nolint: unused
func (ca *createNewFileArgs) String() string {
	return (*createHandleArgs)(ca).String()
}

/*
  <template tid="SetInformationArgs">
      <data name="Irp" inType="win:Pointer"/>
      <data name="ThreadId" inType="win:Pointer"/>
      <data name="FileObject" inType="win:Pointer"/>
      <data name="FileKey" inType="win:Pointer"/>
      <data name="ExtraInformation" inType="win:Pointer"/>
      <data name="InfoClass" inType="win:UInt32"/>
     </template>

	 <template tid="SetInformationArgs_V1">
      <data name="Irp" inType="win:Pointer"/>
      <data name="FileObject" inType="win:Pointer"/>
      <data name="FileKey" inType="win:Pointer"/>
      <data name="ExtraInformation" inType="win:Pointer"/>
      <data name="IssuingThreadId" inType="win:UInt32"/>
      <data name="InfoClass" inType="win:UInt32"/>
     </template>
*/

type setInformationArgs struct {
	etw.DDEventHeader
	irp        uint64
	threadID   uint64
	fileObject fileObjectPointer
	fileKey    uint64
	extraInfo  uint64
	infoClass  uint32
	fileName   string
}

func parseInformationArgs(e *etw.DDEventRecord) (*setInformationArgs, error) {
	sia := &setInformationArgs{
		DDEventHeader: e.EventHeader,
	}

	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {
		sia.irp = data.GetUint64(0)
		sia.threadID = data.GetUint64(8)
		sia.fileObject = fileObjectPointer(data.GetUint64(16))
		sia.fileKey = data.GetUint64(24)
		sia.extraInfo = data.GetUint64(32)
		sia.infoClass = data.GetUint32(40)
	} else if e.EventHeader.EventDescriptor.Version == 1 {
		sia.irp = data.GetUint64(0)
		sia.fileObject = fileObjectPointer(data.GetUint64(8))
		sia.fileKey = data.GetUint64(16)
		sia.extraInfo = data.GetUint64(24)
		sia.threadID = uint64(data.GetUint32(32))
		sia.infoClass = data.GetUint32(36)
	} else {
		return nil, fmt.Errorf("unknown version number %v", e.EventHeader.EventDescriptor.Version)
	}
	if s, ok := filePathResolver[fileObjectPointer(sia.fileObject)]; ok {
		sia.fileName = s
	}
	return sia, nil
}

// nolint: unused
func (sia *setInformationArgs) String() string {
	var output strings.Builder

	output.WriteString("  SIA TID: " + strconv.Itoa(int(sia.threadID)) + "\n")
	output.WriteString("      Name: " + sia.fileName + "\n")
	output.WriteString("      InfoClass: " + strconv.FormatUint(uint64(sia.infoClass), 16) + "\n")
	return output.String()

}

/*
	<template tid="CleanupArgs">
      <data name="Irp" inType="win:Pointer"/>
      <data name="threadID" inType="win:Pointer"/>
      <data name="FileObject" inType="win:Pointer"/>
      <data name="FileKey" inType="win:Pointer"/>
     </template>

 	<template tid="CleanupArgs_V1">
      <data name="Irp" inType="win:Pointer"/>
      <data name="FileObject" inType="win:Pointer"/>
      <data name="FileKey" inType="win:Pointer"/>
      <data name="IssuingThreadId" inType="win:UInt32"/>
     </template>
*/

type cleanupArgs struct {
	etw.DDEventHeader
	irp        uint64
	threadID   uint64
	fileObject fileObjectPointer
	fileKey    uint64
	fileName   string
}

// nolint: unused
type closeArgs cleanupArgs

// nolint: unused
type flushArgs cleanupArgs

func parseCleanupArgs(e *etw.DDEventRecord) (*cleanupArgs, error) {
	ca := &cleanupArgs{
		DDEventHeader: e.EventHeader,
	}
	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {
		ca.irp = data.GetUint64(0)
		ca.threadID = data.GetUint64(8)
		ca.fileObject = fileObjectPointer(data.GetUint64(16))
		ca.fileKey = data.GetUint64(24)

	} else if e.EventHeader.EventDescriptor.Version == 1 {
		ca.irp = data.GetUint64(0)
		ca.fileObject = fileObjectPointer(data.GetUint64(8))
		ca.fileKey = data.GetUint64(16)
		ca.threadID = uint64(data.GetUint32(24))
	} else {
		return nil, fmt.Errorf("unknown version number %v", e.EventHeader.EventDescriptor.Version)
	}
	if s, ok := filePathResolver[ca.fileObject]; ok {
		ca.fileName = s

	}
	return ca, nil
}

// nolint: unused
func parseCloseArgs(e *etw.DDEventRecord) (*closeArgs, error) {
	ca, err := parseCleanupArgs(e)
	if err != nil {
		return nil, err
	}
	return (*closeArgs)(ca), nil
}

// nolint: unused
func parseFlushArgs(e *etw.DDEventRecord) (*flushArgs, error) {
	ca, err := parseCleanupArgs(e)
	if err != nil {
		return nil, err
	}
	return (*flushArgs)(ca), nil
}

// nolint: unused
func (ca *cleanupArgs) String() string {
	var output strings.Builder

	output.WriteString("  CLEANUP: TID: " + strconv.Itoa(int(ca.threadID)) + "\n")
	output.WriteString("           Name: " + ca.fileName + "\n")
	output.WriteString("         OBJ:  " + strconv.FormatUint(uint64(ca.fileObject), 16) + "\n")
	return output.String()

}

// nolint: unused
func (ca *closeArgs) String() string {
	return (*cleanupArgs)(ca).String()
}

// nolint: unused
func (fa *flushArgs) String() string {
	return (*cleanupArgs)(fa).String()
}
