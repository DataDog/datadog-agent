// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package detectors

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	model "github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func NewInjectorDetector() model.Detector {
	return injectorDetector{}
}

type injectorDetector struct{}

func (injectorDetector) DetectLanguage(proc model.Process) (model.Language, error) {
	pid := proc.GetPid()
	path, found := findInjectorFile(int(pid))
	if !found {
		return model.Language{}, errors.New("no injector file found")
	}

	f, err := os.Open(path)
	if err != nil {
		return model.Language{}, errors.New("could not open injector file")
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return model.Language{}, errors.New("could not read injector file")
	}

	var name model.LanguageName
	switch string(data) {
	case "nodejs", "js", "node":
		name = model.Node
	case "php":
		name = model.PHP
	case "jvm":
		name = model.Java
	case "python":
		name = model.Python
	case "ruby":
		name = model.Ruby
	case "dotnet":
		name = model.Dotnet
	default:
		return model.Language{}, fmt.Errorf("unknonw language detected %s", data)
	}

	return model.Language{
		Name: name,
	}, nil
}

// findInjectorFile searches for the injector file in the process open file descriptors.
// In order to find the correct file, we
// need to iterate the list of files (named after file descriptor numbers) in
// /proc/$PID/fd and get the name from the target of the symbolic link.
//
// ```
// $ ls -l /proc/1750097/fd/
// total 0
// lrwx------ 1 foo foo 64 Aug 13 14:24 0 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 1 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 2 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 3 -> '/memfd:dd_language (deleted)'
// ```
func findInjectorFile(pid int) (string, bool) {
	fdsPath := kernel.HostProc(strconv.Itoa(pid), "fd")
	// quick path, the shadow file is the first opened file by the process
	// unless there are inherited fds
	path := filepath.Join(fdsPath, "3")
	if isInjectorFile(path) {
		return path, true
	}
	fdDir, err := os.Open(fdsPath)
	if err != nil {
		log.Warnf("failed to open %s: %s", fdsPath, err)
		return "", false
	}
	defer fdDir.Close()
	fds, err := fdDir.Readdirnames(-1)
	if err != nil {
		log.Warnf("failed to read %s: %s", fdsPath, err)
		return "", false
	}
	for _, fd := range fds {
		switch fd {
		case "0", "1", "2", "3":
			continue
		default:
			path := filepath.Join(fdsPath, fd)
			if isInjectorFile(path) {
				return path, true
			}
		}
	}
	return "", false
}

func isInjectorFile(path string) bool {
	name, err := os.Readlink(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(name, "memfd:dd_langauge")
}
