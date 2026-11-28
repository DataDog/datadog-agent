// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package inuse records the files that processes run in each workload, the
// containers of an image and the host, for the runtime usage reports that
// system-probe sends the core agent.
package inuse

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// DeletedSuffix ends the path that procfs gives a file replaced under a
// running process.
const DeletedSuffix = " (deleted)"

// codeSuffixes are the suffixes of the files that loaders and interpreters
// run, beside executables and shared libraries.
var codeSuffixes = []string{
	".py", ".pyc", ".pyo", ".pl", ".pm", ".rb", ".js", ".mjs", ".cjs", ".node",
	".jar", ".war", ".ear", ".egg", ".class", ".lua", ".php", ".phar", ".tcl", ".sh", ".beam", ".dll",
	".wasm", ".ko", ".ko.gz", ".ko.xz", ".ko.zst",
}

// sharedLibrary matches the names of shared libraries: libfoo.so, and with a
// version, libfoo.so.1 or libssl.so.1.0.2k.
var sharedLibrary = regexp.MustCompile(`\.so(\.[0-9][0-9A-Za-z_]*)*$`)

// HoldsCode reports whether file can hold code that a process runs: an
// executable, a shared library, a kernel module or a module of an interpreter.
// Files under /etc hold configuration, the shell snippets of profile.d that
// every login sources included.
func HoldsCode(file *model.FileEvent) bool {
	if strings.HasPrefix(file.PathnameStr, "/etc/") {
		return false
	}
	return file.Mode&0111 != 0 || CodeName(path.Base(file.PathnameStr))
}

// CodeName reports whether name is that of a shared library or ends with one
// of the suffixes of code files.
func CodeName(name string) bool {
	if sharedLibrary.MatchString(name) {
		return true
	}
	return slices.ContainsFunc(codeSuffixes, func(suffix string) bool {
		return strings.HasSuffix(name, suffix)
	})
}

// codeReaders are the names of the processes that read code without running
// it: the indexers of shared libraries and kernel modules, the builders of
// initramfs images and archives, the verifiers and scanners of files, the
// tools that hash or inspect binaries, and the package managers of languages,
// which read the packages they install. The kernel cuts a process name to 15
// bytes.
var codeReaders = []string{
	"ldconfig", "ldconfig.real", "depmod", "modinfo", "weak-modules",
	"dracut-install", "cpio", "cp", "install", "tar", "rsync",
	"rpmverify", "debsums", "aide", "fapolicyd", "clamscan", "clamd",
	"wazuh-syscheckd", "ossec-syscheckd", "oscap", "oscap-io",
	"md5sum", "sha1sum", "sha224sum", "sha256sum", "sha384sum", "sha512sum",
	"b2sum", "cksum", "file", "strings", "readelf", "objdump", "nm", "gdb",
	"perf", "systemd-coredum",
	"pip", "pip3", "uv", "poetry", "pipenv", "gem", "npm", "yarn", "pnpm", "composer",
}

// bytecodeCompilers are the names of the processes that compile the Python
// modules of a package as it installs, in children that read its sources.
var bytecodeCompilers = []string{"py3compile", "pycompile", "pypy3compile"}

// loaderNames are the prefixes of the names of the dynamic loader run as a
// program, as ldd runs it to list the libraries of a binary.
var loaderNames = []string{"ld-linux", "ld-musl", "ld64.so"}

// ReadsCode reports whether the process of pc reads code without running it.
// A process name may carry a version, as pip3.11 does.
func ReadsCode(pc *model.ProcessContext) bool {
	if reads(pc.Comm) {
		return true
	}
	if pc.Parent != nil && (slices.Contains(bytecodeCompilers, pc.Parent.Comm) || reads(pc.Parent.Comm)) {
		return true
	}
	return slices.ContainsFunc(loaderNames, func(prefix string) bool {
		return strings.HasPrefix(pc.Comm, prefix)
	})
}

// reads reports whether a process named comm reads code without running it.
func reads(comm string) bool {
	name := strings.TrimRight(comm, "0123456789.")
	return slices.Contains(codeReaders, comm) || slices.Contains(codeReaders, name) ||
		slices.Contains(bytecodeCompilers, comm)
}

// IsDir reports whether mode, the mode of the file of an event, is that of a
// directory.
func IsDir(mode uint16) bool {
	return mode&syscall.S_IFMT == syscall.S_IFDIR
}

// RunsAsRoot reports whether a process runs as root, by its real or effective
// UID, as a setuid-root binary run by another user does.
func RunsAsRoot(pc *model.ProcessContext) bool {
	return pc.UID == 0 || pc.EUID == 0
}

// Runs reports whether the process of pc runs file: its executable, or the
// interpreter of its script.
func Runs(pc *model.ProcessContext, file *model.FileEvent) bool {
	return file.PathnameStr == pc.Process.FileEvent.PathnameStr ||
		pc.Process.HasInterpreter() && file.PathnameStr == pc.Process.LinuxBinprm.FileEvent.PathnameStr
}

// IsPlaceholder reports whether pc is the placeholder the probe gives the
// event of a process it could not resolve. The placeholder holds the container
// of the cgroup the kernel reports, with zero credentials and an empty name,
// so the gates on the name and the root flag would misread it.
func IsPlaceholder(pc *model.ProcessContext) bool {
	return pc.Source == model.ProcessCacheEntryFromPlaceholder
}
