// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestReaderV2(t *testing.T) {
	fakeFsPath := t.TempDir()
	paths := []string{
		"kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e.slice/crio-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40.scope",
		"kubepods/kubepods-besteffort/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e/2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41",
		"system.slice/run-containerd-io.containerd.runtime.v2.task-k8s.io-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42-rootfs.mount",
		"kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podc704ef4c297ab11032b83ce52cbfc87b.slice/cri-containerd-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42.scope",
		"libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2/system.slice/var-lib-docker-containers-1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503-mounts-shm.mount",
		"libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda2acd1bccd50fd7790183537181f658e.slice/docker-1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503.scope",
	}

	// Create mock directories for paths and corresponding inodes.
	for _, p := range paths {
		fullPath := filepath.Join(fakeFsPath, p)
		assert.NoErrorf(t, os.MkdirAll(fullPath, 0o750), "impossible to create temp directory '%s'", fullPath)
	}

	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "cgroup.controllers"), []byte("cpu io memory"), 0o640))

	controllers := map[string]struct{}{
		"cpu":    {},
		"io":     {},
		"memory": {},
	}

	r, err := newReaderV2("", fakeFsPath, ContainerFilter, "")
	r.pidMapper = nil
	assert.NoError(t, err)
	assert.NotNil(t, r)

	cgroups := make(map[string]Cgroup)
	cgroups, err = r.parseCgroups(cgroups)
	assert.NoError(t, err)

	expected := map[string]Cgroup{
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40": newCgroupV2("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40", fakeFsPath, paths[0], controllers, r.pidMapper),
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41": newCgroupV2("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41", fakeFsPath, paths[1], controllers, r.pidMapper),
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42": newCgroupV2("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42", fakeFsPath, paths[3], controllers, r.pidMapper),
		"1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503": newCgroupV2("1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503", fakeFsPath, paths[5], controllers, r.pidMapper),
		"6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2": newCgroupV2("6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2", fakeFsPath, "libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2", controllers, r.pidMapper),
	}

	// Initialize Inodes
	for i := range cgroups {
		inode := cgroups[i].Inode()
		assert.NotEqual(t, uint64(0), inode)
	}
	for _, cgroup := range expected {
		inode := cgroup.Inode()
		assert.NotEqual(t, uint64(0), inode)
	}

	assert.Empty(t, cmp.Diff(expected, cgroups, cmp.AllowUnexported(cgroupV2{})))
}

func BenchmarkParseCgroups_Small(b *testing.B) {
	benchmarkParseCgroups(b, 5)
}

func BenchmarkParseCgroups_Medium(b *testing.B) {
	benchmarkParseCgroups(b, 50)
}

func BenchmarkParseCgroups_Large(b *testing.B) {
	// Create memory profile in /tmp with random name
	memProfile := filepath.Join(os.TempDir(), fmt.Sprintf("parse-cgroups-large-memprofile-%d.out", time.Now().UnixNano()))
	f, err := os.Create(memProfile)
	if err != nil {
		b.Fatalf("Failed to create memory profile: %v", err)
	}
	defer f.Close()
	b.Logf("Memory profile will be saved to: %s", memProfile)

	// Set memory profile rate to capture all allocations
	runtime.MemProfileRate = 1

	// Run the benchmark
	benchmarkParseCgroups(b, 500)

	// Write the final memory profile
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		b.Fatalf("Failed to write memory profile: %v", err)
	}
}

func benchmarkParseCgroups(b *testing.B, numCgroups int) {
	// Create a temporary directory structure that mimics cgroup v2
	tmpDir := b.TempDir()

	// Create root cgroup.controllers file
	if err := os.WriteFile(filepath.Join(tmpDir, "cgroup.controllers"), []byte("cpu memory io pids\n"), 0644); err != nil {
		b.Fatal(err)
	}

	// Create a mock cgroup hierarchy
	createMockCgroups(tmpDir, numCgroups)

	// Create a readerV2 instance
	reader, err := newReaderV2("/proc", tmpDir, DefaultFilter, "")
	if err != nil {
		b.Fatal(err)
	}

	// Enable allocation reporting
	b.ReportAllocs()

	// Reset timer before actual benchmark
	b.ResetTimer()

	// Run the benchmark
	b.Run("parseCgroups", func(b *testing.B) {
		cgroups := make(map[string]Cgroup)
		for i := 0; i < b.N; i++ {
			_, err := reader.parseCgroups(cgroups)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func createMockCgroups(root string, numCgroups int) {
	// Create a typical cgroup hierarchy with varying depths and patterns
	dirs := make([]string, 0, numCgroups)

	// System slice hierarchy with various services
	systemServices := []string{
		"docker", "kubelet", "containerd", "systemd-journald", "sshd",
		"cron", "dbus", "networkd", "logind", "polkitd",
	}
	for _, service := range systemServices {
		dirs = append(dirs, fmt.Sprintf("system.slice/%s.service", service))
		dirs = append(dirs, fmt.Sprintf("system.slice/%s.service/%s-1234.scope", service, service))
	}

	// User slice hierarchy with multiple users and sessions
	users := []string{"1000", "1001", "1002", "1003", "1004"}
	for _, user := range users {
		dirs = append(dirs, fmt.Sprintf("user.slice/user-%s.slice", user))
		for session := 1; session <= 3; session++ {
			dirs = append(dirs, fmt.Sprintf("user.slice/user-%s.slice/session-%d.scope", user, session))
		}
	}

	// Container hierarchies with different runtimes
	containerRuntimes := []string{"docker", "containerd", "crio"}
	for _, runtime := range containerRuntimes {
		for i := 0; i < 5; i++ {
			containerID := fmt.Sprintf("%s-%x", runtime, i)
			dirs = append(dirs, fmt.Sprintf("kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod%s.slice/%s.scope",
				containerID, containerID))
		}
	}

	// Add more cgroups to reach desired size with varying patterns
	for i := len(dirs); i < numCgroups; i++ {
		// Create unique paths with varying patterns
		pattern := i % 4
		var path string
		switch pattern {
		case 0:
			// Simple nested structure
			depth := (i % 3) + 1
			path = fmt.Sprintf("custom.slice/group-%d", i)
			for j := 1; j < depth; j++ {
				path = fmt.Sprintf("%s/subgroup-%d", path, j)
			}
		case 1:
			// Container-like structure
			path = fmt.Sprintf("containers.slice/container-%x/container-%x.scope", i, i)
		case 2:
			// Service-like structure
			path = fmt.Sprintf("services.slice/service-%d.service/service-%d.scope", i, i)
		case 3:
			// User-like structure
			path = fmt.Sprintf("users.slice/user-%d.slice/session-%d.scope", i, i%10)
		}
		dirs = append(dirs, path)
	}

	// Create directories and files
	for _, dir := range dirs {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			panic(err)
		}

		// Create typical cgroup files with varying content
		files := []string{
			"cgroup.procs",
			"cgroup.controllers",
			"cpu.stat",
			"memory.current",
			"memory.stat",
			"io.stat",
			"pids.current",
		}

		for _, file := range files {
			// Generate some realistic-looking content
			content := generateCgroupFileContent(file)
			if err := os.WriteFile(filepath.Join(path, file), []byte(content), 0644); err != nil {
				panic(err)
			}
		}
	}
}

func generateCgroupFileContent(filename string) string {
	switch filename {
	case "cgroup.procs":
		return "1234\n5678\n"
	case "cgroup.controllers":
		return "cpu memory io pids\n"
	case "cpu.stat":
		return "usage_usec 1234567\nuser_usec 1000000\nsystem_usec 234567\n"
	case "memory.current":
		return "123456789\n"
	case "memory.stat":
		return "anon 1000000\nfile 2000000\nkernel_stack 50000\nslab 100000\n"
	case "io.stat":
		return "8:0 rbytes=123456 wbytes=789012 rios=100 wios=200\n"
	case "pids.current":
		return "10\n"
	default:
		return "0\n"
	}
}
