// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"os"
	"path"
	"strings"
)

// maxSymlinks bounds the symlinks followed to resolve one directory, as the
// kernel bounds them.
const maxSymlinks = 40

// dirResolver resolves the directories of paths through the symlinks of a
// root, as the kernel resolves them in the paths of the files it reports.
type dirResolver struct {
	root *os.Root
	dirs map[string]string
}

func newDirResolver(root *os.Root) *dirResolver {
	return &dirResolver{root: root, dirs: map[string]string{"/": "/"}}
}

// path returns p, an absolute path in the root, with its directory resolved.
// Its last element stays as listed.
func (d *dirResolver) path(p string) string {
	dir, name := path.Split(path.Clean(p))
	if name == "" {
		return p
	}
	return path.Join(d.dir(path.Clean(dir)), name)
}

// dir returns p, a clean absolute directory in the root, with its symlinks
// resolved, following at most maxSymlinks of them for each element.
func (d *dirResolver) dir(p string) string {
	if resolved, ok := d.dirs[p]; ok {
		return resolved
	}
	d.dirs[p] = p

	parent, name := path.Split(p)
	resolved := path.Join(d.dir(path.Clean(parent)), name)
	for range maxSymlinks {
		target, err := d.root.Readlink(strings.TrimPrefix(resolved, "/"))
		if err != nil {
			break
		}
		resolved = d.follow(path.Dir(resolved), target)
	}

	d.dirs[p] = resolved
	return resolved
}

// follow resolves the target of a symlink in dir one element at a time, as
// the kernel does, so that .. applies to where the symlinks before it lead.
func (d *dirResolver) follow(dir, target string) string {
	if path.IsAbs(target) {
		dir = "/"
	}
	for elem := range strings.SplitSeq(target, "/") {
		switch elem {
		case "", ".":
		case "..":
			dir = path.Dir(dir)
		default:
			dir = d.dir(path.Join(dir, elem))
		}
	}
	return dir
}
