// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
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
	regprefix       = `\REGISTRY`
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
type openKeyArgs createKeyArgs

/*
		<template tid="task_0DeleteKeyArgs">
	      <data name="KeyObject" inType="win:Pointer"/>
	      <data name="Status" inType="win:UInt32"/>
	      <data name="KeyName" inType="win:UnicodeString"/>
	     </template>
*/
type deleteKeyArgs struct {
	etw.DDEventHeader
	keyObject        regObjectPointer
	status           uint32
	keyName          string
	computedFullPath string
}
type flushKeyArgs deleteKeyArgs
type closeKeyArgs deleteKeyArgs
type querySecurityKeyArgs deleteKeyArgs
type setSecurityKeyArgs deleteKeyArgs

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
	data := etwimpl.GetUserData(e)

	crc.baseObject = regObjectPointer(data.GetUint64(0))
	crc.keyObject = regObjectPointer(data.GetUint64(8))
	crc.status = data.GetUint32(16)
	crc.disposition = data.GetUint32(20)

	//var nextOffset int
	//var nulltermidx int
	var nextOffset int
	crc.baseName, nextOffset, _, _ = data.ParseUnicodeString(24)
	if nextOffset == -1 {
		nextOffset = 26
	}
	crc.relativeName, _, _, _ = data.ParseUnicodeString(nextOffset)

	crc.computeFullPath()
	return crc, nil
}

func (cka *createKeyArgs) translateBasePaths() {

	table := map[string]string{
		"\\\\REGISTRY\\MACHINE": "HKEY_LOCAL_MACHINE",
		"\\REGISTRY\\MACHINE":   "HKEY_LOCAL_MACHINE",
		"\\\\REGISTRY\\USER":    "HKEY_USERS",
		"\\REGISTRY\\USER":      "HKEY_USERS",
	}

	for k, v := range table {
		if strings.HasPrefix(strings.ToUpper(cka.relativeName), k) {
			cka.relativeName = v + cka.relativeName[len(k):]
		}
	}
}
func parseOpenRegistryKey(e *etw.DDEventRecord) (*openKeyArgs, error) {
	cka, err := parseCreateRegistryKey(e)
	if err != nil {
		return nil, err
	}
	return (*openKeyArgs)(cka), nil
}

func (cka *createKeyArgs) computeFullPath() {

	// var regPathResolver map[regObjectPointer]string

	if strings.HasPrefix(cka.relativeName, regprefix) {
		cka.translateBasePaths()
		cka.computedFullPath = cka.relativeName
		regPathResolver[cka.keyObject] = cka.relativeName
		return
	}
	if s, ok := regPathResolver[cka.keyObject]; ok {
		cka.computedFullPath = s
	}
	var outstr string
	if cka.baseObject == 0 {
		if len(cka.baseName) > 0 {
			outstr = cka.baseName + "\\"
		}
		outstr += cka.relativeName
	} else {

		if s, ok := regPathResolver[cka.baseObject]; ok {
			outstr = s + "\\" + cka.relativeName
		} else {
			outstr = cka.relativeName
		}
	}
	regPathResolver[cka.keyObject] = outstr
	cka.computedFullPath = outstr
}
func (cka *createKeyArgs) String() string {

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

func (cka *openKeyArgs) String() string {
	return (*createKeyArgs)(cka).String()
}

func parseDeleteRegistryKey(e *etw.DDEventRecord) (*deleteKeyArgs, error) {

	dka := &deleteKeyArgs{
		DDEventHeader: e.EventHeader,
	}

	data := etwimpl.GetUserData(e)

	dka.keyObject = regObjectPointer(data.GetUint64(0))
	dka.status = data.GetUint32(8)
	dka.keyName, _, _, _ = data.ParseUnicodeString(12)
	if s, ok := regPathResolver[dka.keyObject]; ok {
		dka.computedFullPath = s
	}

	return dka, nil
}

func parseFlushKey(e *etw.DDEventRecord) (*flushKeyArgs, error) {
	dka, err := parseDeleteRegistryKey(e)
	if err != nil {
		return nil, err
	}
	return (*flushKeyArgs)(dka), nil
}

func parseCloseKeyArgs(e *etw.DDEventRecord) (*closeKeyArgs, error) {
	dka, err := parseDeleteRegistryKey(e)
	if err != nil {
		return nil, err
	}
	return (*closeKeyArgs)(dka), nil
}
func parseQuerySecurityKeyArgs(e *etw.DDEventRecord) (*querySecurityKeyArgs, error) {
	dka, err := parseDeleteRegistryKey(e)
	if err != nil {
		return nil, err
	}
	return (*querySecurityKeyArgs)(dka), nil
}
func parseSetSecurityKeyArgs(e *etw.DDEventRecord) (*setSecurityKeyArgs, error) {
	dka, err := parseDeleteRegistryKey(e)
	if err != nil {
		return nil, err
	}
	return (*setSecurityKeyArgs)(dka), nil
}

func (dka *deleteKeyArgs) String() string {
	var output strings.Builder

	output.WriteString("  PID: " + strconv.Itoa(int(dka.ProcessID)) + "\n")
	output.WriteString("  Status: " + strconv.Itoa(int(dka.status)) + "\n")
	output.WriteString("  keyName: " + dka.keyName + "\n")
	output.WriteString("  resolved path: " + dka.computedFullPath + "\n")

	//output.WriteString("  CapturedSize: " + strconv.Itoa(int(sv.capturedPreviousDataSize)) + " pvssize: " + strconv.Itoa(int(sv.previousDataSize)) + " capturedpvssize " + strconv.Itoa(int(sv.capturedPreviousDataSize)) + "\n")
	return output.String()

}

func (fka *flushKeyArgs) String() string {
	return (*deleteKeyArgs)(fka).String()
}
func (cka *closeKeyArgs) String() string {
	return (*deleteKeyArgs)(cka).String()
}

//nolint:unused
func (qka *querySecurityKeyArgs) String() string {
	return (*deleteKeyArgs)(qka).String()
}

//nolint:unused
func (ska *setSecurityKeyArgs) String() string {
	return (*deleteKeyArgs)(ska).String()
}

func parseSetValueKey(e *etw.DDEventRecord) (*setValueKeyArgs, error) {

	sv := &setValueKeyArgs{
		DDEventHeader: e.EventHeader,
	}

	data := etwimpl.GetUserData(e)

	/*
		for i := 0; i < int(e.UserDataLength); i++ {
			fmt.Printf(" %2x", data[i])
			if (i+1)%16 == 0 {
				fmt.Printf("\n")
			}
		}
		fmt.Printf("\n")
	*/
	sv.keyObject = regObjectPointer(data.GetUint64(0))
	sv.status = data.GetUint32(8)
	sv.dataType = data.GetUint32(12)
	sv.dataSize = data.GetUint32(16)
	var nextOffset int
	var thisNextOffset int
	sv.keyName, nextOffset, _, _ = data.ParseUnicodeString(20)
	if nextOffset == -1 {
		nextOffset = 22
	}
	sv.valueName, thisNextOffset, _, _ = data.ParseUnicodeString(nextOffset)
	if thisNextOffset == -1 {
		nextOffset += 2
	} else {
		nextOffset = thisNextOffset
	}

	sv.capturedDataSize = data.GetUint16(nextOffset)
	nextOffset += 2

	// make a copy of the data because the underlying buffer here belongs to etw
	sv.capturedData = data.Bytes(nextOffset, int(sv.capturedDataSize))
	nextOffset += int(sv.capturedDataSize)

	sv.previousDataType = data.GetUint32(nextOffset)
	nextOffset += 4

	sv.previousDataSize = data.GetUint32(nextOffset)
	nextOffset += 4

	sv.previousData = data.Bytes(nextOffset, int(sv.previousDataSize))

	if s, ok := regPathResolver[sv.keyObject]; ok {
		sv.computedFullPath = s
	}

	return sv, nil
}

func (sv *setValueKeyArgs) String() string {
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
