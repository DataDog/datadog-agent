// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestReaderV1(t *testing.T) {
	fakeFsPath := t.TempDir()
	paths := []string{
		"kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e.slice/crio-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40.scope",
		"kubepods/kubepods-besteffort/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e/2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41",
		"system.slice/run-containerd-io.containerd.runtime.v2.task-k8s.io-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42-rootfs.mount",
		"kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podc704ef4c297ab11032b83ce52cbfc87b.slice/cri-containerd-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42.scope",
		"libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2/system.slice/var-lib-docker-containers-1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503-mounts-shm.mount",
		"libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda2acd1bccd50fd7790183537181f658e.slice/docker-1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503.scope",
		"kubepods/pod821ad831-6a9a-4970-b623-8cb43cd3462d/f246d96ff6bd76f65c4a687ce17812a8189b735fd6ed643165db2c61e19bc31e",
		// PCF/Garden
		"system.slice/garden.service/garden/016b5740-120b-42b7-562f-3c62",
		"system.slice/garden.service/garden/c034152c-80b0-4a44-70df-5c61-liveness-healthcheck-0",
	}

	for _, p := range paths {
		finalPath := filepath.Join(fakeFsPath, defaultBaseController, p)
		assert.NoErrorf(t, os.MkdirAll(finalPath, 0o750), "impossible to create temp directory '%s'", finalPath)
	}

	fakeMountPoints := map[string]string{
		defaultBaseController: filepath.Join(fakeFsPath, defaultBaseController),
	}

	r, err := newReaderV1("", fakeMountPoints, defaultBaseController, ContainerFilter)
	r.pidMapper = nil
	assert.NoError(t, err)
	assert.NotNil(t, r)

	cgroups, err := r.parseCgroups()
	assert.NoError(t, err)
	assert.Empty(t, cmp.Diff(map[string]Cgroup{
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40": newCgroupV1("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40", paths[0], fakeMountPoints, r.pidMapper),
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41": newCgroupV1("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41", paths[1], fakeMountPoints, r.pidMapper),
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42": newCgroupV1("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42", paths[3], fakeMountPoints, r.pidMapper),
		"1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503": newCgroupV1("1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503", paths[5], fakeMountPoints, r.pidMapper),
		"6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2": newCgroupV1("6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2", "libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2", fakeMountPoints, r.pidMapper),
		"f246d96ff6bd76f65c4a687ce17812a8189b735fd6ed643165db2c61e19bc31e": newCgroupV1("f246d96ff6bd76f65c4a687ce17812a8189b735fd6ed643165db2c61e19bc31e", paths[6], fakeMountPoints, r.pidMapper),
		"016b5740-120b-42b7-562f-3c62":                                     newCgroupV1("016b5740-120b-42b7-562f-3c62", paths[7], fakeMountPoints, r.pidMapper),
	}, cgroups, cmp.AllowUnexported(cgroupV1{})))
}
