// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package inuse

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// KernelFiles returns the paths of the files of the running kernel, those of
// the host packages under root that run outside of any process: its image,
// the list of the modules built into it, and its loaded modules. release
// returns the release of the running kernel, and loaded the names of its
// loaded modules.
func KernelFiles(root string, release func() (string, error), loaded func() ([]string, error)) []string {
	if release == nil {
		return nil
	}
	rel, err := release()
	if err != nil {
		seclog.Debugf("Failed to read the release of the running kernel: %v", err)
		return nil
	}
	dir := "/lib/modules/" + rel
	files := []string{"/boot/vmlinuz-" + rel, dir + "/vmlinuz", dir + "/modules.builtin"}
	if loaded == nil {
		return files
	}

	names, err := loaded()
	if err != nil {
		seclog.Debugf("Failed to list the loaded kernel modules: %v", err)
		return files
	}
	paths, err := modulePaths(root, rel)
	if err != nil {
		seclog.Debugf("Failed to read the modules of kernel %s: %v", rel, err)
		return files
	}
	for _, name := range names {
		if path, ok := paths[name]; ok {
			files = append(files, path)
		}
	}
	return files
}

// modulePaths returns the paths of the modules of kernel release under root,
// by module name, from its modules.dep. The modules of weak-updates link to
// the modules of another kernel, whose package owns them.
func modulePaths(root, release string) (map[string]string, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootFS.Close()

	var dir string
	var dep []byte
	for _, dir = range []string{"usr/lib/modules/" + release, "lib/modules/" + release} {
		if dep, err = rootFS.ReadFile(dir + "/modules.dep"); err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	paths := make(map[string]string)
	for line := range strings.Lines(string(dep)) {
		module, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		path := module
		if !filepath.IsAbs(path) {
			path = "/lib/modules/" + release + "/" + module
			if strings.HasPrefix(module, "weak-updates/") {
				if target, err := rootFS.Readlink(dir + "/" + module); err == nil {
					if !filepath.IsAbs(target) {
						target = filepath.Join(filepath.Dir(path), target)
					}
					path = target
				}
			}
		}
		paths[moduleName(module)] = path
	}
	return paths, nil
}

// moduleName returns the name of the kernel module in the file at path, as
// /proc/modules lists it.
func moduleName(path string) string {
	name := filepath.Base(path)
	if i := strings.Index(name, ".ko"); i >= 0 {
		name = name[:i]
	}
	return strings.ReplaceAll(name, "-", "_")
}

// LiveModules returns the names of the loaded kernel modules.
func LiveModules() ([]string, error) {
	modules, err := utils.FetchLoadedModules()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(modules))
	for name, module := range modules {
		if module.State == "Live" {
			names = append(names, name)
		}
	}
	return names, nil
}
