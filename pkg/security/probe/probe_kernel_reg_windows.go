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
	idRegCreateKey     = uint16(1)  // CreateKeyArgs
	idRegOpenKey       = uint16(2)  // CraeteKeyArgs
	idRegDeleteKey     = uint16(3)  // DeleteKeyArgs
	idRegSetValueKey   = uint16(5)  // setValueKeyArgs
	idRegFlushKey      = uint16(12) // deleteKeyArgs
	idRegCloseKey      = uint16(13) // deleteKeyArgs
	idQuerySecurityKey = uint16(14) // deleteKeyArgs
	idSetSecurityKey   = uint16(15) // deleteKeyArgs

)

/*
<template tid="task_0CreateKeyArgs">
      <data name="BaseObject" inType="win:Pointer"/>
      <data name="KeyObject" inType="win:Pointer"/>
      <data name="Status" inType="win:UInt32"/>
      <data name="Disposition" inType="win:UInt32"/>
      <data name="BaseName" inType="win:UnicodeString"/>
      <data name="RelativeName" inType="win:UnicodeString"/>
     </template>
*/

type createKeyArgs struct {
	baseObject   uint64 // pointer
	keyObject    uint64 //pointer
	status       uint32
	disposition  uint32
	baseName     string
	relativeName string
}

/*
		<template tid="task_0DeleteKeyArgs">
	      <data name="KeyObject" inType="win:Pointer"/>
	      <data name="Status" inType="win:UInt32"/>
	      <data name="KeyName" inType="win:UnicodeString"/>
	     </template>
*/
type deleteKeyArgs struct {
	keyObject uint64
	status    uint32
	keyName   string
}

/*
<template tid="task_0SetValueKeyArgs">

	<data name="KeyObject" inType="win:Pointer"/>
	<data name="Status" inType="win:UInt32"/>
	<data name="Type" inType="win:UInt32"/>
	<data name="DataSize" inType="win:UInt32"/>
	<data name="KeyName" inType="win:UnicodeString"/>
	<data name="ValueName" inType="win:UnicodeString"/>
	<data name="CapturedDataSize" inType="win:UInt16"/>
	<data name="CapturedData" inType="win:Binary" length="CapturedDataSize"/>
	<data name="PreviousDataType" inType="win:UInt32"/>
	<data name="PreviousDataSize" inType="win:UInt32"/>
	<data name="PreviousDataCapturedSize" inType="win:UInt16"/>
	<data name="PreviousData" inType="win:Binary" length="PreviousDataCapturedSize"/>

</template>
*/
type setValueKeyArgs struct {
	keyObject                uint64
	status                   uint32
	dataType                 uint32
	dataSize                 uint32
	keyName                  string
	valueName                string
	capturedDataSize         uint16
	capturedData             []byte
	previousDataType         uint32
	previousDataSize         uint32
	capturedPreviousDataSize uint16
	previousData             []byte
}

func parseCreateRegistryKey(e *etw.DDEventRecord) (*createKeyArgs, error) {

	crc := &createKeyArgs{}

	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	crc.baseObject = binary.LittleEndian.Uint64(data[0:8])
	crc.keyObject = binary.LittleEndian.Uint64(data[8:16])
	crc.status = binary.LittleEndian.Uint32(data[16:20])
	crc.disposition = binary.LittleEndian.Uint32(data[20:24])

	//var nextOffset int
	//var nulltermidx int
	var nextOffset int
	crc.baseName, nextOffset, _, _ = etwutil.ParseUnicodeString(data, 24)
	if nextOffset == -1 {
		nextOffset = 26
	}
	crc.relativeName, _, _, _ = etwutil.ParseUnicodeString(data, nextOffset)

	return crc, nil
}

func parseDeleteRegistryKey(e *etw.DDEventRecord) (*deleteKeyArgs, error) {

	dka := &deleteKeyArgs{}

	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	dka.keyObject = binary.LittleEndian.Uint64(data[0:8])
	dka.status = binary.LittleEndian.Uint32(data[8:12])
	dka.keyName, _, _, _ = etwutil.ParseUnicodeString(data, 12)

	return dka, nil
}

func parseSetValueKey(e *etw.DDEventRecord) (*setValueKeyArgs, error) {

	sv := &setValueKeyArgs{}

	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	sv.keyObject = binary.LittleEndian.Uint64(data[0:8])
	sv.status = binary.LittleEndian.Uint32(data[8:12])
	sv.dataType = binary.LittleEndian.Uint32(data[12:16])
	sv.dataSize = binary.LittleEndian.Uint32(data[16:20])
	var nextOffset int
	var thisNextOffset int
	sv.keyName, nextOffset, _, _ = etwutil.ParseUnicodeString(data, 20)
	if nextOffset == -1 {
		nextOffset = 22
	}
	sv.valueName, thisNextOffset, _, _ = etwutil.ParseUnicodeString(data, nextOffset)
	if thisNextOffset == -1 {
		nextOffset += 2
	} else {
		nextOffset = thisNextOffset
	}

	sv.capturedDataSize = binary.LittleEndian.Uint16(data[nextOffset : nextOffset+2])
	nextOffset += 2

	// make a copy of the data because the underlying buffer here belongs to etw
	sv.capturedData = data[nextOffset : nextOffset+int(sv.capturedDataSize)]
	nextOffset += int(sv.capturedDataSize)

	sv.previousDataType = binary.LittleEndian.Uint32(data[nextOffset : nextOffset+4])
	nextOffset += 4

	sv.previousDataSize = binary.LittleEndian.Uint32(data[nextOffset : nextOffset+4])
	nextOffset += 4

	sv.previousData = data[nextOffset : nextOffset+int(sv.previousDataSize)]

	return sv, nil
}
