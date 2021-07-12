// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

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

func newDindContainerCgroup(namePrefix, target, containerID string) (*tempFolder, *ContainerCgroup, error) {
	// first make a dir that matches the actual cgroup path(contains only one level of container id)
	path, err := ioutil.TempDir("", namePrefix)
	if err != nil {
		return nil, nil, err
	}

	actualPath := filepath.Join(path, "docker", containerID)
	err = os.MkdirAll(actualPath, 0777)
	if err != nil {
		return nil, nil, err
	}
	t := &tempFolder{actualPath}
	dindContainerID := "ada6d7f86865047ecbca0eedc44722173cf48c0ff7184a61ed56a80e7564bc0c"
	return t, &ContainerCgroup{
		ContainerID: "dummy",
		Mounts:      map[string]string{target: path},
		Paths:       map[string]string{target: filepath.Join("/docker", dindContainerID, "docker", containerID)},
	}, nil
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
