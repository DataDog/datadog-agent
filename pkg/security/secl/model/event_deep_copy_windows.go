// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// DeepCopy creates a deep copy of the Event where the copy shares nothing with the original
func (e *Event) DeepCopy() *Event {
	if e == nil {
		return nil
	}
	copied := &Event{}
	copied.BaseEvent = deepCopyBaseEvent(e.BaseEvent)
	copied.ChangePermission = deepCopyChangePermissionEvent(e.ChangePermission)
	copied.CreateNewFile = deepCopyCreateNewFileEvent(e.CreateNewFile)
	copied.CreateRegistryKey = deepCopyCreateRegistryKeyEvent(e.CreateRegistryKey)
	copied.DeleteFile = deepCopyDeleteFileEvent(e.DeleteFile)
	copied.DeleteRegistryKey = deepCopyDeleteRegistryKeyEvent(e.DeleteRegistryKey)
	copied.Exec = deepCopyExecEvent(e.Exec)
	copied.Exit = deepCopyExitEvent(e.Exit)
	copied.OpenRegistryKey = deepCopyOpenRegistryKeyEvent(e.OpenRegistryKey)
	copied.RenameFile = deepCopyRenameFileEvent(e.RenameFile)
	copied.SetRegistryKeyValue = deepCopySetRegistryKeyValueEvent(e.SetRegistryKeyValue)
	copied.WriteFile = deepCopyWriteFileEvent(e.WriteFile)
	// FieldHandlers is an interface that must be copied by reference (not deep copied)
	// It provides access to shared resolvers needed for field resolution
	copied.FieldHandlers = e.FieldHandlers
	return copied
}
func deepCopyBaseEvent(fieldToCopy BaseEvent) BaseEvent {
	copied := BaseEvent{}
	copied.Flags = fieldToCopy.Flags
	copied.Hostname = fieldToCopy.Hostname
	copied.ID = fieldToCopy.ID
	copied.Origin = fieldToCopy.Origin
	copied.Os = fieldToCopy.Os
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext)
	copied.ProcessCacheEntry = deepCopyProcessCacheEntryPtr(fieldToCopy.ProcessCacheEntry)
	copied.ProcessContext = deepCopyProcessContextPtr(fieldToCopy.ProcessContext)
	copied.RuleContext = deepCopyRuleContext(fieldToCopy.RuleContext)
	copied.RuleTags = deepCopystringArr(fieldToCopy.RuleTags)
	copied.Rules = deepCopyMatchedRulePtrArr(fieldToCopy.Rules)
	copied.SecurityProfileContext = deepCopySecurityProfileContext(fieldToCopy.SecurityProfileContext)
	copied.Service = fieldToCopy.Service
	copied.Source = fieldToCopy.Source
	copied.Timestamp = fieldToCopy.Timestamp
	copied.TimestampRaw = fieldToCopy.TimestampRaw
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyPIDContext(fieldToCopy PIDContext) PIDContext {
	copied := PIDContext{}
	copied.Pid = fieldToCopy.Pid
	return copied
}
func deepCopyProcessCacheEntryPtr(fieldToCopy *ProcessCacheEntry) *ProcessCacheEntry {
	if fieldToCopy == nil {
		return nil
	}
	copied := &ProcessCacheEntry{}
	copied.ProcessContext = deepCopyProcessContext(fieldToCopy.ProcessContext)
	return copied
}
func deepCopyProcessContext(fieldToCopy ProcessContext) ProcessContext {
	copied := ProcessContext{}
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent)
	copied.Process = deepCopyProcess(fieldToCopy.Process)
	return copied
}
func deepCopyProcessPtr(fieldToCopy *Process) *Process {
	if fieldToCopy == nil {
		return nil
	}
	copied := &Process{}
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry)
	copied.CmdLine = fieldToCopy.CmdLine
	copied.CmdLineScrubbed = fieldToCopy.CmdLineScrubbed
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext)
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Envp = deepCopystringArr(fieldToCopy.Envp)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry)
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent)
	copied.OwnerSidString = fieldToCopy.OwnerSidString
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext)
	copied.PPid = fieldToCopy.PPid
	copied.ScrubbedCmdLineResolved = fieldToCopy.ScrubbedCmdLineResolved
	copied.TracerTags = deepCopystringArr(fieldToCopy.TracerTags)
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyArgsEntryPtr(fieldToCopy *ArgsEntry) *ArgsEntry {
	if fieldToCopy == nil {
		return nil
	}
	copied := &ArgsEntry{}
	copied.ScrubbedResolved = fieldToCopy.ScrubbedResolved
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values)
	return copied
}
func deepCopystringArr(fieldToCopy []string) []string {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]string, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyContainerContext(fieldToCopy ContainerContext) ContainerContext {
	copied := ContainerContext{}
	copied.ContainerID = fieldToCopy.ContainerID
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Releasable = deepCopyReleasablePtr(fieldToCopy.Releasable)
	copied.Tags = deepCopystringArr(fieldToCopy.Tags)
	return copied
}
func deepCopyReleasablePtr(fieldToCopy *Releasable) *Releasable {
	if fieldToCopy == nil {
		return nil
	}
	copied := &Releasable{}
	return copied
}
func deepCopyEnvsEntryPtr(fieldToCopy *EnvsEntry) *EnvsEntry {
	if fieldToCopy == nil {
		return nil
	}
	copied := &EnvsEntry{}
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values)
	return copied
}
func deepCopyFileEvent(fieldToCopy FileEvent) FileEvent {
	copied := FileEvent{}
	copied.BasenameStr = fieldToCopy.BasenameStr
	copied.Extension = fieldToCopy.Extension
	copied.FileObject = fieldToCopy.FileObject
	copied.PathnameStr = fieldToCopy.PathnameStr
	return copied
}
func deepCopyProcess(fieldToCopy Process) Process {
	copied := Process{}
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry)
	copied.CmdLine = fieldToCopy.CmdLine
	copied.CmdLineScrubbed = fieldToCopy.CmdLineScrubbed
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext)
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Envp = deepCopystringArr(fieldToCopy.Envp)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry)
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent)
	copied.OwnerSidString = fieldToCopy.OwnerSidString
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext)
	copied.PPid = fieldToCopy.PPid
	copied.ScrubbedCmdLineResolved = fieldToCopy.ScrubbedCmdLineResolved
	copied.TracerTags = deepCopystringArr(fieldToCopy.TracerTags)
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyProcessContextPtr(fieldToCopy *ProcessContext) *ProcessContext {
	if fieldToCopy == nil {
		return nil
	}
	copied := &ProcessContext{}
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent)
	copied.Process = deepCopyProcess(fieldToCopy.Process)
	return copied
}
func deepCopyRuleContext(fieldToCopy RuleContext) RuleContext {
	copied := RuleContext{}
	copied.Expression = fieldToCopy.Expression
	copied.MatchingSubExprs = deepCopyMatchingSubExprArr(fieldToCopy.MatchingSubExprs)
	return copied
}
func deepCopyMatchingSubExprArr(fieldToCopy []eval.MatchingSubExpr) []eval.MatchingSubExpr {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]eval.MatchingSubExpr, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchingSubExpr(fieldToCopy[i])
	}
	return copied
}
func deepCopyMatchingValue(fieldToCopy eval.MatchingValue) eval.MatchingValue {
	copied := eval.MatchingValue{}
	copied.Field = fieldToCopy.Field
	copied.Offset = fieldToCopy.Offset
	return copied
}
func deepCopyMatchingSubExpr(fieldToCopy eval.MatchingSubExpr) eval.MatchingSubExpr {
	copied := eval.MatchingSubExpr{}
	copied.Offset = fieldToCopy.Offset
	copied.ValueA = deepCopyMatchingValue(fieldToCopy.ValueA)
	copied.ValueB = deepCopyMatchingValue(fieldToCopy.ValueB)
	return copied
}
func deepCopyMatchedRulePtrArr(fieldToCopy []*MatchedRule) []*MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]*MatchedRule, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchedRulePtr(fieldToCopy[i])
	}
	return copied
}
func deepCopystringMap(fieldToCopy map[string]string) map[string]string {
	if fieldToCopy == nil {
		return nil
	}
	copied := make(map[string]string, len(fieldToCopy))
	for k, v := range fieldToCopy {
		copied[k] = v
	}
	return copied
}
func deepCopyMatchedRulePtr(fieldToCopy *MatchedRule) *MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	copied := &MatchedRule{}
	copied.PolicyName = fieldToCopy.PolicyName
	copied.PolicyVersion = fieldToCopy.PolicyVersion
	copied.RuleID = fieldToCopy.RuleID
	copied.RuleTags = deepCopystringMap(fieldToCopy.RuleTags)
	copied.RuleVersion = fieldToCopy.RuleVersion
	return copied
}
func deepCopySecurityProfileContext(fieldToCopy SecurityProfileContext) SecurityProfileContext {
	copied := SecurityProfileContext{}
	copied.EventTypeState = fieldToCopy.EventTypeState
	copied.EventTypes = deepCopyEventTypeArr(fieldToCopy.EventTypes)
	copied.Name = fieldToCopy.Name
	copied.Tags = deepCopystringArr(fieldToCopy.Tags)
	copied.Version = fieldToCopy.Version
	return copied
}
func deepCopyEventTypeArr(fieldToCopy []EventType) []EventType {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]EventType, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyChangePermissionEvent(fieldToCopy ChangePermissionEvent) ChangePermissionEvent {
	copied := ChangePermissionEvent{}
	copied.NewSd = fieldToCopy.NewSd
	copied.ObjectName = fieldToCopy.ObjectName
	copied.ObjectType = fieldToCopy.ObjectType
	copied.OldSd = fieldToCopy.OldSd
	copied.UserDomain = fieldToCopy.UserDomain
	copied.UserName = fieldToCopy.UserName
	return copied
}
func deepCopyCreateNewFileEvent(fieldToCopy CreateNewFileEvent) CreateNewFileEvent {
	copied := CreateNewFileEvent{}
	copied.File = deepCopyFimFileEvent(fieldToCopy.File)
	return copied
}
func deepCopyFimFileEvent(fieldToCopy FimFileEvent) FimFileEvent {
	copied := FimFileEvent{}
	copied.BasenameStr = fieldToCopy.BasenameStr
	copied.Extension = fieldToCopy.Extension
	copied.FileObject = fieldToCopy.FileObject
	copied.PathnameStr = fieldToCopy.PathnameStr
	copied.UserPathnameStr = fieldToCopy.UserPathnameStr
	return copied
}
func deepCopyCreateRegistryKeyEvent(fieldToCopy CreateRegistryKeyEvent) CreateRegistryKeyEvent {
	copied := CreateRegistryKeyEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry)
	return copied
}
func deepCopyRegistryEvent(fieldToCopy RegistryEvent) RegistryEvent {
	copied := RegistryEvent{}
	copied.KeyName = fieldToCopy.KeyName
	copied.KeyPath = fieldToCopy.KeyPath
	return copied
}
func deepCopyDeleteFileEvent(fieldToCopy DeleteFileEvent) DeleteFileEvent {
	copied := DeleteFileEvent{}
	copied.File = deepCopyFimFileEvent(fieldToCopy.File)
	return copied
}
func deepCopyDeleteRegistryKeyEvent(fieldToCopy DeleteRegistryKeyEvent) DeleteRegistryKeyEvent {
	copied := DeleteRegistryKeyEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry)
	return copied
}
func deepCopyExecEvent(fieldToCopy ExecEvent) ExecEvent {
	copied := ExecEvent{}
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process)
	return copied
}
func deepCopyExitEvent(fieldToCopy ExitEvent) ExitEvent {
	copied := ExitEvent{}
	copied.Cause = fieldToCopy.Cause
	copied.Code = fieldToCopy.Code
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process)
	return copied
}
func deepCopyOpenRegistryKeyEvent(fieldToCopy OpenRegistryKeyEvent) OpenRegistryKeyEvent {
	copied := OpenRegistryKeyEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry)
	return copied
}
func deepCopyRenameFileEvent(fieldToCopy RenameFileEvent) RenameFileEvent {
	copied := RenameFileEvent{}
	copied.New = deepCopyFimFileEvent(fieldToCopy.New)
	copied.Old = deepCopyFimFileEvent(fieldToCopy.Old)
	return copied
}
func deepCopySetRegistryKeyValueEvent(fieldToCopy SetRegistryKeyValueEvent) SetRegistryKeyValueEvent {
	copied := SetRegistryKeyValueEvent{}
	copied.Registry = deepCopyRegistryEvent(fieldToCopy.Registry)
	copied.ValueName = fieldToCopy.ValueName
	return copied
}
func deepCopyWriteFileEvent(fieldToCopy WriteFileEvent) WriteFileEvent {
	copied := WriteFileEvent{}
	copied.File = deepCopyFimFileEvent(fieldToCopy.File)
	return copied
}
