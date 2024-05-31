// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
	regprefix = `\REGISTRY`
)

func (wp *WindowsProbe) parseCreateRegistryKey(e *etw.DDEventRecord) string {

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

	data := etwimpl.GetUserData(e)

	baseObject := regObjectPointer(data.GetUint64(0))
	keyObject := regObjectPointer(data.GetUint64(8))
	var nextOffset int
	var baseName string
	baseName, nextOffset, _, _ = data.ParseUnicodeString(24)
	if nextOffset == -1 {
		nextOffset = 26
	}
	relativeName, _, _, _ := data.ParseUnicodeString(nextOffset)

	return wp.computeFullPath(baseObject, keyObject, baseName, relativeName)
}

func (wp *WindowsProbe) onRegCreateKey(e *etw.DDEventRecord) *model.Event {
	fullPath := wp.parseCreateRegistryKey(e)

	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.CreateRegistryKeyEventType)
	ev.CreateRegistryKey = model.CreateRegistryKeyEvent{
		Registry: model.RegistryEvent{
			KeyPath: fullPath,
			KeyName: filepath.Base(fullPath),
		},
	}
	return ev
}

func (wp *WindowsProbe) onRegOpenKey(e *etw.DDEventRecord) *model.Event {
	fullPath := wp.parseCreateRegistryKey(e)

	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.OpenRegistryKeyEventType)
	ev.OpenRegistryKey = model.OpenRegistryKeyEvent{
		Registry: model.RegistryEvent{
			KeyPath: fullPath,
			KeyName: filepath.Base(fullPath),
		},
	}
	return ev
}
func translateBasePaths(relname string) string {

	table := map[string]string{
		"\\\\REGISTRY\\MACHINE": "HKEY_LOCAL_MACHINE",
		"\\REGISTRY\\MACHINE":   "HKEY_LOCAL_MACHINE",
		"\\\\REGISTRY\\USER":    "HKEY_USERS",
		"\\REGISTRY\\USER":      "HKEY_USERS",
	}

	for k, v := range table {
		if strings.HasPrefix(strings.ToUpper(relname), k) {
			return v + relname[len(k):]
		}
	}
	return relname
}

func (wp *WindowsProbe) computeFullPath(baseObject, keyObject regObjectPointer, baseName, relativeName string) string {
	var computedFullPath string
	if strings.HasPrefix(relativeName, regprefix) {
		relativeName = translateBasePaths(relativeName)
		computedFullPath = relativeName
		if wp.regPathResolver.Add(keyObject, relativeName) {
			wp.stats.registryCacheEvictions++
		}
		return computedFullPath
	}
	var outstr string
	if baseObject == 0 {
		if len(baseName) > 0 {
			outstr = baseName + "\\"
		}
		outstr += relativeName
	} else {

		if s, ok := wp.regPathResolver.Get(baseObject); ok {
			outstr = s + "\\" + relativeName
		} else {
			outstr = relativeName
		}
	}
	if wp.regPathResolver.Add(keyObject, outstr) {
		wp.stats.registryCacheEvictions++
	}
	// leaving this here for now; original algorithm doesn't quite make sense yet.
	computedFullPath = outstr
	return computedFullPath
}

func (wp *WindowsProbe) parseDeleteKeyArgs(e *etw.DDEventRecord) (keyObject regObjectPointer, fullPath string) {

	/*
			<template tid="task_0DeleteKeyArgs">
		      <data name="KeyObject" inType="win:Pointer"/>
		      <data name="Status" inType="win:UInt32"/>
		      <data name="KeyName" inType="win:UnicodeString"/>
		     </template>
	*/
	data := etwimpl.GetUserData(e)

	keyObject = regObjectPointer(data.GetUint64(0))
	//dka.status = data.GetUint32(8)
	//keyName, _, _, _ = data.ParseUnicodeString(12)
	if s, ok := wp.regPathResolver.Get(keyObject); ok {
		fullPath = s
	}
	return
}

func (wp *WindowsProbe) onRegDeleteKey(e *etw.DDEventRecord) *model.Event {
	_, fullPath := wp.parseDeleteKeyArgs(e)

	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.DeleteRegistryKeyEventType)
	ev.DeleteRegistryKey = model.DeleteRegistryKeyEvent{
		Registry: model.RegistryEvent{
			KeyPath: fullPath,
			KeyName: filepath.Base(fullPath),
		},
	}
	return ev
}

func (wp *WindowsProbe) onRegCloseKey(e *etw.DDEventRecord) {
	keyObject, _ := wp.parseDeleteKeyArgs(e)
	wp.regPathResolver.Remove(keyObject)
}

func (wp *WindowsProbe) parseSetValueKey(e *etw.DDEventRecord) (fullPath, valueName string) {
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

	data := etwimpl.GetUserData(e)

	keyObject := regObjectPointer(data.GetUint64(0))
	var nextOffset int

	_, nextOffset, _, _ = data.ParseUnicodeString(20)
	if nextOffset == -1 {
		nextOffset = 22
	}
	valueName, _, _, _ = data.ParseUnicodeString(nextOffset)

	if s, ok := wp.regPathResolver.Get(keyObject); ok {
		fullPath = s
	}

	return
}

func (wp *WindowsProbe) onRegSetValueKey(e *etw.DDEventRecord) *model.Event {
	fullPath, valueName := wp.parseSetValueKey(e)

	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.SetRegistryKeyValueEventType)
	ev.SetRegistryKeyValue = model.SetRegistryKeyValueEvent{
		Registry: model.RegistryEvent{
			KeyName: filepath.Base(fullPath),
			KeyPath: fullPath,
		},
		ValueName: valueName,
	}
	return ev
}
