// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sysctl

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnapshotEvent_ToJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   Snapshot
		want    string
		wantErr bool
	}{
		{
			name:  "empty_snapshot",
			input: NewSnapshot(),
			want:  "{\"sysctl\":{}}",
		},
		{
			name: "full_snapshot",
			input: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
				Sys: map[string]interface{}{
					"kernel": map[string]interface{}{
						"security": map[string]interface{}{
							"lockdown": []string{"none", "[integrity]", "confidentiality"},
						},
					},
				},
			},
			want: "{\"sysctl\":{\"proc\":{\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"2\"}}}},\"sys\":{\"kernel\":{\"security\":{\"lockdown\":[\"none\",\"[integrity]\",\"confidentiality\"]}}}}}",
		},
		{
			name: "proc_only",
			input: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			want: "{\"sysctl\":{\"proc\":{\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"2\"}}}}}}",
		},
		{
			name: "sys_only",
			input: Snapshot{
				Sys: map[string]interface{}{
					"kernel": map[string]interface{}{
						"security": map[string]interface{}{
							"lockdown": []string{"none", "[integrity]", "confidentiality"},
						},
					},
				},
			},
			want: "{\"sysctl\":{\"sys\":{\"kernel\":{\"security\":{\"lockdown\":[\"none\",\"[integrity]\",\"confidentiality\"]}}}}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SnapshotEvent{
				Sysctl: tt.input,
			}
			got, err := s.ToJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.JSONEqf(t, tt.want, string(got), "ToJSON() error")
		})
	}
}

func TestReadSnapshotSystemControl(t *testing.T) {
	t.Run("falls back when the primary path is unavailable", func(t *testing.T) {
		root := t.TempDir()
		primaryPath := filepath.Join(root, "sys", "kernel", "security", "lockdown")
		fallbackPath := filepath.Join(root, "root", "sys", "kernel", "security", "lockdown")
		assert.NoError(t, os.MkdirAll(filepath.Dir(fallbackPath), 0755))
		assert.NoError(t, os.WriteFile(fallbackPath, []byte("integrity\n"), 0644))

		value, err := readSnapshotSystemControl(primaryPath, fallbackPath, nil)
		assert.NoError(t, err)
		assert.Equal(t, "integrity\n", string(value))
	})

	t.Run("returns a sentinel error when both paths are unavailable", func(t *testing.T) {
		root := t.TempDir()
		_, err := readSnapshotSystemControl(filepath.Join(root, "sys", "lockdown"), filepath.Join(root, "root", "sys", "lockdown"), nil)
		assert.ErrorIs(t, err, ErrRequiredSysctlSnapshotFileNotFound)
	})
}

func TestSnapshotProcSys(t *testing.T) {
	t.Run("skips the entries vanishing during the walk", func(t *testing.T) {
		procRoot := t.TempDir()
		root := filepath.Join(procRoot, "sys")
		for _, file := range []string{
			"kernel/yama/ptrace_scope",
			"net/ipv4/conf/tun0/forwarding",
			"net/ipv4/conf/tun1/forwarding",
		} {
			file = filepath.Join(root, file)
			assert.NoError(t, os.MkdirAll(filepath.Dir(file), 0755))
			assert.NoError(t, os.WriteFile(file, []byte("1\n"), 0644))
		}

		// remove a directory and a file right before the walk reads them, like a network interface vanishing
		vanished := []string{
			filepath.Join(root, "net/ipv4/conf/tun0"),
			filepath.Join(root, "net/ipv4/conf/tun1/forwarding"),
		}

		s := NewSnapshot()
		walk := s.procSysWalkFunc(procRoot, root, nil)
		err := filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
			if err == nil && slices.Contains(vanished, file) {
				assert.NoError(t, os.RemoveAll(file))
			}
			return walk(file, d, err)
		})
		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"sys": map[string]interface{}{
				"kernel": map[string]interface{}{
					"yama": map[string]interface{}{
						"ptrace_scope": "1",
					},
				},
			},
		}, s.Proc)
	})

	t.Run("fails when /proc/sys is missing", func(t *testing.T) {
		procRoot := t.TempDir()
		root := filepath.Join(procRoot, "sys")

		s := NewSnapshot()
		err := filepath.WalkDir(root, s.procSysWalkFunc(procRoot, root, nil))
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestFindKernelConfigPath(t *testing.T) {
	const kernelVersion = "6.8.0-1067-aws"

	for name, configPaths := range map[string][]string{
		"finds the config of the boot directory":      {"boot/config-" + kernelVersion},
		"falls back to the proc config":               {"proc/config.gz"},
		"falls back to the boot directory of init":    {"proc/1/root/boot/config-" + kernelVersion},
		"falls back to the modules directory of init": {"proc/1/root/usr/lib/modules/" + kernelVersion + "/config"},
		"prefers the boot directory over the root of init": {
			"boot/config-" + kernelVersion,
			"proc/1/root/boot/config-" + kernelVersion,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range configPaths {
				file = filepath.Join(root, file)
				assert.NoError(t, os.MkdirAll(filepath.Dir(file), 0755))
				assert.NoError(t, os.WriteFile(file, nil, 0644))
			}

			configPath, err := findKernelConfigPath(filepath.Join(root, "boot"), filepath.Join(root, "proc"), kernelVersion)
			assert.NoError(t, err)
			assert.Equal(t, filepath.Join(root, configPaths[0]), configPath)
		})
	}

	t.Run("returns a sentinel error when no config is available", func(t *testing.T) {
		root := t.TempDir()
		_, err := findKernelConfigPath(filepath.Join(root, "boot"), filepath.Join(root, "proc"), kernelVersion)
		assert.ErrorIs(t, err, errKernelConfigNotFound)
	})
}

func TestSnapshot_InsertSnapshotEntry(t *testing.T) {
	type args struct {
		file  string
		value string
	}
	tests := []struct {
		name     string
		snapshot Snapshot
		args     args
		output   string
	}{
		{
			name:     "empty_snapshot_insert_nothing",
			snapshot: NewSnapshot(),
			args:     args{},
			output:   "{}",
		},
		{
			name:     "empty_snapshot_insert_string_single_file",
			snapshot: NewSnapshot(),
			args: args{
				file:  "sys",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":\"123\"}}",
		},
		{
			name:     "empty_snapshot_insert_string_root_file",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/",
				value: "123",
			},
			output: "{\"proc\":{\"\":\"123\"}}",
		},
		{
			name:     "empty_snapshot_insert_string_empty_file",
			snapshot: NewSnapshot(),
			args: args{
				file:  "",
				value: "123",
			},
			output: "{\"proc\":{\"\":\"123\"}}",
		},
		{
			name:     "empty_snapshot_insert_string",
			snapshot: NewSnapshot(),
			args: args{
				file:  "sys/kernel/random/uuid",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string_space",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123 ",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string_tab",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123\t",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name:     "empty_snapshot_insert_string_newline",
			snapshot: NewSnapshot(),
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123\n",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"}}}}}",
		},
		{
			name: "snapshot_with_ptrace_scope_insert_uuid_string",
			snapshot: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			args: args{
				file:  "/sys/kernel/random/uuid",
				value: "123",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"random\":{\"uuid\":\"123\"},\"yama\":{\"ptrace_scope\":\"2\"}}}}}",
		},
		{
			name: "snapshot_with_ptrace_scope_insert_hello_string_spaces",
			snapshot: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			args: args{
				file:  "/hello/world",
				value: " 123 ",
			},
			output: "{\"proc\":{\"hello\":{\"world\":\"123\"},\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"2\"}}}}}",
		},
		{
			name: "snapshot_with_ptrace_scope_override",
			snapshot: Snapshot{
				Proc: map[string]interface{}{
					"sys": map[string]interface{}{
						"kernel": map[string]interface{}{
							"yama": map[string]interface{}{
								"ptrace_scope": "2",
							},
						},
					},
				},
			},
			args: args{
				file:  "/sys/kernel/yama/ptrace_scope",
				value: "0",
			},
			output: "{\"proc\":{\"sys\":{\"kernel\":{\"yama\":{\"ptrace_scope\":\"0\"}}}}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.snapshot.InsertSnapshotEntry(tt.snapshot.Proc, tt.args.file, tt.args.value)
			got, err := tt.snapshot.ToJSON()
			if err != nil {
				t.Errorf("InsertSnapshotEntry - ToJSON error: %v", err)
				return
			}
			assert.JSONEqf(t, string(got), tt.output, "InsertSnapshotEntry error")
		})
	}
}

func TestWalkSystemControls(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"kernel/yama/ptrace_scope":       "2\n",
		"kernel/random/boot_id":          "secret\n",
		"net/ipv4/conf/lo/forwarding":    " 0 \n",
		"net/ipv4/tcp_rmem":              "4096\t131072\t6291456\n",
		"vm/write_only_file_not_visited": "1",
	}
	for name, value := range files {
		file := filepath.Join(root, name)
		assert.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		assert.NoError(t, os.WriteFile(file, []byte(value), 0o644))
	}
	assert.NoError(t, os.Chmod(filepath.Join(root, "vm/write_only_file_not_visited"), 0o200))

	got := make(map[string]string)
	err := walkSystemControls(root, []string{"boot_id"}, func(name, value string) {
		got[name] = value
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"kernel/yama/ptrace_scope":    "2",
		"kernel/random/boot_id":       redactedContent,
		"net/ipv4/conf/lo/forwarding": "0",
		"net/ipv4/tcp_rmem":           "4096\t131072\t6291456",
	}, got)
}

func TestWalkSystemControlsSkipsUnreadableDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read directories without the read permission")
	}

	root := t.TempDir()
	for name, value := range map[string]string{
		"fs/binfmt_misc/status": "enabled\n",
		"fs/file-max":           "100\n",
		"vm/swappiness":         "60\n",
	} {
		file := filepath.Join(root, name)
		assert.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		assert.NoError(t, os.WriteFile(file, []byte(value), 0o644))
	}
	unreadable := filepath.Join(root, "fs/binfmt_misc")
	assert.NoError(t, os.Chmod(unreadable, 0))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	got := make(map[string]string)
	err := walkSystemControls(root, nil, func(name, value string) {
		got[name] = value
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"fs/file-max":   "100",
		"vm/swappiness": "60",
	}, got)
}
