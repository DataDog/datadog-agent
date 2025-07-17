// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

func TestCGroupvParseLine(t *testing.T) {
	line := `5:cpu,cpuacct:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope`
	id, ctrl, path, err := parseCgroupLine(line)

	assert.Nil(t, err)
	assert.Equal(t, "5", id)
	assert.Equal(t, "cpu,cpuacct", ctrl)
	assert.Equal(t, "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope", path)
}

type testCgroup struct {
	name          string
	cgroupContent string
	error         bool
	containerID   string
	flags         containerutils.CGroupFlags
	path          string
}

func TestCGroup(t *testing.T) {
	testsCgroup := []testCgroup{
		{
			name: "cgroupv1-cri",
			cgroupContent: `13:blkio:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
12:memory:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
11:misc:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
10:pids:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
9:hugetlb:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
8:rdma:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
7:perf_event:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
6:cpuset:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
5:cpu,cpuacct:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
4:net_cls,net_prio:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
3:freezer:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
2:devices:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
1:name=systemd:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
0::/
`,
			error:       false,
			containerID: "e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerCRI),
			path:        "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope",
		},
		{
			name: "cgroupv1-docker",
			cgroupContent: `13:memory:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
12:hugetlb:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
11:misc:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
10:blkio:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
9:rdma:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
8:perf_event:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
7:cpuset:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
6:pids:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
5:cpu,cpuacct:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
4:freezer:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
3:devices:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
2:net_cls,net_prio:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
1:name=systemd:/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
0::/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182
`,
			error:       false,
			containerID: "99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerDocker),
			path:        "/docker/99d24a208bd5b9c9663e18c34e4bd793536f062d8299a5cca0e718994abd9182",
		},
		{
			name: "cgroupv1-systemd-service",
			cgroupContent: `13:memory:/system.slice/cups.service
12:hugetlb:/
11:misc:/
10:blkio:/system.slice/cups.service
9:rdma:/
8:perf_event:/
7:cpuset:/
6:pids:/system.slice/cups.service
5:cpu,cpuacct:/system.slice/cups.service
4:freezer:/
3:devices:/system.slice/cups.service
2:net_cls,net_prio:/
1:name=systemd:/system.slice/cups.service
0::/system.slice/cups.service
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd) | containerutils.SystemdService,
			path:        "/system.slice/cups.service",
		},
		{
			name: "cgroupv1-systemd-subservice",
			cgroupContent: `13:memory:/user.slice/user-1000.slice/user@1000.service
12:hugetlb:/
11:misc:/
10:blkio:/user.slice
9:rdma:/
8:perf_event:/
7:cpuset:/
6:pids:/user.slice/user-1000.slice/user@1000.service
5:cpu,cpuacct:/user.slice
4:freezer:/
3:devices:/user.slice
2:net_cls,net_prio:/
1:name=systemd:/user.slice/user-1000.slice/user@1000.service/xdg-desktop-portal-gtk.service
0::/user.slice/user-1000.slice/user@1000.service/xdg-desktop-portal-gtk.service
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd) | containerutils.SystemdService,
			path:        "/user.slice/user-1000.slice/user@1000.service/xdg-desktop-portal-gtk.service",
		},
		{
			name: "cgroupv1-systemd-scope",
			cgroupContent: `13:memory:/user.slice/user-1000.slice/user@1000.service
12:hugetlb:/
11:misc:/
10:blkio:/user.slice
9:rdma:/
8:perf_event:/
7:cpuset:/
6:pids:/user.slice/user-1000.slice/user@1000.service
5:cpu,cpuacct:/user.slice
4:freezer:/
3:devices:/user.slice
2:net_cls,net_prio:/
1:name=systemd:/user.slice/user-1000.slice/user@1000.service/apps.slice/apps-org.gnome.Terminal.slice/vte-spawn-1d0750f1-4e83-4b26-81ae-e3770394b7f3.scope
0::/user.slice/user-1000.slice/user@1000.service/apps.slice/apps-org.gnome.Terminal.slice/vte-spawn-1d0750f1-4e83-4b26-81ae-e3770394b7f3.scope
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd | containerutils.CGroupManager(containerutils.SystemdScope)),
			path:        "/user.slice/user-1000.slice/user@1000.service/apps.slice/apps-org.gnome.Terminal.slice/vte-spawn-1d0750f1-4e83-4b26-81ae-e3770394b7f3.scope",
		},
		{
			name: "cgroupv1-empty",
			cgroupContent: `12:pids:/
11:devices:/
10:blkio:/
9:cpuset:/
8:perf_event:/
7:memory:/
6:freezer:/
5:hugetlb:/
4:rdma:/
3:net_cls,net_prio:/
2:cpu,cpuacct:/
1:name=systemd:/
0::/
`,
			error:       false,
			containerID: "",
			flags:       0,
			path:        "",
		},
		{
			name: "cgroupv1-pid1",
			cgroupContent: `13:memory:/init.scope
12:hugetlb:/
11:misc:/
10:blkio:/init.scope
9:rdma:/
8:perf_event:/
7:cpuset:/
6:pids:/init.scope
5:cpu,cpuacct:/init.scope
4:freezer:/
3:devices:/init.scope
2:net_cls,net_prio:/
1:name=systemd:/init.scope
0::/init.scope
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd | containerutils.CGroupManager(containerutils.SystemdScope)),
			path:        "/init.scope",
		},
		{
			name: "cgroupv2-docker",
			cgroupContent: `0::/system.slice/docker-473a28bd49fcbf3a24eb55563125720311181ee184ae9b88fc9a3fbb30031e47.scope
`,
			error:       false,
			containerID: "473a28bd49fcbf3a24eb55563125720311181ee184ae9b88fc9a3fbb30031e47",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerDocker),
			path:        "/system.slice/docker-473a28bd49fcbf3a24eb55563125720311181ee184ae9b88fc9a3fbb30031e47.scope",
		},
		{
			name: "cgroupv2-systemd-service",
			cgroupContent: `0::/system.slice/ssh.service
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd) | containerutils.SystemdService,
			path:        "/system.slice/ssh.service",
		},
		{
			name: "cgroupv2-systemd-scope",
			cgroupContent: `0::/user.slice/user-1000.slice/session-4.scope
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd | containerutils.CGroupManager(containerutils.SystemdScope)),
			path:        "/user.slice/user-1000.slice/session-4.scope",
		},
		{
			name: "cgroupv2-pid1",
			cgroupContent: `0::/init.scope
`,
			error:       false,
			containerID: "",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerSystemd | containerutils.CGroupManager(containerutils.SystemdScope)),
			path:        "/init.scope",
		},
		{
			name: "cgroupv2-empty",
			cgroupContent: `0::/
`,
			error:       false,
			containerID: "",
			flags:       0,
			path:        "",
		},
		{
			name: "fargate-eks",
			cgroupContent: `11:memory:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
10:perf_event:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
9:pids:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
8:cpuset:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
7:freezer:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
6:hugetlb:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
5:devices:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
4:cpu,cpuacct:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
3:blkio:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
2:net_cls,net_prio:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
1:name=systemd:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
`,
			error:       false,
			containerID: "7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerECS),
			path:        "/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981",
		},
		{
			name: "fargate-ecs",
			cgroupContent: `11:devices:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
10:memory:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
9:blkio:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
1:name=systemd:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770`,
			error:       false,
			containerID: "8a28a84664034325be01ca46b33d1dd3-4092616770",
			flags:       containerutils.CGroupFlags(containerutils.CGroupManagerECS),
			path:        "/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770",
		},
	}

	for _, test := range testsCgroup {
		var (
			containerID   containerutils.ContainerID
			flags         containerutils.CGroupFlags
			cgroupContext CGroupContext
			cgroupPath    string
		)

		t.Run(test.name, func(t *testing.T) {
			err := parseProcControlGroupsData([]byte(test.cgroupContent), func(id, ctrl, path string) bool {
				if path == "/" {
					return false
				} else if ctrl != "" && !strings.HasPrefix(ctrl, "name=") {
					return false
				}
				cgroup, err := makeControlGroup(id, ctrl, path)
				if err != nil {
					return false
				}

				containerID, flags = cgroup.GetContainerContext()
				cgroupContext.CGroupID = containerutils.CGroupID(cgroup.Path)
				cgroupContext.CGroupFlags = flags
				cgroupPath = path
				return true
			})

			assert.Equal(t, test.error, err != nil)
			assert.Equal(t, containerutils.ContainerID(test.containerID), containerID)
			assert.Equal(t, test.flags, flags)
			assert.Equal(t, test.path, cgroupPath)
		})
	}
}
