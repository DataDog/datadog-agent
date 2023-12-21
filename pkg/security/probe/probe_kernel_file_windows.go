// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"encoding/binary"
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
type createArgs struct {
	irp              uint64 // actually a pointer
	fileObject       uint64 // pointer
	threadid         uint32 // actually a pointer
	createOptions    uint32
	createAttributes uint32
	shareAccess      uint32
	fileName         string
}

func parseCreateArgs(e *etw.DDEventRecord) (*createArgs, error) {
	ca := &createArgs{}

	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))
	ca.irp = binary.LittleEndian.Uint64(data[0:8])
	ca.fileObject = binary.LittleEndian.Uint64(data[8:16])
	ca.threadid = binary.LittleEndian.Uint32(data[16:20])
	ca.createOptions = binary.LittleEndian.Uint32(data[20:24])
	ca.createAttributes = binary.LittleEndian.Uint32(data[24:38])
	ca.shareAccess = binary.LittleEndian.Uint32(data[28:32])

	ca.fileName, _, _, _ = etwutil.ParseUnicodeString(data, 32)

	return ca, nil
}
