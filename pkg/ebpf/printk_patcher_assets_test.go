// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"os"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestPatchPrintkAllAssets(t *testing.T) {
	// Debug objects are not staged with the regular ones, so they come from
	// the //pkg/ebpf:ebpf_test data.
	assets := strings.Fields(os.Getenv("PRINTK_PATCHER_TEST_ASSETS"))
	if len(assets) == 0 {
		t.Skip("PRINTK_PATCHER_TEST_ASSETS is not set, run bazel test //pkg/ebpf:ebpf_test")
	}
	totalPatches := 0

	for _, asset := range assets {
		path, err := runfiles.Rlocation(asset)
		require.NoError(t, err)
		t.Run(asset, func(t *testing.T) {
			log.Tracef("Testing program %s, path: %s", asset, path)
			spec, err := ebpf.LoadCollectionSpec(path)
			require.NoError(t, err)

			for _, prog := range spec.Programs {
				t.Run(prog.Name, func(t *testing.T) {
					patcher := newPrintkPatcher(prog)
					patches, err := patcher.patch()
					require.NoError(t, err)
					totalPatches += patches
				})
			}
		})
	}

	// Some programs might not have log_debug calls, but at least one should
	require.NotZero(t, totalPatches)
}
