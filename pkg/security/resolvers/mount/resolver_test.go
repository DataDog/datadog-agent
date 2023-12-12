// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
			"insert_device",
			args{
				[]event{
					{
						mount: &model.MountEvent{
							Mount: model.Mount{
								MountID: 458,
								Device:  44,
								ParentPathKey: model.PathKey{
									MountID: 27,
								},
								MountPointStr: "/usr",
								RootStr:       "",
							},
						},
					},
				},
				[]testCase{
					{
						459,
						44,
						"/usr",
						nil,
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
			"remove_root",
			args{
				[]event{
					{
						umount: &model.UmountEvent{
							MountID: 27,
						},
					},
				},
				[]testCase{
					{
						27,
						0,
						"",
						&ErrMountNotFound{MountID: 27},
					},
					{
						22,
						0,
						"",
						&ErrMountNotFound{MountID: 22},
					},
					{
						31,
						0,
						"",
						&ErrMountNotFound{MountID: 31},
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
			"remove_container",
			args{
				[]event{
					{
						umount: &model.UmountEvent{
							MountID: 176,
						},
					},
					{
						umount: &model.UmountEvent{
							MountID: 638,
						},
					},
				},
				[]testCase{
					{
						176,
						0,
						"",
						&ErrMountNotFound{MountID: 176},
					},
					{
						638,
						0,
						"",
						&ErrMountNotFound{MountID: 638},
					},
					{
						639,
						0,
						"",
						&ErrMountNotFound{MountID: 639},
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

	cr, _ := cgroup.NewResolver(nil)

	// Create mount resolver
	mr, _ := NewResolver(nil, cr, ResolverOpts{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, evt := range tt.args.events {
				if evt.mount != nil {
					mr.insert(&evt.mount.Mount, pid)
				}
				if evt.umount != nil {
					mount, err := mr.ResolveMount(evt.umount.MountID, 0, pid, "")
					if err != nil {
						t.Fatal(err)
					}
					mr.delete(mount)

					// wait end of remption
					time.Sleep(redemptionTime)
				}
			}

			for _, testC := range tt.args.cases {
				p, err := mr.ResolveMountPath(testC.mountID, testC.device, pid, "")
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
	mr := &Resolver{
		mounts: map[uint32]*model.Mount{
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
		},
	}

	parentPath, err := mr.getMountPath(4, 44, 1)
	assert.NoError(t, err)
	assert.Equal(t, "/a/b/c", parentPath)
}

func TestMountLoop(t *testing.T) {
	mr := &Resolver{
		mounts: map[uint32]*model.Mount{
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
		},
	}

	parentPath, err := mr.getMountPath(3, 44, 1)
	assert.Equal(t, ErrMountLoop, err)
	assert.Equal(t, "", parentPath)
}

func BenchmarkGetParentPath(b *testing.B) {
	mr := &Resolver{
		mounts: make(map[uint32]*model.Mount),
	}

	mr.mounts[1] = &model.Mount{
		MountID:       1,
		MountPointStr: "/",
	}

	for i := uint32(1); i != 100; i++ {
		mr.mounts[i+1] = &model.Mount{
			MountID: i + 1,
			ParentPathKey: model.PathKey{
				MountID: i,
			},
			MountPointStr: fmt.Sprintf("/%d", i+1),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mr.getMountPath(100, 44, 1)
	}
}
