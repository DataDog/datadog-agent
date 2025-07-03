// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object_test

import (
	"debug/dwarf"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// This is a very basic test of loading a Go elf object file
// and that it more or less works and sanely loads dwarf.
func TestElfObject(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		binaryPath := testprogs.MustGetBinary(t, "simple", cfg)
		elf, err := safeelf.Open(binaryPath)
		require.NoError(t, err)
		obj, err := object.NewElfObject(elf)
		require.NoError(t, err)
		dd, err := obj.DWARF()
		require.NoError(t, err)
		// Assert that some symbol we expect to exist is in there.
		const targetFunction = "main.main"
		findTargetSubprogram(t, dd, targetFunction)
	}
}

func findTargetSubprogram(
	t *testing.T, dd *dwarf.Data, name string,
) {
	r := dd.Reader()
	for {
		e, err := r.Next()
		require.NoError(t, err)
		if e == nil {
			t.Fatalf("failed to find %q", name)
		}
		if e.Tag != dwarf.TagSubprogram {
			continue
		}
		entryName, ok := e.Val(dwarf.AttrName).(string)
		if !ok {
			continue
		}
		if name == entryName {
			return
		}
	}
}
