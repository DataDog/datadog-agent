// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package containerutils holds activitytree related files
package containerutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input  string
	output string
	flags  CGroupFlags
}

func TestFindContainerID(t *testing.T) {
	testCases := []testCase{
		{ // classic decimal
			input:  "0123456789012345678901234567890123456789012345678901234567890123",
			output: "0123456789012345678901234567890123456789012345678901234567890123",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // classic hexa
			input:  "aAbBcCdDeEfF2345678901234567890123456789012345678901234567890123",
			output: "aAbBcCdDeEfF2345678901234567890123456789012345678901234567890123",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // classic hexa as present in proc
			input:  "/docker/aAbBcCdDeEfF2345678901234567890123456789012345678901234567890123",
			output: "aAbBcCdDeEfF2345678901234567890123456789012345678901234567890123",
			flags:  CGroupFlags(CGroupManagerDocker),
		},
		{ // another proc based
			input:  "/kubepods.slice/kubepods-pod48d25824_cbe2_4fdc_9928_5bb49e05473d.slice/cri-containerd-c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad.scope",
			output: "c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad",
			flags:  CGroupFlags(CGroupManagerCRI),
		},
		{ // with prefix/suffix
			input:  "prefixaAbBcCdDeEfF2345678901234567890123456789012345678901234567890123suffix",
			output: "aAbBcCdDeEfF2345678901234567890123456789012345678901234567890123",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // path reducer test
			input:  "/var/run/docker/overlay2/47c1f1930c1831f2359c6d276912c583be1cda5924233cf273022b91763a20f7/merged/etc/passwd",
			output: "47c1f1930c1831f2359c6d276912c583be1cda5924233cf273022b91763a20f7",
			flags:  CGroupFlags(CGroupManagerDocker),
		},
		{ // GARDEN
			input:  "01234567-0123-4567-890a-bcde",
			output: "01234567-0123-4567-890a-bcde",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // GARDEN as present in proc
			input:  "/docker/01234567-0123-4567-890a-bcde",
			output: "01234567-0123-4567-890a-bcde",
			flags:  CGroupFlags(CGroupManagerDocker),
		},
		{ // incorrect container id format
			input:  "/user.slice/user-1000.slice/user@1000.service/apps.slice/apps-org.gnome.Terminal.slice/vte-spawn-f9176c6a-2a34-4ce2-86af-60d16888ed8e.scope",
			output: "",
			flags:  CGroupFlags(CGroupManagerSystemd) | SystemdScope,
		},
		{ // GARDEN with prefix / suffix
			input:  "prefix01234567-0123-4567-890a-bcdesuffix",
			output: "01234567-0123-4567-890a-bcde",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // double with first having a bad format
			input:  "0123456789aAbBcCdDeEfF0123456789-abcdef6789/0123456789aAbBcCdDeEfF0123456789-0123456789",
			output: "0123456789aAbBcCdDeEfF0123456789-0123456789",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // Docker as present in proc
			input:  "/docker/0123456789aAbBcCdDeEfF0123456789-0123456789",
			output: "0123456789aAbBcCdDeEfF0123456789-0123456789",
			flags:  CGroupFlags(CGroupManagerDocker),
		},
		{ // prefix / suffix
			input:  "prefix0123456789aAbBcCdDeEfF0123456789-0123456789suffix",
			output: "0123456789aAbBcCdDeEfF0123456789-0123456789",
			flags:  CGroupFlags(CgroupManagerUndefined),
		},
		{ // ECS
			input:  "/ecs/0123456789aAbBcCdDeEfF0123456789/0123456789aAbBcCdDeEfF0123456789-012345678",
			output: "0123456789aAbBcCdDeEfF0123456789-012345678",
			flags:  CGroupFlags(CGroupManagerECS),
		},
		{ // EKS
			input:  "/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981",
			output: "7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981",
			flags:  CGroupFlags(CGroupManagerECS),
		},
		{ // containerd
			input:  "/kubepods/besteffort/pod8bbdd97b-f902-4e16-8235-4ac18307cef6/09d3f62464b8761e9106350bacc609deb0dc639403888bdf2112033cb30e1bf6",
			output: "09d3f62464b8761e9106350bacc609deb0dc639403888bdf2112033cb30e1bf6",
			flags:  CGroupFlags(CGroupManagerCRI),
		},
	}

	for _, test := range testCases {
		containerID, flags := FindContainerID(CGroupID(test.input))
		assert.Equal(t, test.output, string(containerID))
		assert.Equal(t, test.flags, flags, "wrong flags for container %s", containerID)
		assert.Equal(t, test.flags.GetCGroupManager(), flags.GetCGroupManager(), "wrong flags for container %s", containerID)
	}
}
