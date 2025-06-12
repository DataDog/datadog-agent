// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FieldHandlers defines a field handlers
type FieldHandlers struct {
	*BaseFieldHandlers
	resolvers *resolvers.Resolvers
}

// NewFieldHandlers returns a new FieldHandlers
func NewFieldHandlers(config *config.Config, resolvers *resolvers.Resolvers, hostname string) (*FieldHandlers, error) {
	bfh, err := NewBaseFieldHandlers(config, hostname)
	if err != nil {
		return nil, err
	}

	return &FieldHandlers{
		BaseFieldHandlers: bfh,
		resolvers:         resolvers,
	}, nil
}

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

// ResolveFilePath resolves the inode to a full path
func (fh *FieldHandlers) ResolveFilePath(_ *model.Event, f *model.FileEvent) string {
	return f.PathnameStr
}

// ResolveFileUserPath resolves the inode to a full user path
func (fh *FieldHandlers) ResolveFileUserPath(_ *model.Event, f *model.FimFileEvent) string {
	return f.UserPathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (fh *FieldHandlers) ResolveFileBasename(_ *model.Event, f *model.FileEvent) string {
	return f.BasenameStr
}

// ResolveFimFilePath resolves the inode to a full path
func (fh *FieldHandlers) ResolveFimFilePath(_ *model.Event, f *model.FimFileEvent) string {
	return f.PathnameStr
}

// ResolveFimFileBasename resolves the inode to a full path
func (fh *FieldHandlers) ResolveFimFileBasename(_ *model.Event, f *model.FimFileEvent) string {
	return f.BasenameStr
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (fh *FieldHandlers) ResolveProcessEnvp(_ *model.Event, process *model.Process) []string {
	return fh.resolvers.ProcessResolver.GetEnvp(process)
}

// ResolveProcessEnvs resolves the envs of the event
func (fh *FieldHandlers) ResolveProcessEnvs(_ *model.Event, process *model.Process) []string {
	return fh.resolvers.ProcessResolver.GetEnvs(process)
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *FieldHandlers) ResolveProcessCacheEntry(ev *model.Event, _ func(*model.ProcessCacheEntry, error)) (*model.ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid)
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid)
		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// ResolveProcessCacheEntryFromPID queries the ProcessResolver to retrieve the ProcessContext of the provided PID
func (fh *FieldHandlers) ResolveProcessCacheEntryFromPID(pid uint32) *model.ProcessCacheEntry {
	return fh.resolvers.ProcessResolver.Resolve(pid)
}

// ResolveProcessCmdLineScrubbed returns a scrubbed version of the cmdline
func (fh *FieldHandlers) ResolveProcessCmdLineScrubbed(_ *model.Event, e *model.Process) string {
	return fh.resolvers.ProcessResolver.GetProcessCmdLineScrubbed(e)
}

// ResolveUser resolves the user name
func (fh *FieldHandlers) ResolveUser(_ *model.Event, process *model.Process) string {
	return fh.resolvers.UserGroupResolver.GetUser(process.OwnerSidString)
}

// ResolveContainerContext retrieve the ContainerContext of the event
func (fh *FieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	return ev.ContainerContext, ev.ContainerContext != nil
}

// ResolveContainerRuntime retrieves the container runtime managing the container
func (fh *FieldHandlers) ResolveContainerRuntime(_ *model.Event, _ *model.ContainerContext) string {
	return ""
}

// ResolveContainerCreatedAt resolves the container creation time of the event
func (fh *FieldHandlers) ResolveContainerCreatedAt(_ *model.Event, e *model.ContainerContext) int {
	return int(e.CreatedAt)
}

// ResolveContainerID resolves the container ID of the event
func (fh *FieldHandlers) ResolveContainerID(_ *model.Event, e *model.ContainerContext) string {
	return string(e.ContainerID)
}

// ResolveContainerTags resolves the container tags of the event
func (fh *FieldHandlers) ResolveContainerTags(_ *model.Event, e *model.ContainerContext) []string {
	return e.Tags
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTimestamp(_ *model.Event, e *model.BaseEvent) int {
	return int(e.TimestampRaw)
}

// ResolveProcessCmdLine resolves the cmd line of the process of the event
func (fh *FieldHandlers) ResolveProcessCmdLine(_ *model.Event, e *model.Process) string {
	return e.CmdLine
}

// ResolveProcessCreatedAt resolves the process creation time of the event
func (fh *FieldHandlers) ResolveProcessCreatedAt(_ *model.Event, e *model.Process) int {
	return int(e.CreatedAt)
}

// ResolveOldSecurityDescriptor resolves the old security descriptor
func (fh *FieldHandlers) ResolveOldSecurityDescriptor(_ *model.Event, cp *model.ChangePermissionEvent) string {
	hrsd, err := fh.resolvers.SecurityDescriptorResolver.GetHumanReadableSD(cp.OldSd)
	if err != nil {
		return cp.OldSd
	}
	return hrsd
}

// ResolveNewSecurityDescriptor resolves the old security descriptor
func (fh *FieldHandlers) ResolveNewSecurityDescriptor(_ *model.Event, cp *model.ChangePermissionEvent) string {
	hrsd, err := fh.resolvers.SecurityDescriptorResolver.GetHumanReadableSD(cp.NewSd)
	if err != nil {
		return cp.NewSd
	}
	return hrsd
}

// ResolveFileMetadataSize resolves file metadata size
func (fh *FieldHandlers) ResolveFileMetadataSize(_ *model.Event, _ *model.FileMetadata) int {
	return 0
}

// ResolveFileMetadataType resolves file metadata type
func (fh *FieldHandlers) ResolveFileMetadataType(_ *model.Event, _ *model.FileMetadata) int {
	return 0
}

// ResolveFileMetadataIsExecutable resolves file metadata is_executable
func (fh *FieldHandlers) ResolveFileMetadataIsExecutable(_ *model.Event, _ *model.FileMetadata) bool {
	return false
}

// ResolveFileMetadataArchitecture resolves file metadata architecture
func (fh *FieldHandlers) ResolveFileMetadataArchitecture(_ *model.Event, _ *model.FileMetadata) int {
	return 0
}

// ResolveFileMetadataABI resolves file metadata ABI
func (fh *FieldHandlers) ResolveFileMetadataABI(_ *model.Event, _ *model.FileMetadata) int {
	return 0
}

// ResolveFileMetadataIsUPXPacked resolves file metadata is_upx_packed
func (fh *FieldHandlers) ResolveFileMetadataIsUPXPacked(_ *model.Event, _ *model.FileMetadata) bool {
	return false
}

// ResolveFileMetadataCompression resolves file metadata compression
func (fh *FieldHandlers) ResolveFileMetadataCompression(_ *model.Event, _ *model.FileMetadata) int {
	return 0
}

// ResolveFileMetadataIsGarbleObfuscated resolves file metadata is_garble_obfuscated
func (fh *FieldHandlers) ResolveFileMetadataIsGarbleObfuscated(_ *model.Event, _ *model.FileMetadata) bool {
	return false
}
