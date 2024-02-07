// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwutil "github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
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
	irp              uint64            // actually a pointer
	fileObject       fileObjectPointer // pointer
	threadID         uint64            // actually a pointer
	createOptions    uint32
	createAttributes uint32
	shareAccess      uint32
	fileName         string
}

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
	ca := &createHandleArgs{}
	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))
	if e.EventHeader.EventDescriptor.Version == 0 {
		ca.irp = binary.LittleEndian.Uint64(data[0:8])
		ca.threadID = binary.LittleEndian.Uint64(data[8:16])
		ca.fileObject = fileObjectPointer(binary.LittleEndian.Uint64(data[16:24]))
		ca.createOptions = binary.LittleEndian.Uint32(data[24:28])
		ca.createAttributes = binary.LittleEndian.Uint32(data[28:32])
		ca.shareAccess = binary.LittleEndian.Uint32(data[32:36])

		ca.fileName, _, _, _ = etwutil.ParseUnicodeString(data, 36)
	} else if e.EventHeader.EventDescriptor.Version == 1 {

		ca.irp = binary.LittleEndian.Uint64(data[0:8])
		ca.fileObject = fileObjectPointer(binary.LittleEndian.Uint64(data[8:16]))
		ca.threadID = uint64(binary.LittleEndian.Uint32(data[16:20]))
		ca.createOptions = binary.LittleEndian.Uint32(data[20:24])
		ca.createAttributes = binary.LittleEndian.Uint32(data[24:38])
		ca.shareAccess = binary.LittleEndian.Uint32(data[28:32])

		ca.fileName, _, _, _ = etwutil.ParseUnicodeString(data, 32)
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
func (ca *createHandleArgs) string() string {
	var output strings.Builder

	output.WriteString("  Create TID: " + strconv.Itoa(int(ca.threadID)) + "\n")
	output.WriteString("         Name: " + ca.fileName + "\n")
	output.WriteString("         Opts: " + strconv.FormatUint(uint64(ca.createOptions), 16) + " Attrs: " + strconv.FormatUint(uint64(ca.createAttributes), 16) + " Share: " + strconv.FormatUint(uint64(ca.shareAccess), 16) + "\n")
	output.WriteString("         OBJ:  " + strconv.FormatUint(uint64(ca.fileObject), 16) + "\n")
	return output.String()
}

// nolint: unused
func (ca *createNewFileArgs) string() string {
	return (*createHandleArgs)(ca).string()
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
	irp        uint64
	threadID   uint64
	fileObject fileObjectPointer
	fileKey    uint64
	extraInfo  uint64
	infoClass  uint32
	fileName   string
}

func parseInformationArgs(e *etw.DDEventRecord) (*setInformationArgs, error) {
	sia := &setInformationArgs{}
	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	if e.EventHeader.EventDescriptor.Version == 0 {
		sia.irp = binary.LittleEndian.Uint64(data[0:8])
		sia.threadID = binary.LittleEndian.Uint64(data[8:16])
		sia.fileObject = fileObjectPointer(binary.LittleEndian.Uint64(data[16:24]))
		sia.fileKey = binary.LittleEndian.Uint64(data[24:32])
		sia.extraInfo = binary.LittleEndian.Uint64(data[32:40])
		sia.infoClass = binary.LittleEndian.Uint32(data[40:44])
	} else if e.EventHeader.EventDescriptor.Version == 1 {
		sia.irp = binary.LittleEndian.Uint64(data[0:8])
		sia.fileObject = fileObjectPointer(binary.LittleEndian.Uint64(data[8:16]))
		sia.fileKey = binary.LittleEndian.Uint64(data[16:24])
		sia.extraInfo = binary.LittleEndian.Uint64(data[24:32])
		sia.threadID = uint64(binary.LittleEndian.Uint32(data[32:36]))
		sia.infoClass = binary.LittleEndian.Uint32(data[36:40])
	} else {
		return nil, fmt.Errorf("unknown version number %v", e.EventHeader.EventDescriptor.Version)
	}
	if s, ok := filePathResolver[fileObjectPointer(sia.fileObject)]; ok {
		sia.fileName = s
	}
	return sia, nil
}

// nolint: unused
func (sia *setInformationArgs) string() string {
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
	ca := &cleanupArgs{}
	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	if e.EventHeader.EventDescriptor.Version == 0 {
		ca.irp = binary.LittleEndian.Uint64(data[0:8])
		ca.threadID = binary.LittleEndian.Uint64(data[8:16])
		ca.fileObject = fileObjectPointer(binary.LittleEndian.Uint64(data[16:24]))
		ca.fileKey = binary.LittleEndian.Uint64(data[24:32])

	} else if e.EventHeader.EventDescriptor.Version == 1 {
		ca.irp = binary.LittleEndian.Uint64(data[0:8])
		ca.fileObject = fileObjectPointer(binary.LittleEndian.Uint64(data[8:16]))
		ca.fileKey = binary.LittleEndian.Uint64(data[16:24])
		ca.threadID = uint64(binary.LittleEndian.Uint32(data[24:28]))
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
func (ca *cleanupArgs) string() string {
	var output strings.Builder

	output.WriteString("  CLEANUP: TID: " + strconv.Itoa(int(ca.threadID)) + "\n")
	output.WriteString("           Name: " + ca.fileName + "\n")
	output.WriteString("         OBJ:  " + strconv.FormatUint(uint64(ca.fileObject), 16) + "\n")
	return output.String()

}

// nolint: unused
func (ca *closeArgs) string() string {
	return (*cleanupArgs)(ca).string()
}

// nolint: unused
func (fa *flushArgs) string() string {
	return (*cleanupArgs)(fa).string()
}
