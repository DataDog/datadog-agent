// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

import (
	"fmt"
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

type dummyCgroupStat map[string]uint64

func (c dummyCgroupStat) String() string {

	lines := make([]string, len(c))
	var i int
	for k, v := range c {
		lines[i] = fmt.Sprintf("%s %d", k, v)
		i++
	}

	return strings.Join(lines, "\n")
}

func newDummyContainerCgroup(rootPath string, targets ...string) *ContainerCgroup {
	cgroup := &ContainerCgroup{
		ContainerID: "dummy",
		Mounts:      make(map[string]string),
		Paths:       make(map[string]string),
	}
	for _, target := range targets {
		cgroup.Mounts[target] = rootPath
		cgroup.Paths[target] = target
	}
	return cgroup
}
