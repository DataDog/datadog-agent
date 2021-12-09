// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestMountResolver(t *testing.T) {
	// Prepare test cases
	type testCase struct {
		mountID           uint32
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
			"insert_overlay",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							SyscallEvent:  model.SyscallEvent{},
							MountID:       127,
							GroupID:       71,
							Device:        52,
							ParentMountID: 27,
							ParentInode:   0,
							FSType:        "overlay",
							MountPointStr: "/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
				},
				[]testCase{
					{
						127,
						"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
						nil,
					},
					{
						0,
						"",
						nil,
					},
					{
						27,
						"",
						ErrMountNotFound,
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
							SyscallEvent: model.SyscallEvent{},
							MountID:      127,
						},
					},
				},
				[]testCase{
					{
						127,
						"",
						ErrMountNotFound,
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
							SyscallEvent:  model.SyscallEvent{},
							MountID:       27,
							GroupID:       0,
							Device:        1,
							ParentMountID: 1,
							ParentInode:   0,
							FSType:        "ext4",
							MountPointStr: "/",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
					{
						mount: &model.MountEvent{
							SyscallEvent:  model.SyscallEvent{},
							MountID:       22,
							GroupID:       0,
							Device:        21,
							ParentMountID: 27,
							ParentInode:   0,
							FSType:        "sysfs",
							MountPointStr: "/sys",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
					{
						mount: &model.MountEvent{
							SyscallEvent:  model.SyscallEvent{},
							MountID:       31,
							GroupID:       0,
							Device:        26,
							ParentMountID: 22,
							ParentInode:   0,
							FSType:        "tmpfs",
							MountPointStr: "/sys/fs/cgroup",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
				},
				[]testCase{
					{
						27,
						"/",
						nil,
					},
					{
						22,
						"/sys",
						nil,
					},
					{
						31,
						"/sys/fs/cgroup",
						nil,
					},
				},
			},
		},
		{
			"remove_root",
			args{
				[]event{
					{
						umount: &model.UmountEvent{
							SyscallEvent: model.SyscallEvent{},
							MountID:      27,
						},
					},
				},
				[]testCase{
					{
						27,
						"",
						ErrMountNotFound,
					},
					{
						22,
						"",
						ErrMountNotFound,
					},
					{
						31,
						"",
						ErrMountNotFound,
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
							SyscallEvent:  model.SyscallEvent{},
							MountID:       27,
							GroupID:       0,
							Device:        1,
							ParentMountID: 1,
							ParentInode:   0,
							FSType:        "ext4",
							MountPointStr: "/",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
					{
						mount: &model.MountEvent{
							SyscallEvent:  model.SyscallEvent{},
							MountID:       176,
							GroupID:       71,
							Device:        52,
							ParentMountID: 27,
							ParentInode:   0,
							FSType:        "overlay",
							MountPointStr: "/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
					{
						mount: &model.MountEvent{
							SyscallEvent:  model.SyscallEvent{},
							MountID:       638,
							GroupID:       71,
							Device:        52,
							ParentMountID: 635,
							ParentInode:   0,
							FSType:        "bind",
							MountPointStr: "/",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
					{
						mount: &model.MountEvent{
							SyscallEvent:  model.SyscallEvent{},
							MountID:       639,
							GroupID:       0,
							Device:        54,
							ParentMountID: 638,
							ParentInode:   0,
							FSType:        "proc",
							MountPointStr: "proc",
							RootMountID:   0,
							RootInode:     0,
							RootStr:       "",
							FSTypeRaw:     [16]byte{},
						},
					},
				},
				[]testCase{
					{
						639,
						"proc",
						nil,
					},
				},
			},
		},
		{
			"remove_container",
			args{
				[]event{
					{
						umount: &model.UmountEvent{
							SyscallEvent: model.SyscallEvent{},
							MountID:      176,
						},
					},
				},
				[]testCase{
					{
						176,
						"",
						ErrMountNotFound,
					},
					{
						638,
						"",
						ErrMountNotFound,
					},
					{
						639,
						"",
						ErrMountNotFound,
					},
				},
			},
		},
	}

	// Create mount resolver
	mr, _ := NewMountResolver(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, evt := range tt.args.events {
				if evt.mount != nil {
					mr.insert(evt.mount)
				}
				if evt.umount != nil {
					if err := mr.Delete(evt.umount.MountID); err != nil {
						t.Fatal(err)
					}
				}
			}

			mr.dequeue(time.Now().Add(1 * time.Minute))

			for _, testC := range tt.args.cases {
				_, p, _, err := mr.GetMountPath(testC.mountID)
				if err != nil {
					if testC.expectedError != nil {
						assert.Equal(t, testC.expectedError.Error(), err.Error())
					} else {
						t.Fatal(err)
					}
					continue
				}
				assert.Equal(t, testC.expectedMountPath, p)
			}
		})
	}
}

func TestGetParentPath(t *testing.T) {
	parentPathCache, err := simplelru.NewLRU(256, nil)
	if err != nil {
		t.Fatal(err)
	}

	mr := &MountResolver{
		mounts: map[uint32]*model.MountEvent{
			1: {
				MountID:       1,
				ParentMountID: 3,
				MountPointStr: "/a",
			},
			2: {
				MountID:       2,
				ParentMountID: 1,
				MountPointStr: "/b",
			},
			3: {
				MountID:       3,
				ParentMountID: 2,
				MountPointStr: "/c",
			},
		},
		parentPathCache: parentPathCache,
	}

	parentPath := mr.getParentPath(3)
	assert.Equal(t, "/a/b/c", parentPath)
}

func BenchmarkGetParentPath(b *testing.B) {
	parentPathCache, err := simplelru.NewLRU(256, nil)
	if err != nil {
		b.Fatal(err)
	}

	mr := &MountResolver{
		mounts:          make(map[uint32]*model.MountEvent),
		parentPathCache: parentPathCache,
	}

	var parentID uint32
	for i := uint32(0); i != 100; i++ {
		mr.mounts[i+1] = &model.MountEvent{
			MountID:       i + 1,
			ParentMountID: parentID,
			MountPointStr: fmt.Sprintf("/%d", i+1),
		}
		parentID = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mr.getParentPath(0)
	}
}
