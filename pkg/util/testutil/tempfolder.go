// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// TempFolder is a temporary folder used for testing
type TempFolder struct {
	RootPath string
}

// NewTempFolder creates a new temporary folder
func NewTempFolder(namePrefix string) (*TempFolder, error) {
	path, err := ioutil.TempDir("", namePrefix)
	if err != nil {
		return nil, err
	}
	return &TempFolder{path}, nil
}

// RemoveAll purges a TempFolder
func (f *TempFolder) RemoveAll() error {
	return os.RemoveAll(f.RootPath)
}

// Add adds a file to a temp folder
func (f *TempFolder) Add(fileName string, contents string) error {
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

func (f *TempFolder) delete(fileName string) error {
	return os.Remove(filepath.Join(f.RootPath, fileName))
}

// Detab removes whitespace from the front of a string on every line
func Detab(str string) string {
	detabbed := make([]string, 0)
	for _, l := range strings.Split(str, "\n") {
		s := strings.TrimSpace(l)
		if len(s) > 0 {
			detabbed = append(detabbed, s)
		}
	}
	return strings.Join(detabbed, "\n")
}
