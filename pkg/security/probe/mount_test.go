// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMountResolver(t *testing.T) {
	// Prepare test cases
	type testCase struct {
		mountID               uint32
		numlower              int32
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
							127,
							71,
							52,
							27,
							0,
							"overlay",
							"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
							0,
							0,
							"",
							[16]byte{},
						},
					},
				},
				[]testCase{
					{
						127,
						0,
						"",
						"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
						nil,
					},
					{
						127,
						1,
						"",
						"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
						nil,
					},
					{
						0,
						0,
						"",
						"",
						nil,
					},
					{
						27,
						0,
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
							127,
						},
					},
				},
				[]testCase{
					{
						127,
						0,
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
							27,
							0,
							1,
							1,
							0,
							"ext4",
							"/",
							0,
							0,
							"",
							[16]byte{},
						},
					},
					{
						mount: &MountEvent{
							22,
							0,
							21,
							27,
							0,
							"sysfs",
							"/sys",
							0,
							0,
							"",
							[16]byte{},
						},
					},
					{
						mount: &MountEvent{
							31,
							0,
							26,
							22,
							0,
							"tmpfs",
							"/fs/cgroup",
							0,
							0,
							"",
							[16]byte{},
						},
					},
				},
				[]testCase{
					{
						27,
						0,
						"",
						"",
						nil,
					},
					{
						27,
						1,
						"",
						"",
						nil,
					},
					{
						31,
						0,
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
							27,
						},
					},
				},
				[]testCase{
					{
						27,
						0,
						"",
						"",
						ErrMountNotFound,
					},
					{
						22,
						0,
						"",
						"",
						ErrMountNotFound,
					},
					{
						31,
						0,
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
							27,
							0,
							1,
							1,
							0,
							"ext4",
							"/",
							0,
							0,
							"",
							[16]byte{},
						},
					},
					{
						mount: &MountEvent{
							176,
							71,
							52,
							27,
							0,
							"overlay",
							"/var/lib/docker/overlay2/f44b5a1fe134f57a31da79fa2e76ea09f8659a34edfa0fa2c3b4f52adbd91963/merged",
							0,
							0,
							"",
							[16]byte{},
						},
					},
					{
						mount: &MountEvent{
							638,
							0,
							52,
							635,
							0,
							"bind",
							"/",
							0,
							0,
							"",
							[16]byte{},
						},
					},
					{
						mount: &MountEvent{
							639,
							0,
							54,
							638,
							0,
							"proc",
							"proc",
							0,
							0,
							"",
							[16]byte{},
						},
					},
				},
				[]testCase{
					{
						639,
						1,
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
							176,
						},
					},
				},
				[]testCase{
					{
						176,
						0,
						"",
						"",
						ErrMountNotFound,
					},
					{
						638,
						0,
						"",
						"",
						ErrMountNotFound,
					},
					{
						639,
						0,
						"",
						"",
						ErrMountNotFound,
					},
				},
			},
		},
	}

	// Create mount resolver
	mr := NewMountResolver()
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
				cp, p, _, err := mr.GetMountPath(testC.mountID, testC.numlower)
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
