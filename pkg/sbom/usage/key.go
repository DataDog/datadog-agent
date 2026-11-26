// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usage joins the files that processes run to the components of an
// SBOM. system-probe reports the keys of the files that ran in a workload, and
// the core agent keeps beside each SBOM an index from the keys of the files
// that tell a component apart to that component.
package usage

import (
	"path"
	"slices"
	"strings"

	"github.com/twmb/murmur3"
)

// Key returns the key of the file at p, a path as Clean returns it.
func Key(p string) uint64 {
	return murmur3.StringSum64(p)
}

// Clean returns p as an absolute path in its shortest form.
func Clean(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// AppendKeys appends to dst the keys through which a use of the file at p, a
// path as Clean returns it, credits a component, and returns the extended
// slice. A file of a Node.js package credits it through its package.json, and a
// file of a Ruby gem through its gemspec, as the index names them. Python
// bytecode credits its source too. Callers strip the " (deleted)" suffix procfs
// gives a replaced file first.
func AppendKeys(dst []uint64, p string) []uint64 {
	if dir, ok := nodePackage(p); ok {
		return append(dst, Key(dir+"/package.json"))
	}
	if spec, ok := gemSpec(p); ok {
		return append(dst, Key(spec))
	}
	dst = append(dst, Key(p))
	if key, ok := SourceKey(p); ok {
		dst = append(dst, key)
	}
	return dst
}

// nodePackage returns the directory of the Node.js package that holds the file
// at p: the directory under the last node_modules of p, with its scope.
func nodePackage(p string) (string, bool) {
	const marker = "/node_modules/"
	i := strings.LastIndex(p, marker)
	if i < 0 {
		return "", false
	}
	rest := p[i+len(marker):]
	name, file, ok := strings.Cut(rest, "/")
	if ok && strings.HasPrefix(name, "@") {
		var scoped string
		scoped, file, ok = strings.Cut(file, "/")
		name += "/" + scoped
	}
	if !ok || file == "" || strings.HasPrefix(name, ".") {
		return "", false
	}
	return p[:i+len(marker)] + name, true
}

// gemSpec returns the gemspec of the Ruby gem that holds the file at p, in
// gems/<name>-<version> or extensions/<platform>/<abi>/<name>-<version> under
// the home of the gem. The home can hold gems in its own path, as
// /usr/lib/ruby/gems/3.2.0 does, so each gems directory of p counts, from the
// last one.
func gemSpec(p string) (string, bool) {
	for end := len(p); ; {
		i := strings.LastIndex(p[:end], "/gems/")
		if i < 0 {
			break
		}
		if gem, file, ok := strings.Cut(p[i+len("/gems/"):], "/"); ok && file != "" && strings.Contains(gem, "-") {
			return p[:i] + "/specifications/" + gem + ".gemspec", true
		}
		end = i
	}
	if i := strings.LastIndex(p, "/extensions/"); i >= 0 {
		parts := strings.SplitN(p[i+len("/extensions/"):], "/", 4)
		if len(parts) == 4 && parts[3] != "" && strings.Contains(parts[2], "-") {
			return p[:i] + "/specifications/" + parts[2] + ".gemspec", true
		}
	}
	return "", false
}

// SourceKey returns the key of the source of the Python bytecode at p, a path
// as Clean returns it, and whether p is such bytecode. A run of bytecode
// credits its source whichever process compiled it.
func SourceKey(p string) (uint64, bool) {
	source, ok := pycSource(p)
	if !ok {
		return 0, false
	}
	return Key(source), true
}

// pycSource returns the source dir/name.py of the bytecode Python caches as
// dir/__pycache__/name.cpython-312.pyc, if p is such bytecode.
func pycSource(p string) (string, bool) {
	dir, name := path.Split(p)
	if !strings.HasSuffix(name, ".pyc") || path.Base(dir) != "__pycache__" {
		return "", false
	}
	module, _, _ := strings.Cut(name, ".")
	return path.Join(path.Dir(path.Clean(dir)), module+".py"), true
}

// mergedDirs are the directories of the root that a usr-merged layout links
// into /usr.
var mergedDirs = []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32"}

// pathAlias returns p under the other prefix of a usr-merged layout, where /bin
// and /usr/bin are one tree, or "" for other paths.
func pathAlias(p string) string {
	if after, ok := strings.CutPrefix(p, "/usr"); ok && inMergedDir(after) {
		return after
	}
	if inMergedDir(p) {
		return "/usr" + p
	}
	return ""
}

// inMergedDir reports whether p lies under one of mergedDirs.
func inMergedDir(p string) bool {
	return slices.ContainsFunc(mergedDirs, func(dir string) bool {
		rest, ok := strings.CutPrefix(p, dir)
		return ok && strings.HasPrefix(rest, "/")
	})
}

// docDirs hold the documentation and translations of packages, which processes
// read as data.
var docDirs = []string{"/usr/share/doc/", "/usr/share/info/", "/usr/share/licenses/", "/usr/share/locale/", "/usr/share/man/"}

// isDoc reports whether p is documentation, which tells no use of its package.
func isDoc(p string) bool {
	for _, dir := range docDirs {
		if strings.HasPrefix(p, dir) {
			return true
		}
	}
	return false
}
