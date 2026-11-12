// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestMountResolver(t *testing.T) {
	// Prepare test cases
	type testCase struct {
		mountID           uint32
		device            uint32
		expectedMountPath string
		expectedError     error
	}
	type event struct {
		mount  *model.MountEvent
		umount *model.UmountEvent
	}
	type args struct {
		events []event
		cases  []testCase
	}
	tests := []struct {
		name string
		args args
	}{

		{
			"insert_root",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 27,
								Device:  1,
								ParentPathKey: model.PathKey{
									MountID: 1,
								},
								FSType:        "ext4",
								MountPointStr: "/",
								RootStr:       "",
							},
						},
					},
				},
				[]testCase{
					{
						27,
						1,
						"/",
						nil,
					},
				},
			},
		},
		{
			"insert_overlay",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 127,
								Device:  52,
								ParentPathKey: model.PathKey{
									MountID: 27,
								},
								FSType:        "overlay",
								MountPointStr: "/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
								RootStr:       "",
							},
						},
					},
				},
				[]testCase{
					{
						127,
						0,
						"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
						nil,
					},
					{
						0,
						0,
						"",
						ErrMountUndefined,
					},
					{
						22,
						0,
						"",
						&ErrMountNotFound{MountID: 22},
					},
				},
			},
		},
		{
			"remove_overlay",
			args{
				[]event{
					{
						umount: &model.UmountEvent{
							MountID: 127,
						},
					},
				},
				[]testCase{
					{
						127,
						0,
						"",
						&ErrMountNotFound{MountID: 127},
					},
				},
			},
		},
		{
			"mount_points_lineage",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 27,
								Device:  1,
								ParentPathKey: model.PathKey{
									MountID: 1,
								},
								FSType:        "ext4",
								MountPointStr: "/",
								RootStr:       "",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 22,
								Device:  21,
								ParentPathKey: model.PathKey{
									MountID: 27,
								},
								FSType:        "sysfs",
								MountPointStr: "/sys",
								RootStr:       "",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 31,
								Device:  26,
								ParentPathKey: model.PathKey{
									MountID: 22,
								},
								FSType:        "tmpfs",
								MountPointStr: "/fs/cgroup",
								RootStr:       "",
							},
						},
					},
				},
				[]testCase{
					{
						27,
						0,
						"/",
						nil,
					},
					{
						22,
						0,
						"/sys",
						nil,
					},
					{
						31,
						0,
						"/sys/fs/cgroup",
						nil,
					},
				},
			},
		},
		{
			"container_creation",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 27,
								Device:  1,
								ParentPathKey: model.PathKey{
									MountID: 1,
								},
								FSType:        "ext4",
								MountPointStr: "/",
								RootStr:       "",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 176,
								Device:  52,
								ParentPathKey: model.PathKey{
									MountID: 27,
								},
								FSType:        "overlay",
								MountPointStr: "/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
								RootStr:       "",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 638,
								Device:  53,
								ParentPathKey: model.PathKey{
									MountID: 635,
								},
								FSType:        "bind",
								MountPointStr: "/",
								RootStr:       "",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 639,
								Device:  54,
								ParentPathKey: model.PathKey{
									MountID: 638,
								},
								FSType:        "proc",
								MountPointStr: "proc",
								RootStr:       "",
							},
						},
					},
				},
				[]testCase{
					{
						639,
						0,
						"/proc",
						nil,
					},
				},
			},
		},
		{
			"identical_mountpoints",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 32,
								Device:  97,
								ParentPathKey: model.PathKey{
									MountID: 638,
								},
								MountPointStr: "/",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 41,
								Device:  98,
								ParentPathKey: model.PathKey{
									MountID: 32,
								},
								MountPointStr: "/tmp",
							},
						},
					},
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 42,
								Device:  99,
								ParentPathKey: model.PathKey{
									MountID: 41,
								},
								MountPointStr: "/tmp",
							},
						},
					},
				},
				[]testCase{
					{
						32,
						0,
						"/",
						nil,
					},
					{
						41,
						0,
						"/tmp",
						nil,
					},
					{
						42,
						0,
						"/tmp/tmp",
						nil,
					},
				},
			},
		},
	}

	var (
		pid uint32 = 1
	)

	// Create mount resolver
	mr, _ := NewResolver(nil, nil, ResolverOpts{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, evt := range tt.args.events {
				if evt.mount != nil {
					mr.insert(&evt.mount.Mount)
				}
				if evt.umount != nil {
					mount, _, _, err := mr.ResolveMount(evt.umount.MountID, pid)
					if err != nil {
						t.Fatal(err)
					}
					mr.delete(mount)
				}
			}

			for _, testC := range tt.args.cases {
				p, _, _, err := mr.ResolveMountPath(testC.mountID, pid)
				if err != nil {
					if testC.expectedError != nil {
						assert.Equal(t, testC.expectedError.Error(), err.Error())
					} else {
						t.Fatalf("case %v: %v", testC, err)
					}
					continue
				}
				assert.Equal(t, testC.expectedMountPath, p)
			}
		})
	}
}

func TestMountGetParentPath(t *testing.T) {
	mounts := map[uint32]*model.Mount{
		1: {
			MountID:       1,
			MountPointStr: "/",
		},
		2: {
			MountID: 2,
			ParentPathKey: model.PathKey{
				MountID: 1,
			},
			MountPointStr: "/a",
		},
		3: {
			MountID: 3,
			ParentPathKey: model.PathKey{
				MountID: 2,
			},
			MountPointStr: "/b",
		},
		4: {
			MountID: 4,
			ParentPathKey: model.PathKey{
				MountID: 3,
			},
			MountPointStr: "/c",
		},
	}

	// Create mount resolver
	mr, _ := NewResolver(nil, nil, ResolverOpts{})
	for _, m := range mounts {
		mr.mounts.Add(m.MountID, m)
	}

	parentPath, _, _, err := mr.getMountPath(4, 1)
	assert.NoError(t, err)
	assert.Equal(t, "/a/b/c", parentPath)
}

func TestMountLoop(t *testing.T) {
	mounts := map[uint32]*model.Mount{
		1: {
			MountID:       1,
			MountPointStr: "/",
		},
		2: {
			MountID: 2,
			ParentPathKey: model.PathKey{
				MountID: 4,
			},
			MountPointStr: "/a",
		},
		3: {
			MountID: 3,
			ParentPathKey: model.PathKey{
				MountID: 2,
			},
			MountPointStr: "/b",
		},
		4: {
			MountID: 4,
			ParentPathKey: model.PathKey{
				MountID: 3,
			},
			MountPointStr: "/c",
		},
	}

	// Create mount resolver
	mr, _ := NewResolver(nil, nil, ResolverOpts{})
	for _, m := range mounts {
		mr.mounts.Add(m.MountID, m)
	}

	parentPath, _, _, err := mr.getMountPath(3, 1)
	assert.Equal(t, ErrMountLoop, err)
	assert.Equal(t, "", parentPath)
}

func TestIterateNamespacePreservesPivotedContainerMounts(t *testing.T) {
	const (
		containerNs  = uint32(4026532200)
		containerPid = uint32(42)
		clonedNs     = uint32(4026532201)
		clonedPid    = uint32(43)
	)

	fakeProc := t.TempDir()
	origProcFSRoot := kernel.ProcFSRoot
	kernel.ProcFSRoot = func() string { return fakeProc }
	t.Cleanup(func() {
		kernel.ProcFSRoot = origProcFSRoot
	})

	writeFakeProcPid := func(t *testing.T, pid uint32, nsIno uint32, mountInfo string) {
		t.Helper()
		pidDir := filepath.Join(fakeProc, strconv.FormatUint(uint64(pid), 10))
		require.NoError(t, os.MkdirAll(filepath.Join(pidDir, "ns"), 0o755))
		require.NoError(t, os.Symlink(fmt.Sprintf("mnt:[%d]", nsIno), filepath.Join(pidDir, "ns", "mnt")))
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "mountinfo"), []byte(mountInfo), 0o644))
	}

	writeFakeProcPid(t, containerPid, containerNs, ""+
		"700 699 0:45 / / rw,relatime - overlay overlay rw\n"+
		"701 700 0:30 /@/tmp/test-dir /tmp/test-dir rw,relatime - btrfs /dev/sda1 rw,subvol=/@/tmp/test-dir\n")

	mr, err := NewResolver(nil, nil, ResolverOpts{UseProcFS: true})
	require.NoError(t, err)

	require.NoError(t, mr.Insert(model.Mount{
		MountID:        700,
		NamespaceInode: containerNs,
		FSType:         "overlay",
		MountPointStr:  "/",
		Path:           "/",
		RootStr:        "/",
		Origin:         model.MountOriginPivotRoot,
	}))
	require.NoError(t, mr.Insert(model.Mount{
		MountID:        701,
		NamespaceInode: containerNs,
		ParentPathKey:  model.PathKey{MountID: 700},
		FSType:         "btrfs",
		MountPointStr:  "/tmp/test-dir",
		RootStr:        "/test-dir",
		Origin:         model.MountOriginEvent,
	}))

	var visited []*model.Mount
	mr.IterateNamespace(containerNs, containerPid, func(m *model.Mount) {
		visited = append(visited, m)
	})
	assert.Len(t, visited, 2)

	rootPath, _, origin, err := mr.ResolveMountRoot(701, containerPid)
	require.NoError(t, err)
	assert.Equal(t, "/test-dir", rootPath)
	assert.Equal(t, model.MountOriginEvent, origin)

	writeFakeProcPid(t, clonedPid, clonedNs, ""+
		"499 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n"+
		"500 499 0:25 / /dev/shm rw,relatime - tmpfs tmpfs rw\n"+
		"600 500 0:26 / /dev/shm rw,relatime - tmpfs tmpfs rw\n")

	require.NoError(t, mr.Insert(model.Mount{
		MountID:        600,
		NamespaceInode: clonedNs,
		ParentPathKey:  model.PathKey{MountID: 500},
		FSType:         "tmpfs",
		MountPointStr:  "/",
		RootStr:        "/",
		Origin:         model.MountOriginEvent,
	}))

	visited = nil
	mr.IterateNamespace(clonedNs, clonedPid, func(m *model.Mount) {
		visited = append(visited, m)
	})
	assert.Len(t, visited, 3)
}

func TestGetPidProcfsZombieLeaderFallback(t *testing.T) {
	const (
		zombieLeaderPid = uint32(100)
		workerTid       = uint32(101)
		expectedNs      = uint32(4026532351)
	)

	fakeProc := t.TempDir()
	leaderTaskDir := filepath.Join(fakeProc, strconv.FormatUint(uint64(zombieLeaderPid), 10), "task", strconv.FormatUint(uint64(zombieLeaderPid), 10))
	require.NoError(t, os.MkdirAll(leaderTaskDir, 0o755))

	workerTaskDir := filepath.Join(fakeProc, strconv.FormatUint(uint64(zombieLeaderPid), 10), "task", strconv.FormatUint(uint64(workerTid), 10))
	require.NoError(t, os.MkdirAll(filepath.Join(workerTaskDir, "ns"), 0o755))
	require.NoError(t, os.Symlink(fmt.Sprintf("mnt:[%d]", expectedNs), filepath.Join(workerTaskDir, "ns", "mnt")))
	require.NoError(t, os.WriteFile(filepath.Join(workerTaskDir, "mountinfo"), []byte("800 799 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n"), 0o644))

	var mounts []*model.Mount
	err := GetPidProcfs(fakeProc, zombieLeaderPid, func(m *model.Mount) {
		mounts = append(mounts, m)
	})
	require.NoError(t, err)
	require.Len(t, mounts, 1)
	assert.Equal(t, uint32(800), mounts[0].MountID)
	assert.Equal(t, expectedNs, mounts[0].NamespaceInode)
}

func TestInsertReusedMountIDDifferentUniqueID(t *testing.T) {
	mr, err := NewResolver(nil, nil, ResolverOpts{})
	require.NoError(t, err)

	require.NoError(t, mr.Insert(model.Mount{
		MountID:       1,
		MountPointStr: "/",
	}))
	require.NoError(t, mr.Insert(model.Mount{
		MountID:       42,
		MountIDUnique: 100,
		ParentPathKey: model.PathKey{MountID: 1},
		MountPointStr: "/old",
		Origin:        model.MountOriginEvent,
	}))
	require.NoError(t, mr.Insert(model.Mount{
		MountID:       43,
		MountIDUnique: 101,
		ParentPathKey: model.PathKey{MountID: 42},
		MountPointStr: "/sub",
		Origin:        model.MountOriginEvent,
	}))

	require.NoError(t, mr.Insert(model.Mount{
		MountID:       42,
		MountIDUnique: 200,
		ParentPathKey: model.PathKey{MountID: 1},
		MountPointStr: "/new",
		Origin:        model.MountOriginEvent,
	}))

	reused, _, _, err := mr.ResolveMount(42, 0)
	require.NoError(t, err)
	assert.Empty(t, reused.Children)

	p, _, _, err := mr.ResolveMountPath(42, 0)
	require.NoError(t, err)
	assert.Equal(t, "/new", p)
}

func TestProcfsOutOfOrderDanglingAndResyncCleanup(t *testing.T) {
	mr, err := NewResolver(nil, nil, ResolverOpts{UseProcFS: true})
	require.NoError(t, err)

	require.NoError(t, mr.Insert(model.Mount{
		MountID:       1,
		MountPointStr: "/",
		Path:          "/",
		Origin:        model.MountOriginProcfs,
	}))
	require.NoError(t, mr.Insert(model.Mount{
		MountID:       51,
		ParentPathKey: model.PathKey{MountID: 50},
		MountPointStr: "sub",
		Path:          "/mnt/sub",
		Origin:        model.MountOriginProcfs,
	}))
	assert.Equal(t, 1, mr.dangling.Len())

	require.NoError(t, mr.Insert(model.Mount{
		MountID:       50,
		ParentPathKey: model.PathKey{MountID: 1},
		MountPointStr: "mnt",
		Path:          "/mnt",
		Origin:        model.MountOriginProcfs,
	}))
	assert.Equal(t, 0, mr.dangling.Len())

	parent, _, _, err := mr.ResolveMount(50, 0)
	require.NoError(t, err)
	assert.Equal(t, []uint32{51}, parent.Children)

	require.NoError(t, mr.Insert(model.Mount{
		MountID:       50,
		ParentPathKey: model.PathKey{MountID: 1},
		MountPointStr: "/moved",
		Origin:        model.MountOriginEvent,
	}))

	childPath, _, _, err := mr.ResolveMountPath(51, 0)
	require.NoError(t, err)
	assert.Equal(t, "/moved/sub", childPath)
}

func BenchmarkGetParentPath(b *testing.B) {
	// Create mount resolver
	mr, _ := NewResolver(nil, nil, ResolverOpts{})

	mr.mounts.Add(1, &model.Mount{
		MountID:       1,
		MountPointStr: "/",
	})

	for i := uint32(1); i != 100; i++ {
		mr.mounts.Add(i+1, &model.Mount{
			MountID: i + 1,
			ParentPathKey: model.PathKey{
				MountID: i,
			},
			MountPointStr: fmt.Sprintf("/%d", i+1),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = mr.getMountPath(100, 1)
	}
}
