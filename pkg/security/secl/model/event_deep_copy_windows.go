// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"unsafe"
)

// deepCopyType identifies the type of a pointer, slice or map. It is part of deepCopyKey because a struct
// and its first field share the same address.
type deepCopyType int

// deepCopyKey identifies a pointer, slice or map of the original.
type deepCopyKey struct {
	ptr unsafe.Pointer
	len int
	typ deepCopyType
}

// deepCopyContext maps the pointers, slices and maps of the original to the memory of their copy, so that
// the ones sharing memory in the original share it in the copy too.
type deepCopyContext map[deepCopyKey]unsafe.Pointer

func pointerKey[T any](p *T, typ deepCopyType) deepCopyKey {
	return deepCopyKey{unsafe.Pointer(p), 0, typ}
}
func sliceKey[T any](s []T, typ deepCopyType) deepCopyKey {
	return deepCopyKey{unsafe.Pointer(unsafe.SliceData(s)), len(s), typ}
}
func mapKey[V any](m map[string]V, typ deepCopyType) deepCopyKey {
	return deepCopyKey{mapPointer(m), 0, typ}
}
func mapPointer[V any](m map[string]V) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&m))
}
func mapFromPointer[V any](p unsafe.Pointer) map[string]V {
	return *(*map[string]V)(unsafe.Pointer(&p))
}

// DeepCopy creates a deep copy of the Event where the copy shares nothing with the original
func (e *Event) DeepCopy() *Event {
	if e == nil {
		return nil
	}
	copied := &Event{}
	c := make(deepCopyContext, 32)
	copied.BaseEvent = deepCopyBaseEvent(e.BaseEvent, c)
	copied.ChangePermission = deepCopyChangePermissionEvent(e.ChangePermission, c)
	copied.CreateNewFile = deepCopyCreateNewFileEvent(e.CreateNewFile, c)
	copied.CreateRegistryKey = deepCopyCreateRegistryKeyEvent(e.CreateRegistryKey, c)
	copied.DeleteFile = deepCopyDeleteFileEvent(e.DeleteFile, c)
	copied.DeleteRegistryKey = deepCopyDeleteRegistryKeyEvent(e.DeleteRegistryKey, c)
	copied.Exec = deepCopyExecEvent(e.Exec, c)
	copied.Exit = deepCopyExitEvent(e.Exit, c)
	copied.OpenRegistryKey = deepCopyOpenRegistryKeyEvent(e.OpenRegistryKey, c)
	copied.RenameFile = deepCopyRenameFileEvent(e.RenameFile, c)
	copied.SetRegistryKeyValue = deepCopySetRegistryKeyValueEvent(e.SetRegistryKeyValue, c)
	copied.WriteFile = deepCopyWriteFileEvent(e.WriteFile, c)
	// FieldHandlers is an interface that must be copied by reference (not deep copied)
	// It provides access to shared resolvers needed for field resolution
	copied.FieldHandlers = e.FieldHandlers
	return copied
}
func deepCopyBaseEvent(fieldToCopy BaseEvent, c deepCopyContext) BaseEvent {
	copied := BaseEvent{}
	copied.Flags = fieldToCopy.Flags
	copied.Hostname = fieldToCopy.Hostname
	copied.ID = fieldToCopy.ID
	copied.Origin = fieldToCopy.Origin
	copied.Os = fieldToCopy.Os
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext, c)
	copied.ProcessCacheEntry = deepCopyProcessCacheEntryPtr(fieldToCopy.ProcessCacheEntry, c)
	copied.ProcessContext = deepCopyProcessContextPtr(fieldToCopy.ProcessContext, c)
	copied.RuleContext = deepCopyRuleContext(fieldToCopy.RuleContext, c)
	copied.RuleTags = deepCopystringArr(fieldToCopy.RuleTags, c)
	copied.Rules = deepCopyMatchedRulePtrArr(fieldToCopy.Rules, c)
	copied.SecurityProfileContext = deepCopySecurityProfileContext(fieldToCopy.SecurityProfileContext, c)
	copied.Service = fieldToCopy.Service
	copied.Source = fieldToCopy.Source
	copied.Timestamp = fieldToCopy.Timestamp
	copied.TimestampRaw = fieldToCopy.TimestampRaw
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyPIDContext(fieldToCopy PIDContext, c deepCopyContext) PIDContext {
	copied := PIDContext{}
	copied.Pid = fieldToCopy.Pid
	return copied
}
func deepCopyProcessCacheEntryPtr(fieldToCopy *ProcessCacheEntry, c deepCopyContext) *ProcessCacheEntry {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyProcessCacheEntryPtrType)
	if copied, ok := c[key]; ok {
		return (*ProcessCacheEntry)(copied)
	}
	copied := &ProcessCacheEntry{}
	c[key] = unsafe.Pointer(copied)
	c[pointerKey(&fieldToCopy.ProcessContext, deepCopyProcessContextPtrType)] = unsafe.Pointer(&copied.ProcessContext)
	c[pointerKey(&fieldToCopy.ProcessContext.Process, deepCopyProcessPtrType)] = unsafe.Pointer(&copied.ProcessContext.Process)
	copied.ProcessContext = deepCopyProcessContext(fieldToCopy.ProcessContext, c)
	return copied
}
func deepCopyProcessContext(fieldToCopy ProcessContext, c deepCopyContext) ProcessContext {
	copied := ProcessContext{}
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor, c)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent, c)
	copied.Process = deepCopyProcess(fieldToCopy.Process, c)
	return copied
}
func deepCopyProcessPtr(fieldToCopy *Process, c deepCopyContext) *Process {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyProcessPtrType)
	if copied, ok := c[key]; ok {
		return (*Process)(copied)
	}
	copied := &Process{}
	c[key] = unsafe.Pointer(copied)
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry, c)
	copied.CmdLine = fieldToCopy.CmdLine
	copied.CmdLineScrubbed = fieldToCopy.CmdLineScrubbed
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext, c)
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Envp = deepCopystringArr(fieldToCopy.Envp, c)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs, c)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry, c)
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent, c)
	copied.OwnerSidString = fieldToCopy.OwnerSidString
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext, c)
	copied.PPid = fieldToCopy.PPid
	copied.ScrubbedCmdLineResolved = fieldToCopy.ScrubbedCmdLineResolved
	copied.Tracer = deepCopyTracer(fieldToCopy.Tracer, c)
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyArgsEntryPtr(fieldToCopy *ArgsEntry, c deepCopyContext) *ArgsEntry {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyArgsEntryPtrType)
	if copied, ok := c[key]; ok {
		return (*ArgsEntry)(copied)
	}
	copied := &ArgsEntry{}
	c[key] = unsafe.Pointer(copied)
	copied.ScrubbedResolved = fieldToCopy.ScrubbedResolved
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values, c)
	return copied
}
func deepCopystringArr(fieldToCopy []string, c deepCopyContext) []string {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopystringArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*string)(copied), len(fieldToCopy))
	}
	copied := make([]string, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyContainerContext(fieldToCopy ContainerContext, c deepCopyContext) ContainerContext {
	copied := ContainerContext{}
	copied.ContainerID = fieldToCopy.ContainerID
	copied.ContainerSource = fieldToCopy.ContainerSource
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.PodUID = fieldToCopy.PodUID
	copied.Releasable = deepCopyReleasablePtr(fieldToCopy.Releasable, c)
	copied.Tags = deepCopystringArr(fieldToCopy.Tags, c)
	return copied
}
func deepCopyReleasablePtr(fieldToCopy *Releasable, c deepCopyContext) *Releasable {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyReleasablePtrType)
	if copied, ok := c[key]; ok {
		return (*Releasable)(copied)
	}
	copied := &Releasable{}
	c[key] = unsafe.Pointer(copied)
	return copied
}
func deepCopyEnvsEntryPtr(fieldToCopy *EnvsEntry, c deepCopyContext) *EnvsEntry {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyEnvsEntryPtrType)
	if copied, ok := c[key]; ok {
		return (*EnvsEntry)(copied)
	}
	copied := &EnvsEntry{}
	c[key] = unsafe.Pointer(copied)
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values, c)
	return copied
}
func deepCopyFileEvent(fieldToCopy FileEvent, c deepCopyContext) FileEvent {
	copied := FileEvent{}
	copied.BasenameStr = fieldToCopy.BasenameStr
	copied.Extension = fieldToCopy.Extension
	copied.FileObject = fieldToCopy.FileObject
	copied.PathnameStr = fieldToCopy.PathnameStr
	return copied
}
func deepCopyTracer(fieldToCopy Tracer, c deepCopyContext) Tracer {
	copied := Tracer{}
	copied.Metadata = deepCopyTracerMetadata(fieldToCopy.Metadata, c)
	copied.MetadataSource = fieldToCopy.MetadataSource
	copied.ThreadlocalAttributeKeys = deepCopystringArr(fieldToCopy.ThreadlocalAttributeKeys, c)
	copied.Trace = deepCopySpanContext(fieldToCopy.Trace, c)
	return copied
}
func deepCopyTracerMetadata(fieldToCopy tracermetadata.TracerMetadata, c deepCopyContext) tracermetadata.TracerMetadata {
	copied := tracermetadata.TracerMetadata{}
	copied.ContainerID = fieldToCopy.ContainerID
	copied.Hostname = fieldToCopy.Hostname
	copied.LogsCollected = fieldToCopy.LogsCollected
	copied.ProcessTags = fieldToCopy.ProcessTags
	copied.RuntimeID = fieldToCopy.RuntimeID
	copied.SchemaVersion = fieldToCopy.SchemaVersion
	copied.ServiceEnv = fieldToCopy.ServiceEnv
	copied.ServiceName = fieldToCopy.ServiceName
	copied.ServiceVersion = fieldToCopy.ServiceVersion
	copied.TracerLanguage = fieldToCopy.TracerLanguage
	copied.TracerVersion = fieldToCopy.TracerVersion
	return copied
}
func deepCopySpanContext(fieldToCopy SpanContext, c deepCopyContext) SpanContext {
	copied := SpanContext{}
	copied.Attributes = deepCopystringMap(fieldToCopy.Attributes, c)
	copied.ExtraAttrsID = fieldToCopy.ExtraAttrsID
	copied.HasExtraAttrs = fieldToCopy.HasExtraAttrs
	copied.SpanID = fieldToCopy.SpanID
	copied.TraceID = deepCopyTraceID(fieldToCopy.TraceID, c)
	return copied
}
func deepCopystringMap(fieldToCopy map[string]string, c deepCopyContext) map[string]string {
	if fieldToCopy == nil {
		return nil
	}
	key := mapKey(fieldToCopy, deepCopystringMapType)
	if copied, ok := c[key]; ok {
		return mapFromPointer[string](copied)
	}
	copied := make(map[string]string, len(fieldToCopy))
	c[key] = mapPointer(copied)
	for k, v := range fieldToCopy {
		copied[k] = v
	}
	return copied
}
func deepCopyTraceID(fieldToCopy utils.TraceID, c deepCopyContext) utils.TraceID {
	return fieldToCopy
}
func deepCopyProcess(fieldToCopy Process, c deepCopyContext) Process {
	copied := Process{}
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry, c)
	copied.CmdLine = fieldToCopy.CmdLine
	copied.CmdLineScrubbed = fieldToCopy.CmdLineScrubbed
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext, c)
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Envp = deepCopystringArr(fieldToCopy.Envp, c)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs, c)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry, c)
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent, c)
	copied.OwnerSidString = fieldToCopy.OwnerSidString
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext, c)
	copied.PPid = fieldToCopy.PPid
	copied.ScrubbedCmdLineResolved = fieldToCopy.ScrubbedCmdLineResolved
	copied.Tracer = deepCopyTracer(fieldToCopy.Tracer, c)
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyProcessContextPtr(fieldToCopy *ProcessContext, c deepCopyContext) *ProcessContext {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyProcessContextPtrType)
	if copied, ok := c[key]; ok {
		return (*ProcessContext)(copied)
	}
	copied := &ProcessContext{}
	c[key] = unsafe.Pointer(copied)
	c[pointerKey(&fieldToCopy.Process, deepCopyProcessPtrType)] = unsafe.Pointer(&copied.Process)
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor, c)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent, c)
	copied.Process = deepCopyProcess(fieldToCopy.Process, c)
	return copied
}
func deepCopyRuleContext(fieldToCopy RuleContext, c deepCopyContext) RuleContext {
	copied := RuleContext{}
	copied.Expression = fieldToCopy.Expression
	copied.MatchingSubExprs = deepCopyMatchingSubExprArr(fieldToCopy.MatchingSubExprs, c)
	return copied
}
func deepCopyMatchingSubExprArr(fieldToCopy []eval.MatchingSubExpr, c deepCopyContext) []eval.MatchingSubExpr {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyMatchingSubExprArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*eval.MatchingSubExpr)(copied), len(fieldToCopy))
	}
	copied := make([]eval.MatchingSubExpr, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchingSubExpr(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyMatchingValue(fieldToCopy eval.MatchingValue, c deepCopyContext) eval.MatchingValue {
	copied := eval.MatchingValue{}
	copied.Field = fieldToCopy.Field
	copied.Offset = fieldToCopy.Offset
	return copied
}
func deepCopyMatchingSubExpr(fieldToCopy eval.MatchingSubExpr, c deepCopyContext) eval.MatchingSubExpr {
	copied := eval.MatchingSubExpr{}
	copied.Offset = fieldToCopy.Offset
	copied.ValueA = deepCopyMatchingValue(fieldToCopy.ValueA, c)
	copied.ValueB = deepCopyMatchingValue(fieldToCopy.ValueB, c)
	return copied
}
func deepCopyMatchedRulePtrArr(fieldToCopy []*MatchedRule, c deepCopyContext) []*MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyMatchedRulePtrArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((**MatchedRule)(copied), len(fieldToCopy))
	}
	copied := make([]*MatchedRule, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchedRulePtr(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyMatchedRulePtr(fieldToCopy *MatchedRule, c deepCopyContext) *MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyMatchedRulePtrType)
	if copied, ok := c[key]; ok {
		return (*MatchedRule)(copied)
	}
	copied := &MatchedRule{}
	c[key] = unsafe.Pointer(copied)
	copied.PolicyName = fieldToCopy.PolicyName
	copied.PolicyVersion = fieldToCopy.PolicyVersion
	copied.RuleID = fieldToCopy.RuleID
	copied.RuleTags = deepCopystringMap(fieldToCopy.RuleTags, c)
	copied.RuleVersion = fieldToCopy.RuleVersion
	return copied
}
func deepCopySecurityProfileContext(fieldToCopy SecurityProfileContext, c deepCopyContext) SecurityProfileContext {
	copied := SecurityProfileContext{}
	copied.EventTypeState = fieldToCopy.EventTypeState
	copied.EventTypes = deepCopyEventTypeArr(fieldToCopy.EventTypes, c)
	copied.Name = fieldToCopy.Name
	copied.ProfileAlreadySent = fieldToCopy.ProfileAlreadySent
	copied.Tags = deepCopystringArr(fieldToCopy.Tags, c)
	copied.Version = fieldToCopy.Version
	return copied
}
func deepCopyEventTypeArr(fieldToCopy []EventType, c deepCopyContext) []EventType {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyEventTypeArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*EventType)(copied), len(fieldToCopy))
	}
	copied := make([]EventType, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyChangePermissionEvent(fieldToCopy ChangePermissionEvent, c deepCopyContext) ChangePermissionEvent {
	copied := ChangePermissionEvent{}
	copied.NewSd = fieldToCopy.NewSd
	copied.ObjectName = fieldToCopy.ObjectName
	copied.ObjectType = fieldToCopy.ObjectType
	copied.OldSd = fieldToCopy.OldSd
	copied.UserDomain = fieldToCopy.UserDomain
	copied.UserName = fieldToCopy.UserName
	return copied
}
func deepCopyCreateNewFileEvent(fieldToCopy CreateNewFileEvent, c deepCopyContext) CreateNewFileEvent {
	copied := CreateNewFileEvent{}
	copied.File = deepCopyFimFileEvent(fieldToCopy.File, c)
	return copied
}
func deepCopyFimFileEvent(fieldToCopy FimFileEvent, c deepCopyContext) FimFileEvent {
	copied := FimFileEvent{}
	copied.BasenameStr = fieldToCopy.BasenameStr
	copied.Extension = fieldToCopy.Extension
	copied.FileObject = fieldToCopy.FileObject
	copied.PathnameStr = fieldToCopy.PathnameStr
	copied.UserPathnameStr = fieldToCopy.UserPathnameStr
	return copied
}
func deepCopyCreateRegistryKeyEvent(fieldToCopy CreateRegistryKeyEvent, c deepCopyContext) CreateRegistryKeyEvent {
	copied := CreateRegistryKeyEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry, c)
	return copied
}
func deepCopyRegistryEvent(fieldToCopy RegistryEvent, c deepCopyContext) RegistryEvent {
	copied := RegistryEvent{}
	copied.KeyName = fieldToCopy.KeyName
	copied.KeyPath = fieldToCopy.KeyPath
	return copied
}
func deepCopyDeleteFileEvent(fieldToCopy DeleteFileEvent, c deepCopyContext) DeleteFileEvent {
	copied := DeleteFileEvent{}
	copied.File = deepCopyFimFileEvent(fieldToCopy.File, c)
	return copied
}
func deepCopyDeleteRegistryKeyEvent(fieldToCopy DeleteRegistryKeyEvent, c deepCopyContext) DeleteRegistryKeyEvent {
	copied := DeleteRegistryKeyEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry, c)
	return copied
}
func deepCopyExecEvent(fieldToCopy ExecEvent, c deepCopyContext) ExecEvent {
	copied := ExecEvent{}
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process, c)
	return copied
}
func deepCopyExitEvent(fieldToCopy ExitEvent, c deepCopyContext) ExitEvent {
	copied := ExitEvent{}
	copied.Cause = fieldToCopy.Cause
	copied.Code = fieldToCopy.Code
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process, c)
	return copied
}
func deepCopyOpenRegistryKeyEvent(fieldToCopy OpenRegistryKeyEvent, c deepCopyContext) OpenRegistryKeyEvent {
	copied := OpenRegistryKeyEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry, c)
	return copied
}
func deepCopyRenameFileEvent(fieldToCopy RenameFileEvent, c deepCopyContext) RenameFileEvent {
	copied := RenameFileEvent{}
	copied.New = deepCopyFimFileEvent(fieldToCopy.New, c)
	copied.Old = deepCopyFimFileEvent(fieldToCopy.Old, c)
	return copied
}
func deepCopySetRegistryKeyValueEvent(fieldToCopy SetRegistryKeyValueEvent, c deepCopyContext) SetRegistryKeyValueEvent {
	copied := SetRegistryKeyValueEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry, c)
	copied.ValueName = fieldToCopy.ValueName
	return copied
}
func deepCopyWriteFileEvent(fieldToCopy WriteFileEvent, c deepCopyContext) WriteFileEvent {
	copied := WriteFileEvent{}
	copied.File = deepCopyFimFileEvent(fieldToCopy.File, c)
	return copied
}

const (
	_ deepCopyType = iota
	deepCopyArgsEntryPtrType
	deepCopyEnvsEntryPtrType
	deepCopyEventTypeArrType
	deepCopyMatchedRulePtrType
	deepCopyMatchedRulePtrArrType
	deepCopyMatchingSubExprArrType
	deepCopyProcessCacheEntryPtrType
	deepCopyProcessContextPtrType
	deepCopyProcessPtrType
	deepCopyReleasablePtrType
	deepCopystringArrType
	deepCopystringMapType
)
