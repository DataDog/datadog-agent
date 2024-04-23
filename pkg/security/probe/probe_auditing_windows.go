// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"

	"golang.org/x/sys/windows"
)

// the auditing manifest isn't nearly as complete as some of the others
// link https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-17134/Microsoft-Windows-Security-Auditing.xml

// this site does an OK job of documenting the event logs, which are just translations of the ETW events
// https://www.ultimatewindowssecurity.com/securitylog/encyclopedia/

const (
	// unfortunately, in the manifest, the event ids don't have useful names the way they do for file/registry.
	// so we'll make them up.
	idObjectPermsChange = uint16(4670) // the ever helpful task_04670
)

/*
<template tid="task_04670Args">
<data name="SubjectUserSid" inType="win:SID"/>
<data name="SubjectUserName" inType="win:UnicodeString"/>
<data name="SubjectDomainName" inType="win:UnicodeString"/>
<data name="SubjectLogonId" inType="win:HexInt64"/>
<data name="ObjectServer" inType="win:UnicodeString"/>
<data name="ObjectType" inType="win:UnicodeString"/>
<data name="ObjectName" inType="win:UnicodeString"/>
<data name="HandleId" inType="win:Pointer"/>
<data name="OldSd" inType="win:UnicodeString"/>
<data name="NewSd" inType="win:UnicodeString"/>
<data name="ProcessId" inType="win:Pointer"/>
<data name="ProcessName" inType="win:UnicodeString"/>
</template>
*/

// we're going to try for a slightly more useful name
//
//revive:disable:var-naming
type objectPermsChange struct {
	etw.DDEventHeader
	subjectUserSid    string
	subjectUserName   string
	subjectDomainName string
	subjectLogonId    string
	objectServer      string
	objectType        string
	objectName        string
	handleId          fileObjectPointer
	oldSd             string
	newSd             string
	processId         fileObjectPointer
	processName       string
}

func (wp *WindowsProbe) parseObjectPermsChange(e *etw.DDEventRecord) (*objectPermsChange, error) {

	pc := &objectPermsChange{
		DDEventHeader: e.EventHeader,
	}
	data := etwimpl.GetUserData(e)

	reader := stringparser{nextRead: 0}
	pc.subjectUserSid = reader.GetSIDString(data)
	pc.subjectUserName = reader.GetNextString(data)
	pc.subjectDomainName = reader.GetNextString(data)
	pc.subjectLogonId = strconv.FormatUint(reader.GetUint64(data), 16)
	pc.objectServer = reader.GetNextString(data)
	pc.objectType = reader.GetNextString(data)
	pc.objectName = reader.GetNextString(data)

	pc.handleId = fileObjectPointer(reader.GetUint64(data))

	pc.oldSd = reader.GetNextString(data)
	pc.newSd = reader.GetNextString(data)

	pc.processId = fileObjectPointer(reader.GetUint64(data))

	pc.processName = reader.GetNextString(data)

	return pc, nil
}

func (pc *objectPermsChange) String() string {
	var output strings.Builder
	output.WriteString(" ObjectPermsChange name: " + pc.objectName + "\n")
	output.WriteString("                   oldsd: " + pc.oldSd + "\n")
	output.WriteString("                   newsd: " + pc.newSd + "\n")

	return output.String()
}

type stringparser struct {
	nextRead int
}

func (sp *stringparser) GetNextString(data etw.UserData) string {
	s, no, _, _ := data.ParseUnicodeString(sp.nextRead)

	if no == -1 {
		sp.nextRead += 2
	} else {
		sp.nextRead = no
	}
	return s
}

func (sp *stringparser) GetSIDString(data etw.UserData) string {
	l := data.Length()
	b := data.Bytes(sp.nextRead, l-sp.nextRead)
	sid := (*windows.SID)(unsafe.Pointer(&b[0]))
	sidlen := windows.GetLengthSid(sid)
	sp.nextRead += int(sidlen)

	var winstring *uint16
	err := windows.ConvertSidToStringSid(sid, &winstring)
	if err != nil {
		return ""
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(winstring)))

	return windows.UTF16PtrToString(winstring)

}

func (sp *stringparser) GetUint64(data etw.UserData) uint64 {
	n := data.GetUint64(sp.nextRead)
	sp.nextRead += 8
	return n
}
func (sp *stringparser) SetNextReadOffset(offset int) {
	sp.nextRead = offset
}

func (sp *stringparser) GetNextReadOffset() int {
	return sp.nextRead
}
