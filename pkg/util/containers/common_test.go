// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build linux

package containers

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type tempProc struct {
	RootPath string
}

func newTempProc(namePrefix string) (*tempProc, error) {
	path, err := ioutil.TempDir("", namePrefix)
	if err != nil {
		return nil, err
	}
	os.Setenv("HOST_PROC", path)
	return &tempProc{path}, nil
}

func (f *tempProc) delete(fileName string) error {
	return os.Remove(filepath.Join(f.RootPath, fileName))
}

func (f *tempProc) removeAll() error {
	os.Unsetenv("HOST_PROC")
	return os.RemoveAll(f.RootPath)
}

func (f *tempProc) addFile(fileName, contents string) error {
	filePath := filepath.Join(f.RootPath, fileName)
	dirPath := filepath.Dir(filePath)
	err := os.MkdirAll(dirPath, 0777)
	if err != nil {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	_, err = file.WriteString(contents)
	return err
}

func (f *tempProc) addDummyStatus(pid string, fields map[string]string) error {
	var contents string
	for field, value := range fields {
		contents += fmt.Sprintf("%s:\t%s\n", field, value)
	}
	return f.addFile(filepath.Join(pid, "status"), contents)
}

func (f *tempProc) addDummyProcess(pid, ppid, cmdline string) error {
	err := f.addFile(filepath.Join(pid, "cmdline"), strings.Replace(cmdline, " ", "\u0000", -1))
	if err != nil {
		return err
	}
	err = f.addDummyStatus(pid, map[string]string{"Ppid": ppid})
	return err
}
