// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type input struct {
	path        string
	fileEvent   *model.FileEvent
	processNode *ProcessNode
}

var tests = []struct {
	name  string
	input input
	want  string
}{
	{
		name: "proc_1",
		input: input{
			path:      "/host/proc/2144/smaps",
			fileEvent: &model.FileEvent{},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2144,
				},
			},
		},
		want: "/host/proc/self/smaps",
	},
	{
		name: "proc_2",
		input: input{
			path:      "/proc/2144/status",
			fileEvent: &model.FileEvent{},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2,
				},
			},
		},
		want: "/proc/<num>/status",
	},
	{
		name: "proc_3",
		input: input{
			path:      "/proc/self/exe",
			fileEvent: &model.FileEvent{},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2,
				},
			},
		},
		want: "/proc/self/exe",
	},
	{
		name: "proc_4",
		input: input{
			path:      "/host/proc/1/smaps",
			fileEvent: &model.FileEvent{},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2144,
				},
			},
		},
		want: "/host/proc/<num>/smaps",
	},
	{
		name: "proc_task_1",
		input: input{
			path:      "/host/proc/2144/task/2144/smaps",
			fileEvent: &model.FileEvent{},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2144,
				},
			},
		},
		want: "/host/proc/self/task/<num>/smaps",
	},
	{
		name: "proc_task_1",
		input: input{
			path:      "/host/proc/self/task/2144/smaps",
			fileEvent: &model.FileEvent{},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2,
				},
			},
		},
		want: "/host/proc/self/task/<num>/smaps",
	},
	{
		name: "cgroup_1",
		input: input{
			path: "/sys/fs/cgroup/kubepods.slice/kubepods-pod13asf23dasf3.slice/kubepods.slice/cri-containerd-3i47135.scope/cpuset.threads",
			fileEvent: &model.FileEvent{
				Filesystem: "sysfs",
			},
		},
		want: "/sys/fs/cgroup/kubepods.slice/kubepods-<any>.slice/kubepods.slice/cri-containerd-<any>.scope/cpuset.threads",
	},
	{
		name: "proc_cgroup_1",
		input: input{
			path: "/host/proc/1/root/sys/fs/cgroup/kubepods.slice/kubepods-pod13asf23dasf3.slice/kubepods.slice/cri-containerd-3i47135.scope/cpuset.threads",
			fileEvent: &model.FileEvent{
				Filesystem: "sysfs",
			},
			processNode: &ProcessNode{
				Process: ProcessInfo{
					Pid: 2,
				},
			},
		},
		want: "/host/proc/<num>/root/sys/fs/cgroup/kubepods.slice/kubepods-<any>.slice/kubepods.slice/cri-containerd-<any>.scope/cpuset.threads",
	},
	{
		name: "container_id_1",
		input: input{
			path: "/var/run/docker/overlay2/47c1f1930c1831f2359c6d276912c583be1cda5924233cf273022b91763a20f7/merged/etc/passwd",
			fileEvent: &model.FileEvent{
				Filesystem: "sysfs",
			},
		},
		want: "/var/run/docker/overlay2/<container_id>/merged/etc/passwd",
	},
	{
		name: "container_id_2",
		input: input{
			path:      "/var/lib/docker/containers/47c1f1930c1831f2359c6d276912c583be1cda5924233cf273022b91763a20f7/47c1f1930c1831f2359c6d276912c583be1cda5924233cf273022b91763a20f7-json.log",
			fileEvent: &model.FileEvent{},
		},
		want: "/var/lib/docker/containers/<container_id>/<container_id>-json.log",
	},
	{
		name: "uuid_not_container_id",
		input: input{
			path:      "/srv/app/8f14e45f-ceea-4a7b-9c1d-2b3c4d5e6f70.json",
			fileEvent: &model.FileEvent{},
		},
		want: "/srv/app/8f14e45f-ceea-4a7b-9c1d-2b3c4d5e6f70.json",
	},
	{
		name: "long_hex_not_container_id",
		input: input{
			path:      "/srv/cache/cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
			fileEvent: &model.FileEvent{},
		},
		want: "/srv/cache/cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	},
	{
		name: "block_device_1",
		input: input{
			path: "/host/proc/self/root/sys/devices/virtual/block/dm-0",
			fileEvent: &model.FileEvent{
				Filesystem: "sysfs",
			},
		},
		want: "/host/proc/self/root/sys/devices/virtual/block/dm-<num>",
	},
	{
		name: "block_device_2",
		input: input{
			path: "/sys/devices/virtual/block/loop1234",
			fileEvent: &model.FileEvent{
				Filesystem: "sysfs",
			},
		},
		want: "/sys/devices/virtual/block/loop<num>",
	},
	{
		name: "k8s_service_account_secret_1",
		input: input{
			path: "/run/secrets/kubernetes.io/serviceaccount/..2023_05_25_09_34_13.734441344/token",
		},
		want: "/run/secrets/kubernetes.io/serviceaccount/..<date>.<num>/token",
	},
	{
		name: "k8s_service_account_secret_2",
		input: input{
			path: "/run/secrets/kubernetes.io/serviceaccount/..data/token",
		},
		want: "/run/secrets/kubernetes.io/serviceaccount/..data/token",
	},
}

func TestPathsReducer_ReducePath(t *testing.T) {
	r := NewPathsReducer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, r.ReducePath(tt.input.path, tt.input.fileEvent, tt.input.processNode), "ReducePath(%v)", tt.input)
		})
	}
}

// ---------------------------
// | Output of the benchmark |
// ---------------------------
//
// BenchmarkPathsReducer_ReducePath
// BenchmarkPathsReducer_ReducePath/proc_1
// BenchmarkPathsReducer_ReducePath/proc_1-16         	  652135	      2072 ns/op	     555 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/proc_2
// BenchmarkPathsReducer_ReducePath/proc_2-16         	  724998	      1690 ns/op	     549 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/proc_3
// BenchmarkPathsReducer_ReducePath/proc_3-16         	  618006	      1974 ns/op	      48 B/op	       1 allocs/op
// BenchmarkPathsReducer_ReducePath/proc_4
// BenchmarkPathsReducer_ReducePath/proc_4-16         	  654654	      2148 ns/op	     556 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/proc_task_1
// BenchmarkPathsReducer_ReducePath/proc_task_1-16    	  481893	      2703 ns/op	     839 B/op	       6 allocs/op
// BenchmarkPathsReducer_ReducePath/proc_task_1#01
// BenchmarkPathsReducer_ReducePath/proc_task_1#01-16 	  334278	      3932 ns/op	     563 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/cgroup_1
// BenchmarkPathsReducer_ReducePath/cgroup_1-16       	  102973	     11161 ns/op	    1012 B/op	       6 allocs/op
// BenchmarkPathsReducer_ReducePath/proc_cgroup_1
// BenchmarkPathsReducer_ReducePath/proc_cgroup_1-16  	   94389	     12676 ns/op	    1417 B/op	       8 allocs/op
// BenchmarkPathsReducer_ReducePath/container_id_1
// BenchmarkPathsReducer_ReducePath/container_id_1-16 	  173617	      6567 ns/op	     580 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/block_device_1
// BenchmarkPathsReducer_ReducePath/block_device_1-16 	  403512	      3135 ns/op	     595 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/block_device_2
// BenchmarkPathsReducer_ReducePath/block_device_2-16 	 1000000	      1135 ns/op	     565 B/op	       4 allocs/op
// BenchmarkPathsReducer_ReducePath/k8s_service_account_secret_1
// BenchmarkPathsReducer_ReducePath/k8s_service_account_secret_1-16         	  478562	      2477 ns/op	     595 B/op	       4 allocs/op

func BenchmarkPathsReducer_ReducePath(b *testing.B) {
	r := NewPathsReducer()

	for _, tt := range tests {
		b.Run(tt.name, func(caseB *testing.B) {
			for i := 0; i < caseB.N; i++ {
				r.ReducePath(tt.input.path, tt.input.fileEvent, tt.input.processNode)
			}
		})
	}
}
