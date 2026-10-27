// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptPath(t *testing.T) {
	tests := []struct {
		name string
		comm string
		exe  string
		argv []string
		want string
	}{
		// The kernel runs the interpreter of the shebang.
		{"shebang", "daemon.py", "/usr/bin/python3.12", []string{"/usr/bin/python3", "/opt/app/daemon.py", "--flag"}, "/opt/app/daemon.py"},
		{"shebang argument", "firewalld", "/usr/bin/python3.12", []string{"/usr/bin/python3", "-s", "/usr/sbin/firewalld", "--nofork", "--nopid"}, "/usr/sbin/firewalld"},
		{"cut name", "networkd-dispat", "/usr/bin/python3.10", []string{"/usr/bin/python3", "/usr/bin/networkd-dispatcher", "--run-startup-triggers"}, "/usr/bin/networkd-dispatcher"},
		{"shebang argument and cut name", "sbom-script-dae", "/usr/bin/dash", []string{"/bin/sh", "-e", "/usr/bin/sbom-script-daemon"}, "/usr/bin/sbom-script-daemon"},
		{"prefix shorter than the cut", "sbom-script-da", "/usr/bin/dash", []string{"/bin/sh", "-e", "/usr/bin/sbom-script-daemon"}, ""},
		{"replaced interpreter", "firewalld", "/usr/bin/python3.12 (deleted)", []string{"/usr/bin/python3", "-s", "/usr/sbin/firewalld"}, "/usr/sbin/firewalld"},
		{"busybox shebang", "app.sh", "/bin/busybox", []string{"/bin/sh", "/usr/local/bin/app.sh"}, "/usr/local/bin/app.sh"},
		{"relative path", "daemon.py", "/usr/bin/python3.12", []string{"/usr/bin/python3", "-s", "./daemon.py", "--flag"}, ""},
		{"relative namesake", "worker", "/usr/bin/dash", []string{"/bin/sh", "./worker", "/var/lib/worker"}, ""},
		{"dot dot", "daemon.py", "/usr/bin/python3.12", []string{"/usr/bin/python3", "/opt/app/../app/daemon.py"}, ""},
		{"double slash", "daemon.py", "/usr/bin/python3.12", []string{"/usr/bin/python3", "//opt/app/daemon.py"}, ""},
		{"file descriptor", "3", "/usr/bin/dash", []string{"/bin/sh", "/dev/fd/3"}, ""},
		{"proc", "3", "/usr/bin/dash", []string{"/bin/sh", "/proc/self/fd/3"}, ""},
		{"relative interpreter", "daemon.py", "/usr/bin/python3.12", []string{"python3", "/opt/app/daemon.py"}, ""},
		{"past the shebang argument", "daemon.py", "/usr/bin/python3.12", []string{"/usr/bin/python3", "-s", "-E", "/opt/app/daemon.py"}, ""},
		{"renamed process", "worker", "/usr/bin/python3.12", []string{"/usr/bin/python3", "/opt/app/main.py"}, ""},
		{"empty comm", "", "/usr/bin/python3.12", []string{"/usr/bin/python3", "/opt/app/daemon.py"}, ""},
		{"no argument", "daemon.py", "/usr/bin/python3.12", []string{"/usr/bin/python3"}, ""},
		{"no argv", "daemon.py", "/usr/bin/python3.12", nil, ""},

		// A native program, or a program outside the interpreters.
		{"native", "file", "/usr/bin/file", []string{"file", "/usr/bin/file"}, ""},
		{"replaced native", "file", "/usr/bin/file (deleted)", []string{"file", "/usr/bin/file"}, ""},
		{"cut native name", "containerd-shim", "/usr/bin/containerd-shim-runc-v2", []string{"/usr/bin/containerd-shim-runc-v2", "/usr/bin/containerd-shim"}, ""},
		{"extended native name", "systemd-journald", "/usr/lib/systemd/systemd-journald", []string{"/usr/lib/systemd/systemd-journald", "/usr/lib/systemd/systemd-journald"}, ""},
		{"file argument", "vim", "/usr/bin/vim.basic", []string{"vim", "/etc/passwd"}, ""},
		{"unknown interpreter", "java", "/usr/lib/jvm/java-17-openjdk-amd64/bin/java", []string{"java", "-jar", "/opt/app/app.jar"}, ""},
		{"busybox applet", "tail", "/bin/busybox", []string{"tail", "-f", "/dev/null"}, ""},

		// An interpreter run by name, given its script as an operand.
		{"waagent", "python3", "/usr/bin/python3.10", []string{"/usr/bin/python3", "-u", "/usr/sbin/waagent", "-daemon"}, "/usr/sbin/waagent"},
		{"env shebang", "python3", "/usr/bin/python3.12", []string{"python3", "/opt/app/daemon.py"}, "/opt/app/daemon.py"},
		{"python versioned", "python3.12", "/usr/bin/python3.12", []string{"/usr/bin/python3.12", "/usr/bin/x"}, "/usr/bin/x"},
		{"python -c", "python3", "/usr/bin/python3.12", []string{"python3", "-c", "import os", "/etc/passwd"}, ""},
		{"python -m", "python3", "/usr/bin/python3.12", []string{"python3", "-m", "http.server", "/srv"}, ""},
		{"python -W ignore", "python3", "/usr/bin/python3.12", []string{"python3", "-W", "ignore", "/usr/bin/x"}, "/usr/bin/x"},
		{"python -Werror", "python3", "/usr/bin/python3.12", []string{"python3", "-Werror", "/usr/bin/x"}, "/usr/bin/x"},
		{"python -uB", "python3", "/usr/bin/python3.12", []string{"python3", "-uB", "/usr/bin/x"}, "/usr/bin/x"},
		{"python -Bc", "python3", "/usr/bin/python3.12", []string{"python3", "-Bc", "pass"}, ""},
		{"python --check-hash-based-pycs", "python3", "/usr/bin/python3.12", []string{"python3", "--check-hash-based-pycs", "always", "/usr/bin/x"}, "/usr/bin/x"},
		{"python --", "python3", "/usr/bin/python3.12", []string{"python3", "--", "/usr/bin/x"}, "/usr/bin/x"},
		{"python -", "python3", "/usr/bin/python3.12", []string{"python3", "-", "/usr/bin/x"}, ""},
		{"python relative", "python3", "/usr/bin/python3.12", []string{"python3", "-u", "app.py"}, ""},
		{"platform-python", "platform-python", "/usr/libexec/platform-python3.6", []string{"/usr/libexec/platform-python", "-s", "/usr/bin/x"}, "/usr/bin/x"},
		{"sh -e", "sh", "/usr/bin/dash", []string{"/bin/sh", "-e", "/usr/bin/x"}, "/usr/bin/x"},
		{"sh -c", "sh", "/usr/bin/dash", []string{"/bin/sh", "-c", "/usr/bin/x arg"}, ""},
		{"sh -s", "sh", "/usr/bin/dash", []string{"/bin/sh", "-s"}, ""},
		{"sh -", "sh", "/usr/bin/dash", []string{"/bin/sh", "-", "/usr/bin/x"}, "/usr/bin/x"},
		{"bash -o pipefail", "bash", "/usr/bin/bash", []string{"/bin/bash", "-o", "pipefail", "/usr/bin/x"}, "/usr/bin/x"},
		{"bash -eo pipefail", "bash", "/usr/bin/bash", []string{"/bin/bash", "-eo", "pipefail", "/usr/bin/x"}, "/usr/bin/x"},
		{"bash +o history", "bash", "/usr/bin/bash", []string{"/bin/bash", "+o", "history", "/usr/bin/x"}, "/usr/bin/x"},
		{"bash --rcfile", "bash", "/usr/bin/bash", []string{"/bin/bash", "--rcfile", "/etc/bash.bashrc", "/usr/bin/x"}, "/usr/bin/x"},
		{"bash -noprofile", "bash", "/usr/bin/bash", []string{"/bin/bash", "-noprofile", "/usr/bin/x", "/etc/y"}, "/usr/bin/x"},
		{"bash -login", "bash", "/usr/bin/bash", []string{"/bin/bash", "-login", "/usr/bin/x", "/etc/y"}, "/usr/bin/x"},
		{"bash -rcfile", "bash", "/usr/bin/bash", []string{"/bin/bash", "-rcfile", "/etc/bash.bashrc", "/usr/bin/x"}, "/usr/bin/x"},
		{"bash process substitution", "bash", "/usr/bin/bash", []string{"bash", "/dev/fd/63"}, ""},
		{"script named like an option", "sh", "/usr/bin/dash", []string{"/bin/sh", "o", "/usr/bin/x"}, ""},
		{"zsh -oerrexit", "zsh", "/usr/bin/zsh", []string{"/bin/zsh", "-oerrexit", "/usr/bin/x", "/etc/y"}, "/usr/bin/x"},
		{"zsh -o errexit", "zsh", "/usr/bin/zsh", []string{"/bin/zsh", "-o", "errexit", "/usr/bin/x"}, "/usr/bin/x"},
		{"ksh -xoerrexit", "ksh", "/usr/bin/ksh93", []string{"/bin/ksh", "-xoerrexit", "/usr/bin/x", "/etc/y"}, "/usr/bin/x"},
		{"login shell", "bash", "/usr/bin/bash", []string{"-bash"}, ""},
		{"busybox sh", "sh", "/bin/busybox", []string{"/bin/sh", "/usr/bin/x"}, "/usr/bin/x"},
		{"perl -w", "perl", "/usr/bin/perl", []string{"/usr/bin/perl", "-w", "/usr/bin/x"}, "/usr/bin/x"},
		{"perl -ne", "perl", "/usr/bin/perl", []string{"perl", "-ne", "print", "/etc/passwd"}, ""},
		{"perl -lne", "perl", "/usr/bin/perl", []string{"perl", "-lne", "print", "/etc/passwd"}, ""},
		{"perl -I attached", "perl", "/usr/bin/perl", []string{"perl", "-I/usr/share/perl5", "/usr/bin/x"}, "/usr/bin/x"},
		{"perl -I", "perl", "/usr/bin/perl", []string{"perl", "-I", "/usr/share/perl5", "/usr/bin/x"}, "/usr/bin/x"},
		{"perl -0777", "perl", "/usr/bin/perl5.36.0", []string{"perl", "-0777", "/usr/bin/x"}, "/usr/bin/x"},
		{"ruby -I", "ruby", "/usr/bin/ruby3.0", []string{"ruby", "-I", "/usr/lib/ruby/vendor", "/usr/bin/x"}, "/usr/bin/x"},
		{"ruby -e", "ruby", "/usr/bin/ruby3.0", []string{"ruby", "-e", "puts 1"}, ""},
		{"ruby --parser", "ruby", "/usr/bin/ruby3.3", []string{"ruby", "--parser", "prism", "/usr/bin/x"}, "/usr/bin/x"},
		{"node --require", "node", "/usr/bin/node", []string{"node", "--require", "dotenv/config", "/usr/lib/app/server.js"}, "/usr/lib/app/server.js"},
		{"node --require=", "node", "/usr/bin/node", []string{"node", "--require=dotenv/config", "/usr/lib/app/server.js"}, "/usr/lib/app/server.js"},
		{"node --env-file", "node", "/usr/bin/node", []string{"node", "--env-file", "/etc/app.env", "/usr/lib/app/server.js"}, "/usr/lib/app/server.js"},
		{"node --test-reporter-destination", "node", "/usr/bin/node", []string{"node", "--test-reporter-destination", "/var/log/report", "/usr/lib/app/server.js"}, "/usr/lib/app/server.js"},
		{"node --experimental-policy", "node", "/usr/bin/node", []string{"node", "--experimental-policy", "/etc/app/policy.json", "/usr/lib/app/server.js"}, "/usr/lib/app/server.js"},
		{"node -e", "node", "/usr/bin/node", []string{"node", "-e", "1"}, ""},
		{"nodejs", "nodejs", "/usr/bin/nodejs", []string{"nodejs", "/usr/lib/app/server.js"}, "/usr/lib/app/server.js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ScriptPath(tt.comm, tt.exe, tt.argv))
		})
	}
}

// FuzzScriptPath checks that ScriptPath returns "" or one of the arguments
// after argv[0], in a form that can name a script.
func FuzzScriptPath(f *testing.F) {
	f.Add("python3", "/usr/bin/python3.10", "/usr/bin/python3\x00-u\x00/usr/sbin/waagent\x00-daemon")
	f.Add("sbom-script-dae", "/usr/bin/dash", "/bin/sh\x00-e\x00/usr/bin/sbom-script-daemon")
	f.Add("perl", "/usr/bin/perl", "perl\x00-lne\x00print")
	f.Fuzz(func(t *testing.T, comm, exe, args string) {
		argv := strings.Split(args, "\x00")
		script := ScriptPath(comm, exe, argv)
		if script == "" {
			return
		}
		if !slices.Contains(argv[1:], script) || scriptArg(script) != script {
			t.Errorf("ScriptPath(%q, %q, %q) = %q", comm, exe, argv, script)
		}
	})
}

// TestStatScript checks that the file of a script is the file its path
// resolves to through the root of the process, symlinks followed, as the exec
// of a script resolves it.
func TestStatScript(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	script := filepath.Join(dir, "daemon.py")
	require.NoError(t, os.WriteFile(script, []byte("import time\n"), 0o644))
	relative := filepath.Join(dir, "daemon")
	require.NoError(t, os.Symlink("daemon.py", relative))
	absolute := filepath.Join(dir, "daemon-abs")
	require.NoError(t, os.Symlink(script, absolute))
	pid := uint32(os.Getpid())

	fi, err := os.Stat(script)
	require.NoError(t, err)
	st := fi.Sys().(*syscall.Stat_t)
	for _, path := range []string{script, relative, absolute} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file := StatScript(pid, path)
			assert.Equal(t, script, file.PathnameStr)
			assert.True(t, file.IsPathnameStrResolved)
			assert.Equal(t, filepath.Base(script), file.BasenameStr)
			assert.True(t, file.IsBasenameStrResolved)
			assert.Equal(t, st.Ino, file.Inode)
			assert.Equal(t, uint16(st.Mode), file.Mode)
			assert.Equal(t, st.Uid, file.UID)
		})
	}

	t.Run("missing", func(t *testing.T) {
		missing := filepath.Join(dir, "missing.py")
		file := StatScript(pid, missing)
		assert.Equal(t, missing, file.PathnameStr)
		assert.Zero(t, file.FileFields)
	})

	t.Run("dangling", func(t *testing.T) {
		dangling := filepath.Join(dir, "dangling")
		require.NoError(t, os.Symlink("gone.py", dangling))
		file := StatScript(pid, dangling)
		assert.Equal(t, filepath.Join(dir, "gone.py"), file.PathnameStr)
		assert.Zero(t, file.FileFields)
	})
}
