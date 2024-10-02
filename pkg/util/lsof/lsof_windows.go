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

func openFiles(pid int) (Files, error) {
	handle, err := windows.OpenProcess(windows.STANDARD_RIGHTS_ALL|windows.SPECIFIC_RIGHTS_ALL, false, uint32(pid))
	if err != nil {
		return nil, err
	}

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
		file, err := ofl.getDLLFile(process, hModule)
		if err != nil {
			continue
		}
		files = append(files, file)
	}

	return files, nil
}

func (ofl *openFilesLister) getDLLFile(process windows.Handle, hModule windows.Handle) (File, error) {
	// get DLL file path
	modPath, err := ofl.getModulePath(process, hModule)
	if err != nil {
		return File{}, err
	}

	// try to get some permissions and file size
	filePerms := "<unknown>"
	var fileSize int64 = -1
	fileInfo, err := ofl.Stat(modPath)
	if err == nil {
		fileSize = fileInfo.Size()
		filePerms = fileInfo.Mode().Perm().String()
	}

	file := File{
		Fd:       fmt.Sprintf("%d", hModule),
		Type:     "DLL",
		FilePerm: filePerms,
		Size:     fileSize,
		Name:     modPath,
	}
	return file, nil
}

// listOpenDLL returns the list of DLLs opened by the process
func (ofl *openFilesLister) listOpenDLL(process windows.Handle) ([]windows.Handle, error) {
	var hModules [1024]windows.Handle
	var cbNeeded uint32

	if err := ofl.EnumProcessModules(process, &hModules[0], uint32(unsafe.Sizeof(hModules)), &cbNeeded); err != nil {
		return nil, err
	}

	nModules := int(cbNeeded / uint32(unsafe.Sizeof(hModules[0])))
	return hModules[:nModules], nil
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
