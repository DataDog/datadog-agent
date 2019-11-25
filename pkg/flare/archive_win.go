// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

package flare

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
)

func zipCounterStrings(tempDir, hostname string) error {
	bufferIncrement := uint32(1024)
	bufferSize := bufferIncrement
	var counterlist []uint16
	for {
		var regtype uint32
		counterlist = make([]uint16, bufferSize)
		var sz uint32
		sz = bufferSize
		regerr := windows.RegQueryValueEx(windows.HKEY_PERFORMANCE_DATA,
			windows.StringToUTF16Ptr("Counter 009"),
			nil, // reserved
			&regtype,
			(*byte)(unsafe.Pointer(&counterlist[0])),
			&sz)
		if regerr == error(windows.ERROR_MORE_DATA) {
			// buffer's not big enough
			bufferSize += bufferIncrement
			continue
		}
		break
	}
	clist := winutil.ConvertWindowsStringList(counterlist)
	fname := filepath.Join(tempDir, hostname, "counter_strings.txt")
	err := ensureParentDirsExist(fname)
	if err != nil {
		return err
	}
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	for i := 0; i < len(clist); i++ {
		f.WriteString(clist[i])
		f.WriteString("\r\n")
	}
	f.Sync()
	return nil

}

func zipTypeperfData(tempDir, hostname string) error {
	cmd := exec.Command("typeperf", "-qx")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return err
	}
	f := filepath.Join(tempDir, hostname, "typeperf.txt")
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, out.Bytes(), os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func (p permissionsInfos) add(filePath string) {}
func (p permissionsInfos) commit(tempDir, hostname string, mode os.FileMode) error {
	return nil
}
