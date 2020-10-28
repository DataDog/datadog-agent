// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMountResolver(t *testing.T) {
	// Prepare test cases
	type testCase struct {
		mountID               uint32
		expectedMountPath     string
		expectedContainerPath string
		expectedError         error
	}
	type event struct {
		mount  *MountEvent
		umount *UmountEvent
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						"",
						nil,
					},
					{
						0,
						"",
						"",
						nil,
					},
					{
						27,
						"",
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
						umount: &UmountEvent{
							BaseEvent: BaseEvent{},
							MountID:   127,
						},
					},
				},
				[]testCase{
					{
						127,
						"",
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						"",
						nil,
					},
					{
						22,
						"/sys",
						"",
						nil,
					},
					{
						31,
						"/sys/fs/cgroup",
						"",
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
						umount: &UmountEvent{
							BaseEvent: BaseEvent{},
							MountID:   27,
						},
					},
				},
				[]testCase{
					{
						27,
						"",
						"",
						ErrMountNotFound,
					},
					{
						22,
						"",
						"",
						ErrMountNotFound,
					},
					{
						31,
						"",
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						mount: &MountEvent{
							BaseEvent:     BaseEvent{},
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
						"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
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
						umount: &UmountEvent{
							BaseEvent: BaseEvent{},
							MountID:   176,
						},
					},
				},
				[]testCase{
					{
						176,
						"",
						"",
						ErrMountNotFound,
					},
					{
						638,
						"",
						"",
						ErrMountNotFound,
					},
					{
						639,
						"",
						"",
						ErrMountNotFound,
					},
				},
			},
		},
	}

	// Create mount resolver
	mr := NewMountResolver(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, evt := range tt.args.events {
				if evt.mount != nil {
					mr.Insert(evt.mount)
				}
				if evt.umount != nil {
					if err := mr.Delete(evt.umount.MountID); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, testC := range tt.args.cases {
				cp, p, _, err := mr.GetMountPath(testC.mountID)
				if err != nil {
					if testC.expectedError != nil {
						assert.Equal(t, testC.expectedError.Error(), err.Error())
					} else {
						t.Fatal(err)
					}
					continue
				}
				assert.Equal(t, testC.expectedMountPath, p)
				assert.Equal(t, testC.expectedContainerPath, cp)
			}
		})
	}
}
