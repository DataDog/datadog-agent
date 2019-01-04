// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package docker

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type tempFolder struct {
	RootPath string
}

func newTempFolder(namePrefix string) (*tempFolder, error) {
	path, err := ioutil.TempDir("", namePrefix)
	if err != nil {
		return nil, err
	}
	return &tempFolder{path}, nil
}
func (f *tempFolder) removeAll() error {
	return os.RemoveAll(f.RootPath)
}

func (f *tempFolder) add(fileName string, contents string) error {
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

func (f *tempFolder) delete(fileName string) error {
	return os.Remove(filepath.Join(f.RootPath, fileName))
}

// detab removes whitespace from the front of a string on every line
func detab(str string) string {
	detabbed := make([]string, 0)
	for _, l := range strings.Split(str, "\n") {
		s := strings.TrimSpace(l)
		if len(s) > 0 {
			detabbed = append(detabbed, s)
		}
	}
	return strings.Join(detabbed, "\n")
}
