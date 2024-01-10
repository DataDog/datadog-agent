// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"encoding/binary"
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/etw"
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

type regObjectPointer uint64

var (
	regPathResolver = make(map[regObjectPointer]string, 0)
	regprefix        = `\REGISTRY`
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
	etw.DDEventHeader
	baseObject       regObjectPointer // pointer
	keyObject        regObjectPointer //pointer
	status           uint32
	disposition      uint32
	baseName         string
	relativeName     string
	computedFullPath string
}

/*
		<template tid="task_0DeleteKeyArgs">
	      <data name="KeyObject" inType="win:Pointer"/>
	      <data name="Status" inType="win:UInt32"/>
	      <data name="KeyName" inType="win:UnicodeString"/>
	     </template>
*/
type deleteKeyArgs struct {
	etw.DDEventHeader
	keyObject regObjectPointer
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
	etw.DDEventHeader
	keyObject                regObjectPointer
	status                   uint32
	dataType                 uint32
	dataSize                 uint32
	keyName                  string
	valueName                string
	capturedDataSize         uint16
	capturedData             []byte
	previousDataType         uint32
	previousDataSize         uint32
	capturedPreviousDataSize uint16 //nolint:golint,unused
	previousData             []byte
	computedFullPath         string
}

func parseCreateRegistryKey(e *etw.DDEventRecord) (*createKeyArgs, error) {

	crc := &createKeyArgs{
		DDEventHeader: e.EventHeader,
	}
	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	crc.baseObject = regObjectPointer(binary.LittleEndian.Uint64(data[0:8]))
	crc.keyObject = regObjectPointer(binary.LittleEndian.Uint64(data[8:16]))
	crc.status = binary.LittleEndian.Uint32(data[16:20])
	crc.disposition = binary.LittleEndian.Uint32(data[20:24])

	//var nextOffset int
	//var nulltermidx int
	var nextOffset int
	crc.baseName, nextOffset, _, _ = parseUnicodeString(data, 24)
	if nextOffset == -1 {
		nextOffset = 26
	}
	crc.relativeName, _, _, _ = parseUnicodeString(data, nextOffset)

	crc.computeFullPath()
	return crc, nil
}
func (cka *createKeyArgs) computeFullPath() {

	// var regPathResolver map[regObjectPointer]string

	if strings.HasPrefix(cka.relativeName, regprefix) {
		cka.computedFullPath = cka.relativeName
		regPathResolver[cka.keyObject] = cka.relativeName
		return
	}
	if s, ok := regPathResolver[cka.keyObject]; ok {
		cka.computedFullPath = s
	}
	var outstr string
	if cka.baseObject == 0 {
		outstr = cka.baseName + "\\" + cka.relativeName
	} else {

		if s, ok := regPathResolver[cka.baseObject]; ok {
			outstr = s + "\\" + cka.relativeName
		} else {
			outstr = "\\" + cka.relativeName
		}
	}
	regPathResolver[cka.keyObject] = outstr
	cka.computedFullPath = outstr
}
func (cka *createKeyArgs) string() string {

	var output strings.Builder

	output.WriteString("  PID: " + strconv.Itoa(int(cka.ProcessID)) + "\n")
	output.WriteString("  Status: " + strconv.Itoa(int(cka.status)) + " Disposition: " + strconv.Itoa(int(cka.disposition)) + "\n")
	output.WriteString("  baseObject: " + strconv.FormatUint(uint64(cka.baseObject), 16) + "\n")
	output.WriteString("  keyObject: " + strconv.FormatUint(uint64(cka.keyObject), 16) + "\n")
	output.WriteString("  basename: " + cka.baseName + "\n")
	output.WriteString("  relativename: " + cka.relativeName + "\n")
	output.WriteString("  computedfullpath: " + cka.computedFullPath + "\n")
	return output.String()
}

func parseDeleteRegistryKey(e *etw.DDEventRecord) (*deleteKeyArgs, error) {

	dka := &deleteKeyArgs{
		DDEventHeader: e.EventHeader,
	}

	data := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))

	dka.keyObject = regObjectPointer(binary.LittleEndian.Uint64(data[0:8]))
	dka.status = binary.LittleEndian.Uint32(data[8:12])
	dka.keyName, _, _, _ = parseUnicodeString(data, 12)

	return dka, nil
}

func (dka *deleteKeyArgs) string() string {
	var output strings.Builder

	output.WriteString("  PID: " + strconv.Itoa(int(dka.ProcessID)) + "\n")
	output.WriteString("  Status: " + strconv.Itoa(int(dka.status)) + "\n")
	output.WriteString("  keyName: " + dka.keyName + "\n")
	if s, ok := regPathResolver[dka.keyObject]; ok {
		output.WriteString("  resolved path: " + s + "\n")
	}

	//output.WriteString("  CapturedSize: " + strconv.Itoa(int(sv.capturedPreviousDataSize)) + " pvssize: " + strconv.Itoa(int(sv.previousDataSize)) + " capturedpvssize " + strconv.Itoa(int(sv.capturedPreviousDataSize)) + "\n")
	return output.String()

}

func parseSetValueKey(e *etw.DDEventRecord) (*setValueKeyArgs, error) {

	sv := &setValueKeyArgs{
		DDEventHeader: e.EventHeader,
	}

	ds := unsafe.Slice((*byte)(e.UserData), uint64(e.UserDataLength))
	data := ds

	/*
		for i := 0; i < int(e.UserDataLength); i++ {
			fmt.Printf(" %2x", data[i])
			if (i+1)%16 == 0 {
				fmt.Printf("\n")
			}
		}
		fmt.Printf("\n")
	*/
	sv.keyObject = regObjectPointer(binary.LittleEndian.Uint64(data[0:8]))
	sv.status = binary.LittleEndian.Uint32(data[8:12])
	sv.dataType = binary.LittleEndian.Uint32(data[12:16])
	sv.dataSize = binary.LittleEndian.Uint32(data[16:20])
	var nextOffset int
	var thisNextOffset int
	sv.keyName, nextOffset, _, _ = parseUnicodeString(data, 20)
	if nextOffset == -1 {
		nextOffset = 22
	}
	sv.valueName, thisNextOffset, _, _ = parseUnicodeString(data, nextOffset)
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

	if s, ok := regPathResolver[sv.keyObject]; ok {
		sv.computedFullPath = s
	}

	return sv, nil
}

func (sv *setValueKeyArgs) string() string {
	var output strings.Builder

	output.WriteString("  PID: " + strconv.Itoa(int(sv.ProcessID)) + "\n")
	output.WriteString("  Status: " + strconv.Itoa(int(sv.status)) + " dataType: " + strconv.Itoa(int(sv.dataType)) + " dataSize " + strconv.Itoa(int(sv.dataSize)) + "\n")
	output.WriteString("  keyObject: " + strconv.FormatUint(uint64(sv.keyObject), 16) + "\n")
	output.WriteString("  keyName: " + sv.keyName + "\n")
	output.WriteString("  valueName: " + sv.valueName + "\n")
	if s, ok := regPathResolver[sv.keyObject]; ok {
		output.WriteString("  resolved path: " + s + "\n")
	}

	//output.WriteString("  CapturedSize: " + strconv.Itoa(int(sv.capturedPreviousDataSize)) + " pvssize: " + strconv.Itoa(int(sv.previousDataSize)) + " capturedpvssize " + strconv.Itoa(int(sv.capturedPreviousDataSize)) + "\n")
	return output.String()

}
