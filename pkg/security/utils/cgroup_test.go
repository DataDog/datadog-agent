// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/archive"
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
		{
			name:          "relative-path",
			cgroupContent: `0::/../../../../kuberuntime.slice/containerd.service`,
			error:         false,
			containerID:   "",
			flags:         containerutils.CGroupFlags(containerutils.CGroupManagerSystemd) | containerutils.SystemdService,
			path:          "/../../../../kuberuntime.slice/containerd.service",
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
			err := parseProcControlGroupsData([]byte(test.cgroupContent), func(id, ctrl, path string) (bool, error) {
				if path == "/" {
					return false, nil
				} else if ctrl != "" && !strings.HasPrefix(ctrl, "name=") {
					return false, nil
				}
				cgroup, err := makeControlGroup(id, ctrl, path)
				if err != nil {
					return false, err
				}

				containerID, flags = cgroup.GetContainerContext()
				cgroupContext.CGroupID = containerutils.CGroupID(cgroup.Path)
				cgroupContext.CGroupFlags = flags
				cgroupPath = path
				return true, nil
			})

			assert.Equal(t, test.error, err != nil)
			assert.Equal(t, containerutils.ContainerID(test.containerID), containerID)
			assert.Equal(t, test.flags, flags)
			assert.Equal(t, test.path, cgroupPath)
		})
	}
}

func untar(t *testing.T, tarxzArchive string, destinationDir string) {
	// extract all the regularfiles
	err := archive.TarXZExtractAll(tarxzArchive, destinationDir)
	assert.NoError(t, err)

	// untar symlink
	archive.WalkTarXZArchive(tarxzArchive, func(_ *tar.Reader, hdr *tar.Header) error {
		if hdr.Typeflag == tar.TypeSymlink {
			name := filepath.Join(destinationDir, hdr.Name)

			os.Symlink(hdr.Linkname, name)
		}

		return nil
	})
}

func TestCGroupFS(t *testing.T) {
	tempDir := t.TempDir()

	hostProc := filepath.Join(tempDir, "proc")

	os.Setenv("HOST_PROC", hostProc)
	os.Setenv("HOST_SYS", filepath.Join(tempDir, "sys"))

	t.Run("cgroupv2", func(t *testing.T) {
		defer os.RemoveAll(tempDir)

		untar(t, "testdata/cgroupv2.tar.xz", tempDir)

		cfs := newCGroupFS()
		cfs.cGroupMountPoints = []string{
			filepath.Join(tempDir, "sys/fs/cgroup"),
		}
		cfs.detectCurrentCgroupPath(GetpidFrom(hostProc), GetpidFrom(hostProc))

		t.Run("find-cgroup-context-ko", func(t *testing.T) {
			_, _, _, err := cfs.FindCGroupContext(567, 567)
			assert.Error(t, err)
		})

		t.Run("detect-current-cgroup-path", func(t *testing.T) {
			expectedCgroupPath := filepath.Join(tempDir, "sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podf444bc33_254d_3cba_c5e8_ce94f864b774.slice/cri-containerd-6c70abd66a591fe3d997d8b1a0a4df08d35f6b2cd4a551b514533d27d7086e37.scope")
			assert.Equal(t, expectedCgroupPath, cfs.GetRootCGroupPath())
		})

		t.Run("find-cgroup-context-ok", func(t *testing.T) {
			expectedCgroupPath := filepath.Join(tempDir, "sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podf099a5b1_192b_4df6_b6d2_aa0b366fc2f1.slice/cri-containerd-b82e1003daa43c88a8f2ee5369e1a6905db37e28d29d61e189d176aea52b2b17.scope")

			containerID, cgroupContext, cgroupPath, err := cfs.FindCGroupContext(600894, 600894)
			assert.Equal(t, containerutils.CGroupID(expectedCgroupPath), cgroupContext.CGroupID)
			assert.Equal(t, containerutils.ContainerID("b82e1003daa43c88a8f2ee5369e1a6905db37e28d29d61e189d176aea52b2b17"), containerID)
			assert.NoError(t, err)
			assert.Equal(t, expectedCgroupPath, cgroupPath)
		})
	})

	t.Run("cgroupv1", func(t *testing.T) {
		defer os.RemoveAll(tempDir)

		untar(t, "testdata/cgroupv1.tar.xz", tempDir)

		cfs := newCGroupFS()
		cfs.cGroupMountPoints = []string{
			filepath.Join(tempDir, "sys/fs/cgroup"),
		}

		cfs.detectCurrentCgroupPath(GetpidFrom(hostProc), GetpidFrom(hostProc))

		t.Run("find-cgroup-context-ko", func(t *testing.T) {
			_, _, _, err := cfs.FindCGroupContext(567, 567)
			assert.Error(t, err)
		})

		t.Run("detect-current-cgroup-path", func(t *testing.T) {
			expectedCgroupPath := filepath.Join(tempDir, "sys/fs/cgroup/pids/docker/027b54806f2263be225a934f5f083fd6ab718ddae9aedf441195c835ebf7f17d")
			assert.Equal(t, expectedCgroupPath, cfs.GetRootCGroupPath())
		})

		t.Run("find-cgroup-context-ok", func(t *testing.T) {
			expectedCgroupPath := filepath.Join(tempDir, "sys/fs/cgroup/pids/docker/d308fe417b11e128c3b42d910f3b3df6f778439edba9ab600e62dfeb5631a46f")

			containerID, cgroupContext, cgroupPath, err := cfs.FindCGroupContext(18865, 18865)
			assert.Equal(t, containerutils.CGroupID(expectedCgroupPath), cgroupContext.CGroupID)
			assert.Equal(t, containerutils.ContainerID("d308fe417b11e128c3b42d910f3b3df6f778439edba9ab600e62dfeb5631a46f"), containerID)
			assert.NoError(t, err)
			assert.Equal(t, expectedCgroupPath, cgroupPath)
		})
	})
}
