// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// permissions required to read DLLs loaded by another process
//
//nolint:revive
const (
	PROCESS_QUERY_INFORMATION = 0x0400
	PROCESS_VM_READ           = 0x0010
)

func openFiles(pid int) (Files, error) {
	handle, err := windows.OpenProcess(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	defer windows.Close(handle)

	ofl := &openFilesLister{
		windows.EnumProcessModules,
		windows.GetModuleFileNameEx,
		os.Stat,
	}

	files, err := ofl.getDLLFiles(handle)

	return files, err
}

type openFilesLister struct {
	EnumProcessModules  func(windows.Handle, *windows.Handle, uint32, *uint32) error
	GetModuleFileNameEx func(windows.Handle, windows.Handle, *uint16, uint32) error
	Stat                func(string) (os.FileInfo, error)
}

// getDLLFiles returns the list of File for the DLLs opened by the process
func (ofl *openFilesLister) getDLLFiles(process windows.Handle) (Files, error) {
	hModules, err := ofl.listOpenDLL(process)
	if err != nil {
		return nil, err
	}

	var files Files
	for _, hModule := range hModules {
		file := ofl.getDLLFile(process, hModule)
		files = append(files, file)
	}

	return files, nil
}

func (ofl *openFilesLister) getDLLFile(process windows.Handle, hModule windows.Handle) File {
	file := File{
		Fd:       fmt.Sprintf("%d", hModule),
		Type:     "DLL",
		FilePerm: "<unknown>",
		Size:     -1,
	}

	// get DLL file path
	modPath, err := ofl.getModulePath(process, hModule)
	if err != nil {
		file.Name = fmt.Sprintf("<error: %s>", err)
		return file
	}

	file.Name = modPath

	// try to get some permissions and file size
	fileStat, err := ofl.Stat(modPath)
	if err == nil {
		file.Size = fileStat.Size()
		file.FilePerm = fileStat.Mode().Perm().String()
	}

	return file
}

// listOpenDLL returns the list of DLLs opened by the process
func (ofl *openFilesLister) listOpenDLL(process windows.Handle) ([]windows.Handle, error) {
	var hModules []windows.Handle
	var cbNeeded uint32 = 1024

	var h windows.Handle
	handleTypeSize := uint32(unsafe.Sizeof(h))

	// retry until the module slice is big enough
	// the for loop is necessary in case new DLLs are loaded between calls to EnumProcessModules
	// in most cases the initial 1024 should be enough
	for cbNeeded > uint32(len(hModules)) {
		hModules = make([]windows.Handle, cbNeeded)
		if err := ofl.EnumProcessModules(process, &hModules[0], cbNeeded*handleTypeSize, &cbNeeded); err != nil {
			return nil, err
		}

		cbNeeded /= handleTypeSize
	}

	return hModules[:cbNeeded], nil
}

// getModulePath returns the path to the given module
func (ofl *openFilesLister) getModulePath(process windows.Handle, module windows.Handle) (string, error) {
	var modName [windows.MAX_PATH]uint16
	if err := ofl.GetModuleFileNameEx(process, module, &modName[0], windows.MAX_PATH); err != nil {
		return "", err
	}

	modPath := windows.UTF16ToString(modName[:])
	return modPath, nil
}
