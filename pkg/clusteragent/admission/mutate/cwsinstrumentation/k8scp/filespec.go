// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8scp

import (
	"path"
	"path/filepath"
	"strings"
)

// FileSpec is used to identify a local or remote file
type FileSpec struct {
	PodName      string
	PodNamespace string
	File         pathSpec
}

type pathSpec interface {
	String() string
}

// localPath represents a client-native path, which will differ based
// on the client OS, its methods will use path/filepath package which
// is OS dependant
type localPath struct {
	file string
}

func newLocalPath(fileName string) localPath {
	file := stripTrailingSlash(fileName)
	return localPath{file: file}
}

func (p localPath) String() string {
	return p.file
}

// Dir returns the directory of the localPath
func (p localPath) Dir() localPath {
	return newLocalPath(filepath.Dir(p.file))
}

// Base returns the base of the localPath
func (p localPath) Base() localPath {
	return newLocalPath(filepath.Base(p.file))
}

// Clean returns a new cleaned localPath
func (p localPath) Clean() localPath {
	return newLocalPath(filepath.Clean(p.file))
}

// Join joins the input path with the current localPath
func (p localPath) Join(elem pathSpec) localPath {
	return newLocalPath(filepath.Join(p.file, elem.String()))
}

// Glob returns the list of files matching the input file name
func (p localPath) Glob() (matches []string, err error) {
	return filepath.Glob(p.file)
}

// StripSlashes strips leading slashes
func (p localPath) StripSlashes() localPath {
	return newLocalPath(stripLeadingSlash(p.file))
}

// remotePath represents always UNIX path, its methods will use path
// package which is always using `/`
type remotePath struct {
	file string
}

func newRemotePath(fileName string) remotePath {
	// we assume remote file is a linux container but we need to convert
	// windows path separators to unix style for consistent processing
	file := strings.ReplaceAll(stripTrailingSlash(fileName), `\`, "/")
	return remotePath{file: file}
}

func (p remotePath) String() string {
	return p.file
}

// Dir returns the directory of the remotePath
func (p remotePath) Dir() remotePath {
	return newRemotePath(path.Dir(p.file))
}

// Base returns the base of the remotePath
func (p remotePath) Base() remotePath {
	return newRemotePath(path.Base(p.file))
}

// Clean returns a new cleaned remotePath
func (p remotePath) Clean() remotePath {
	return newRemotePath(path.Clean(p.file))
}

// Join joins the input path with the current remotePath
func (p remotePath) Join(elem pathSpec) remotePath {
	return newRemotePath(path.Join(p.file, elem.String()))
}

// StripShortcuts removes any leading or trailing `../` in the path
func (p remotePath) StripShortcuts() remotePath {
	p = p.Clean()
	return newRemotePath(stripPathShortcuts(p.file))
}

// StripSlashes strips leading slashes
func (p remotePath) StripSlashes() remotePath {
	return newRemotePath(stripLeadingSlash(p.file))
}

// strips trailing slash (if any) both unix and windows style
func stripTrailingSlash(file string) string {
	if len(file) == 0 {
		return file
	}
	if file != "/" && strings.HasSuffix(string(file[len(file)-1]), "/") {
		return file[:len(file)-1]
	}
	return file
}

func stripLeadingSlash(file string) string {
	// tar strips the leading '/' and '\' if it's there, so we will too
	return strings.TrimLeft(file, `/\`)
}

// stripPathShortcuts removes any leading or trailing "../" from a given path
func stripPathShortcuts(p string) string {
	newPath := p
	trimmed := strings.TrimPrefix(newPath, "../")

	for trimmed != newPath {
		newPath = trimmed
		trimmed = strings.TrimPrefix(newPath, "../")
	}

	// trim leftover {".", ".."}
	if newPath == "." || newPath == ".." {
		newPath = ""
	}

	if len(newPath) > 0 && string(newPath[0]) == "/" {
		return newPath[1:]
	}

	return newPath
}
