// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows"
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
	errDiscardedPath = errors.New("discarded path")
)

/*
 * these constants are defined in the windows driver kit (wdm.h).  Copied
 * here because the correspond to the createOptions field
 */
const (
	kernelDisposition_FILE_SUPERSEDE           = uint32(0x00000000) // nolint:unused,revive
	kernelDisposition_FILE_OPEN                = uint32(0x00000001) // nolint:unused,revive
	kernelDisposition_FILE_CREATE              = uint32(0x00000002) // nolint:unused,revive
	kernelDisposition_FILE_OPEN_IF             = uint32(0x00000003) // nolint:unused,revive
	kernelDisposition_FILE_OVERWRITE           = uint32(0x00000004) // nolint:unused,revive
	kernelDisposition_FILE_OVERWRITE_IF        = uint32(0x00000005) // nolint:unused,revive
	kernelDisposition_FILE_MAXIMUM_DISPOSITION = uint32(0x00000005) // nolint:unused,revive
)

const (
	kernelCreateOpts_FILE_DIRECTORY_FILE            = uint32(0x00000001) // nolint:unused,revive
	kernelCreateOpts_FILE_WRITE_THROUGH             = uint32(0x00000002) // nolint:unused,revive
	kernelCreateOpts_FILE_SEQUENTIAL_ONLY           = uint32(0x00000004) // nolint:unused,revive
	kernelCreateOpts_FILE_NO_INTERMEDIATE_BUFFERING = uint32(0x00000008) // nolint:unused,revive

	kernelCreateOpts_FILE_SYNCHRONOUS_IO_ALERT    = uint32(0x00000010) // nolint:unused,revive
	kernelCreateOpts_FILE_SYNCHRONOUS_IO_NONALERT = uint32(0x00000020) // nolint:unused,revive
	kernelCreateOpts_FILE_NON_DIRECTORY_FILE      = uint32(0x00000040) // nolint:unused,revive
	kernelCreateOpts_FILE_CREATE_TREE_CONNECTION  = uint32(0x00000080) // nolint:unused,revive

	kernelCreateOpts_FILE_COMPLETE_IF_OPLOCKED = uint32(0x00000100) // nolint:unused,revive
	kernelCreateOpts_FILE_NO_EA_KNOWLEDGE      = uint32(0x00000200) // nolint:unused,revive
	kernelCreateOpts_FILE_OPEN_REMOTE_INSTANCE = uint32(0x00000400) // nolint:unused,revive
	kernelCreateOpts_FILE_RANDOM_ACCESS        = uint32(0x00000800) // nolint:unused,revive

	kernelCreateOpts_FILE_DELETE_ON_CLOSE        = uint32(0x00001000) // nolint:unused,revive
	kernelCreateOpts_FILE_OPEN_BY_FILE_ID        = uint32(0x00002000) // nolint:unused,revive
	kernelCreateOpts_FILE_OPEN_FOR_BACKUP_INTENT = uint32(0x00004000) // nolint:unused,revive
	kernelCreateOpts_FILE_NO_COMPRESSION         = uint32(0x00008000) // nolint:unused,revive
)

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
func (wp *WindowsProbe) parseCreateHandleArgs(e *etw.DDEventRecord) (fileObjectPointer, fileCache, error) {

	/*
		version 0
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
	   	version 1
	    	<data name="Irp" inType="win:Pointer"/>
	         <data name="FileObject" inType="win:Pointer"/>
	         <data name="IssuingThreadId" inType="win:UInt32"/>
	         <data name="CreateOptions" inType="win:UInt32"/>
	         <data name="CreateAttributes" inType="win:UInt32"/>
	         <data name="ShareAccess" inType="win:UInt32"/>
	         <data name="FileName" inType="win:UnicodeString"/>
	*/
	var fileName string
	var fileObject fileObjectPointer

	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {

		fileObject = fileObjectPointer(data.GetUint64(16))

		fileName, _, _, _ = data.ParseUnicodeString(36)

	} else if e.EventHeader.EventDescriptor.Version == 1 {

		fileObject = fileObjectPointer(data.GetUint64(8))

		fileName, _, _, _ = data.ParseUnicodeString(32)

	} else {
		return 0, fileCache{}, fmt.Errorf("unknown version %v", e.EventHeader.EventDescriptor.Version)
	}

	// not amazing to double compute the basename..
	basename := filepath.Base(fileName)

	if !wp.approveFimBasename(basename) {
		wp.discardedFileHandles.Add(fileObjectPointer(fileObject), struct{}{})
		wp.stats.createFileApproverRejects++
		return 0, fileCache{}, errDiscardedPath
	}

	if _, ok := wp.discardedPaths.Get(fileName); ok {
		wp.discardedFileHandles.Add(fileObjectPointer(fileObject), struct{}{})
		wp.stats.fileCreateSkippedDiscardedPaths++
		return 0, fileCache{}, errDiscardedPath
	}

	userFileName := wp.mustConvertDrivePath(fileName)
	if _, ok := wp.discardedUserPaths.Get(userFileName); ok {
		wp.stats.fileCreateSkippedDiscardedPaths++
		return 0, fileCache{}, errDiscardedPath
	}

	if _, ok := wp.discardedBasenames.Get(basename); ok {
		wp.discardedFileHandles.Add(fileObjectPointer(fileObject), struct{}{})
		wp.stats.fileCreateSkippedDiscardedBasenames++
		return 0, fileCache{}, errDiscardedPath
	}

	// lru is thread safe, has its own locking
	fc := fileCache{
		fileName:     fileName,
		userFileName: userFileName,
		baseName:     basename,
	}
	if wp.filePathResolver.Add(fileObject, fc) {
		wp.stats.fileNameCacheEvictions++
	}
	// if we get here, we have a new file handle.  Remove it from the discarder cache in case
	// we missed the close notification
	wp.discardedFileHandles.Remove(fileObjectPointer(ca.fileObject))

	return fileObject, fc, nil
}

func (wp *WindowsProbe) onCreateHandleArgs(e *etw.DDEventRecord) {
	// for the createHandle, we don't need to do anything other than ensure that the
	// file is processed and added to the cache.
	_, _, _ = wp.parseCreateHandleArgs(e)
}

func (wp *WindowsProbe) onCreateNewFileArgs(e *etw.DDEventRecord) *model.Event {
	fo, fc, err := wp.parseCreateHandleArgs(e)
	if err != nil {
		return nil
	}
	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.CreateNewFileEventType)
	ev.CreateNewFile = model.CreateNewFileEvent{
		File: model.FimFileEvent{
			FileObject:      uint64(fo),
			PathnameStr:     fc.fileName,
			UserPathnameStr: fc.userFileName,
			BasenameStr:     fc.baseName,
		},
	}
	return ev
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
// nolint: unused

// nolint: unused
func (wp *WindowsProbe) parseInformationArgs(e *etw.DDEventRecord) (fileObjectPointer, fileCache, error) {
	var fileObject fileObjectPointer
	var fc fileCache

	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {

		fileObject = fileObjectPointer(data.GetUint64(16))

	} else if e.EventHeader.EventDescriptor.Version == 1 {

		fileObject = fileObjectPointer(data.GetUint64(8))

	} else {
		return fileObject, fc, fmt.Errorf("unknown version number %v", e.EventHeader.EventDescriptor.Version)
	}

	if _, ok := wp.discardedFileHandles.Get(fileObjectPointer(fileObject)); ok {
		return fileObject, fc, errDiscardedPath
	}
	// lru is thread safe, has its own locking
	var ok bool
	if fc, ok = wp.filePathResolver.Get(fileObjectPointer(fileObject)); !ok {
		return fileObject, fc, errDiscardedPath
	}

	return fileObject, fc, nil
}

func (wp *WindowsProbe) onRenameArgs(e *etw.DDEventRecord) {
	fo, fc, err := wp.parseInformationArgs(e)
	if err != nil {
		// couldn't parse/be found, nothing to do
	}
	// otherwise, add to the rename prArgs cache
	wp.renamePreArgs.Add(uint64(fo), fc)
	return
}

func (wp *WindowsProbe) onRenamePath(e *etw.DDEventRecord) *model.Event {
	fo, fc, err := wp.parseDeletePathArgs(e)
	if err != nil {
		// couldn't parse/be found, nothing to do
		return nil
	}
	oldfc, found := wp.renamePreArgs.Get(uint64(fo))
	if !found {
		return nil
	}
	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.FileRenameEventType)
	ev.RenameFile = model.RenameFileEvent{
		Old: model.FimFileEvent{
			FileObject:      uint64(fo),
			PathnameStr:     oldfc.fileName,
			UserPathnameStr: oldfc.userFileName,
			BasenameStr:     oldfc.baseName,
		},
		New: model.FimFileEvent{
			FileObject:      uint64(fo),
			PathnameStr:     fc.fileName,
			UserPathnameStr: fc.userFileName,
			BasenameStr:     fc.baseName,
		},
	}
	wp.renamePreArgs.Remove(uint64(fo))
	return ev
}

func (wp *WindowsProbe) onSetDelete(e *etw.DDEventRecord) *model.Event {
	fo, fc, err := wp.parseInformationArgs(e)
	if err != nil {
		// couldn't parse/be found, nothing to do
		return nil
	}
	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.DeleteFileEventType)
	ev.DeleteFile = model.DeleteFileEvent{
		File: model.FimFileEvent{
			FileObject:      uint64(fo),
			PathnameStr:     fc.fileName,
			UserPathnameStr: fc.userFileName,
			BasenameStr:     fc.baseName,
		},
	}

	return ev
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
	etw.DDEventHeader
	irp          uint64
	threadID     uint64
	fileObject   fileObjectPointer
	fileKey      uint64
	fileName     string
	userFileName string
}

// nolint: unused
type closeArgs cleanupArgs

// nolint: unused
type flushArgs cleanupArgs

func (wp *WindowsProbe) parseCleanupArgs(e *etw.DDEventRecord) (fileObjectPointer, error) {
	var fileObject fileObjectPointer

	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {

		fileObject = fileObjectPointer(data.GetUint64(16))

	} else if e.EventHeader.EventDescriptor.Version == 1 {

		fileObject = fileObjectPointer(data.GetUint64(8))

	} else {
		return fileObject, fmt.Errorf("unknown version number %v", e.EventHeader.EventDescriptor.Version)
	}

	if _, ok := wp.discardedFileHandles.Get(fileObjectPointer(fileObject)); ok {
		return fileObject, errDiscardedPath
	}

	return fileObject, nil
}

func (wp *WindowsProbe) onCloseArgs(e *etw.DDEventRecord) {
	fo, err := wp.parseCleanupArgs(e)
	if err != nil {
		// couldn't parse/be found, nothing to do
		return
	}
	// otherwise, add to the close prArgs cache
	wp.filePathResolver.Remove(fo)
}
func (wp *WindowsProbe) parseReadWriteArgs(e *etw.DDEventRecord) (fileObjectPointer, fileCache, error) {
	var fileObject fileObjectPointer
	var fc fileCache
	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {

		fileObject = fileObjectPointer(data.GetUint64(24))

	} else if e.EventHeader.EventDescriptor.Version == 1 {

		fileObject = fileObjectPointer(data.GetUint64(16))

	} else {
		return fileObject, fc, fmt.Errorf("unknown version number %v", e.EventHeader.EventDescriptor.Version)
	}
	if _, ok := wp.discardedFileHandles.Get(fileObjectPointer(fileObject)); ok {
		return fileObject, fc, errDiscardedPath
	}
	// lru is thread safe, has its own locking
	var ok bool
	if fc, ok = wp.filePathResolver.Get(fileObjectPointer(fileObject)); ok {
		return fileObject, fc, nil
	}
	return fileObject, fc, errDiscardedPath
}

func (wp *WindowsProbe) onWriteArgs(e *etw.DDEventRecord) *model.Event {
	fo, fc, err := wp.parseReadWriteArgs(e)
	if err != nil {
		return nil
	}
	ev, err := wp.eventCache.Get()
	if err != nil {
		wp.stats.eventCacheUnderflow++
		return nil
	}
	ev.Type = uint32(model.WriteFileEventType)
	ev.WriteFile = model.WriteFileEvent{
		File: model.FimFileEvent{
			FileObject:      uint64(fo),
			PathnameStr:     fc.fileName,
			UserPathnameStr: fc.userFileName,
			BasenameStr:     fc.baseName,
		},
	}
	return ev
}

/*
	     <template tid="DeletePathArgs">
	      <data name="Irp" inType="win:Pointer"/>
	      <data name="ThreadId" inType="win:Pointer"/>
	      <data name="FileObject" inType="win:Pointer"/>
	      <data name="FileKey" inType="win:Pointer"/>
	      <data name="ExtraInformation" inType="win:Pointer"/>
	      <data name="InfoClass" inType="win:UInt32"/>
	      <data name="FilePath" inType="win:UnicodeString"/>
	     </template>
		      <template tid="DeletePathArgs_V1">
	      <data name="Irp" inType="win:Pointer"/>
	      <data name="FileObject" inType="win:Pointer"/>
	      <data name="FileKey" inType="win:Pointer"/>
	      <data name="ExtraInformation" inType="win:Pointer"/>
	      <data name="IssuingThreadId" inType="win:UInt32"/>
	      <data name="InfoClass" inType="win:UInt32"/>
	      <data name="FilePath" inType="win:UnicodeString"/>
	     </template>
*/

func (wp *WindowsProbe) parseDeletePathArgs(e *etw.DDEventRecord) (fileObjectPointer, fileCache, error) {
	var fileObject fileObjectPointer
	var fileName string
	var fc fileCache

	data := etwimpl.GetUserData(e)
	if e.EventHeader.EventDescriptor.Version == 0 {
		
		fileObject = fileObjectPointer(data.GetUint64(16))
		
		fileName, _, _, _ = data.ParseUnicodeString(44)

	} else if e.EventHeader.EventDescriptor.Version == 1 {
		
		fileObject = fileObjectPointer(data.GetUint64(8))
		fileName, _, _, _ = data.ParseUnicodeString(40)
	}
	
	if _, ok := wp.discardedFileHandles.Get(fileObjectPointer(fileObject)); ok {
		return fileObject, fc , errDiscardedPath
	}
	basename := filepath.Base(fileName)
	userFileName := wp.mustConvertDrivePath(fileName)
	fc = fileCache {
		fileName: fileName,
		userFileName: userFileName,
		baseName: basename,
	}
	return fileObject, fc, nil
}


// nolint: unused
func (wp *WindowsProbe) convertDrivePath(devicefilename string) (string, error) {
	// filepath doesn't seem to like the \Device\HarddiskVolume1 format
	pathchunks := strings.Split(devicefilename, "\\")
	if len(pathchunks) > 2 {
		if strings.EqualFold(pathchunks[1], "device") {
			pathchunks[2] = wp.volumeMap[strings.ToLower(pathchunks[2])]
			return filepath.Join(pathchunks[2:]...), nil
		}
	}
	return "", fmt.Errorf("Unable to parse path %v", devicefilename)
}

func (wp *WindowsProbe) mustConvertDrivePath(devicefilename string) string {
	userPath, err := wp.convertDrivePath(devicefilename)
	if err != nil {
		seclog.Errorf("failed to convert drive path: %v", err)
		return devicefilename
	}
	return userPath
}

func (wp *WindowsProbe) initializeVolumeMap() error {

	buf := make([]uint16, 1024)
	bufferLength := uint32(len(buf))

	_, err := windows.GetLogicalDriveStrings(bufferLength, &buf[0])
	if err != nil {
		return err
	}
	drives := winutil.ConvertWindowsStringList(buf)
	for _, drive := range drives {
		t := windows.GetDriveType(windows.StringToUTF16Ptr(drive[:3]))
		/*
			DRIVE_UNKNOWN
			0
			The drive type cannot be determined.
			DRIVE_NO_ROOT_DIR
			1
			The root path is invalid; for example, there is no volume mounted at the specified path.
			DRIVE_REMOVABLE
			2
			The drive has removable media; for example, a floppy drive, thumb drive, or flash card reader.
			DRIVE_FIXED
			3
			The drive has fixed media; for example, a hard disk drive or flash drive.
			DRIVE_REMOTE
			4
			The drive is a remote (network) drive.
			DRIVE_CDROM
			5
			The drive is a CD-ROM drive.
			DRIVE_RAMDISK
			6
			The drive is a RAM disk.
		*/
		if t == windows.DRIVE_FIXED {
			volpath := make([]uint16, 1024)
			vollen := uint32(len(volpath))
			_, err = windows.QueryDosDevice(windows.StringToUTF16Ptr(drive[:2]), &volpath[0], vollen)
			if err == nil {
				devname := windows.UTF16PtrToString(&volpath[0])
				paths := strings.Split(devname, "\\") // apparently, filepath.split doesn't like volume names

				if len(paths) > 2 {
					// the \Device leads to the first entry being empty
					if strings.EqualFold(paths[1], "device") {
						wp.volumeMap[strings.ToLower(paths[2])] = drive
					}
				}
			}
		}
	}
	return nil
}
