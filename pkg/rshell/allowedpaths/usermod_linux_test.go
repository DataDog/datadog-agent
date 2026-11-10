// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package allowedpaths

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const usermodPasswd = "root:x:0:0:root:/root:/bin/sh\ndd-agent:x:123:123:Agent:/var/lib/dd-agent:/bin/false\n"
const usermodGroup = "other:x:124:alice,dd-agent\ndocker:x:998:alice\n"
const usermodShadow = "other:!:admin:alice,dd-agent\ndocker:!secret:admin:alice\n"

func usermodFixture(t *testing.T) (string, *Sandbox) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	for name, data := range map[string]string{"passwd": usermodPasswd, "group": usermodGroup, "gshadow": usermodShadow} {
		mode := os.FileMode(0o644)
		if name == "gshadow" {
			mode = 0o640
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), mode))
	}
	sandbox, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	sandbox.SetWritable()
	t.Cleanup(func() { require.NoError(t, sandbox.Close()) })
	return dir, sandbox
}

func TestUsermodAppendAndIdempotence(t *testing.T) {
	dir, sandbox := usermodFixture(t)
	// Preserve a supplementary GID too, without requiring root or changing host groups.
	groups, err := os.Getgroups()
	require.NoError(t, err)
	for _, gid := range groups {
		if gid != os.Getgid() {
			require.NoError(t, os.Chown(filepath.Join(dir, "gshadow"), -1, gid))
			break
		}
	}
	before := map[string]os.FileInfo{}
	for _, name := range []string{"passwd", "group", "gshadow"} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		before[name] = info
	}
	require.NoError(t, sandbox.addDDAgentToDocker(context.Background(), dir))
	for name, original := range map[string]string{"passwd": usermodPasswd, "group": usermodGroup, "gshadow": usermodShadow} {
		expected := original
		if name != "passwd" {
			expected = strings.TrimSuffix(original, "\n") + ",dd-agent\n"
		}
		actual, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, expected, string(actual))
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		require.True(t, os.SameFile(before[name], info))
		require.Equal(t, before[name].Mode(), info.Mode())
		old, now := before[name].Sys().(*syscall.Stat_t), info.Sys().(*syscall.Stat_t)
		require.Equal(t, old.Uid, now.Uid)
		require.Equal(t, old.Gid, now.Gid)
	}
	backups, err := filepath.Glob(filepath.Join(dir, "*.rshell-usermod-*"))
	require.NoError(t, err)
	require.Len(t, backups, 2)
	for _, path := range backups {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Zero(t, info.Mode().Perm()&0o077)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		if strings.HasPrefix(filepath.Base(path), "gshadow.") {
			require.Equal(t, usermodShadow, string(data))
		} else {
			require.Equal(t, usermodGroup, string(data))
		}
	}
	require.NoError(t, sandbox.addDDAgentToDocker(context.Background(), dir))
	after, err := filepath.Glob(filepath.Join(dir, "*.rshell-usermod-*"))
	require.NoError(t, err)
	require.Equal(t, backups, after)
}

func TestUsermodDivergenceAndMissingShadow(t *testing.T) {
	for _, name := range []string{"group", "gshadow", "absent"} {
		t.Run(name, func(t *testing.T) {
			dir, sandbox := usermodFixture(t)
			if name == "absent" {
				require.NoError(t, os.Remove(filepath.Join(dir, "gshadow")))
			} else {
				data, err := os.ReadFile(filepath.Join(dir, name))
				require.NoError(t, err)
				updated, err := appendDockerMember(data, name == "gshadow")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), updated, 0o600))
			}
			require.NoError(t, sandbox.addDDAgentToDocker(context.Background(), dir))
			for _, target := range []string{"group", "gshadow"} {
				if name == "absent" && target == "gshadow" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, target))
				require.NoError(t, err)
				require.Contains(t, string(data), "alice,dd-agent\n")
			}
		})
	}
}

func TestUsermodRejectsBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"missing-user", "missing-group", "missing-shadow-entry", "duplicate", "readonly", "denied", "symlink", "directory", "fifo", "hardlink", "canceled", "editor-lock", "in-process-busy"} {
		t.Run(scenario, func(t *testing.T) {
			dir, sandbox := usermodFixture(t)
			ctx := context.Background()
			groupPath := filepath.Join(dir, "group")
			switch scenario {
			case "missing-user":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "passwd"), []byte("root:x:0:0::/:/bin/sh\n"), 0o644))
			case "missing-group":
				require.NoError(t, os.WriteFile(groupPath, []byte("other:x:124:alice\n"), 0o644))
			case "missing-shadow-entry":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "gshadow"), []byte("other:!::alice\n"), 0o640))
			case "duplicate":
				require.NoError(t, os.WriteFile(groupPath, []byte(usermodGroup+"docker:x:999:\n"), 0o644))
			case "readonly":
				sandbox.readOnly = true
			case "denied":
				sandbox.roots[0].mode = pathModeReadOnly
			case "symlink", "directory", "fifo":
				require.NoError(t, os.Rename(groupPath, filepath.Join(dir, "original")))
				if scenario == "symlink" {
					require.NoError(t, os.Symlink("original", groupPath))
				}
				if scenario == "directory" {
					require.NoError(t, os.Mkdir(groupPath, 0o700))
				}
				if scenario == "fifo" {
					require.NoError(t, unix.Mkfifo(groupPath, 0o600))
				}
			case "hardlink":
				require.NoError(t, os.Link(groupPath, filepath.Join(dir, "alias")))
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "editor-lock":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "group.lock"), []byte("123\x00"), 0o600))
			case "in-process-busy":
				usermodMu.Lock()
				defer usermodMu.Unlock()
			}
			originalShadow, err := os.ReadFile(filepath.Join(dir, "gshadow"))
			require.NoError(t, err)
			require.Error(t, sandbox.addDDAgentToDocker(ctx, dir))
			shadow, err := os.ReadFile(filepath.Join(dir, "gshadow"))
			require.NoError(t, err)
			require.Equal(t, originalShadow, shadow)
			copies, err := filepath.Glob(filepath.Join(dir, "*.rshell-usermod-*"))
			require.NoError(t, err)
			require.Empty(t, copies)
		})
	}
}

func TestUsermodDetectsConcurrentContentAndIdentity(t *testing.T) {
	dir, sandbox := usermodFixture(t)
	path := filepath.Join(dir, "group")
	f, err := sandbox.openAccountFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)
	snapshot := &accountSnapshot{file: f, path: path, info: info, before: []byte(usermodGroup)}
	require.NoError(t, sandbox.checkAccountSnapshot(snapshot))
	require.NoError(t, os.WriteFile(path, []byte(usermodGroup+"new:x:900:someone\n"), 0o644))
	require.ErrorContains(t, sandbox.checkAccountSnapshot(snapshot), "contents changed")
	require.NoError(t, os.Rename(path, path+".old"))
	require.NoError(t, os.WriteFile(path, []byte(usermodGroup), 0o644))
	require.ErrorContains(t, sandbox.checkAccountSnapshot(snapshot), "identity")
}

func TestUsermodWriteFailure(t *testing.T) {
	dir, sandbox := usermodFixture(t)
	f, err := sandbox.openAccountFile(filepath.Join(dir, "group"), os.O_RDONLY, 0)
	require.NoError(t, err)
	defer f.Close()
	mutated, err := writeChunkedCancellable(context.Background(), f, []byte("must not succeed"))
	require.Error(t, err)
	require.False(t, mutated)
	data, err := os.ReadFile(filepath.Join(dir, "group"))
	require.NoError(t, err)
	require.Equal(t, usermodGroup, string(data))
	if os.Geteuid() != 0 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pwd.lock"), nil, 0o600))
		require.NoError(t, os.Chmod(dir, 0o500))
		defer os.Chmod(dir, 0o700)
		require.ErrorContains(t, sandbox.addDDAgentToDocker(context.Background(), dir), "recovery copy")
		data, err = os.ReadFile(filepath.Join(dir, "gshadow"))
		require.NoError(t, err)
		require.Equal(t, usermodShadow, string(data))
	}
}

func TestUsermodExternalLock(t *testing.T) {
	if dir := os.Getenv("RSHELL_USERMOD_LOCK_FIXTURE"); dir != "" {
		if !strings.HasPrefix(dir, os.TempDir()+string(os.PathSeparator)) {
			os.Exit(2)
		}
		f, err := os.OpenFile(filepath.Join(dir, ".pwd.lock"), os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			os.Exit(3)
		}
		lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: int16(io.SeekStart)}
		if unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lock) != nil {
			os.Exit(4)
		}
		_, _ = os.Stdout.Write([]byte("locked\n"))
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = f.Close()
		os.Exit(0)
	}
	dir, sandbox := usermodFixture(t)
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(binary, "-test.run=^TestUsermodExternalLock$")
	cmd.Env = append(os.Environ(), "RSHELL_USERMOD_LOCK_FIXTURE="+dir)
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() { _ = input.Close(); _ = cmd.Wait() }()
	ready := make([]byte, 7)
	_, err = io.ReadFull(output, ready)
	require.NoError(t, err)
	require.Equal(t, "locked\n", string(ready))
	require.ErrorContains(t, sandbox.addDDAgentToDocker(context.Background(), dir), "cannot be locked")
	require.NoError(t, input.Close())
	require.NoError(t, cmd.Wait())
	require.NoError(t, sandbox.addDDAgentToDocker(context.Background(), dir))
}

// A process-local file-size limit injects a real partial write after both
// recovery copies have completed, without touching any host identity file.
func TestUsermodPartialWriteRetainsRecovery(t *testing.T) {
	if dir := os.Getenv("RSHELL_USERMOD_FAILURE_FIXTURE"); dir != "" {
		if !strings.HasPrefix(dir, os.TempDir()+string(os.PathSeparator)) {
			os.Exit(2)
		}
		sandbox, _, err := New([]string{dir + ":rw"})
		if err != nil {
			t.Fatal(err)
		}
		sandbox.SetWritable()
		signal.Ignore(syscall.SIGXFSZ)
		limit := uint64(max(len(usermodGroup), len(usermodShadow)))
		if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: limit, Max: limit}); err != nil {
			t.Fatal(err)
		}
		err = sandbox.addDDAgentToDocker(context.Background(), dir)
		if err == nil || !strings.Contains(err.Error(), "may be partially updated") || !strings.Contains(err.Error(), "recovery copy") {
			t.Fatalf("unexpected write outcome: %v", err)
		}
		os.Exit(0)
	}
	dir, _ := usermodFixture(t)
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(binary, "-test.run=^TestUsermodPartialWriteRetainsRecovery$")
	cmd.Env = append(os.Environ(), "RSHELL_USERMOD_FAILURE_FIXTURE="+dir)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	group, err := os.ReadFile(filepath.Join(dir, "group"))
	require.NoError(t, err)
	require.Equal(t, usermodGroup, string(group))
	backups, err := filepath.Glob(filepath.Join(dir, "gshadow.rshell-usermod-*"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	backup, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	require.Equal(t, usermodShadow, string(backup))
	shadow, err := os.ReadFile(filepath.Join(dir, "gshadow"))
	require.NoError(t, err)
	require.NotEqual(t, backup, shadow)
}
