// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype_test

import (
	"errors"
	"flag"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype/gotypeprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

var rewrite = flag.Bool("rewrite", func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}(), "rewrite the test files")

const snapshotDir = "testdata/snapshot"

func TestGoTypeSnapshot(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	progs := testprogs.MustGetPrograms(t)
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					runGoTypeSnapshot(t, cfg, prog)
				})
			}
		})
	}
}

func runGoTypeSnapshot(t *testing.T, cfg testprogs.Config, prog string) {
	binPath := testprogs.MustGetBinary(t, prog, cfg)
	mef, err := object.OpenMMappingElfFile(binPath)
	require.NoError(t, err)
	defer func() {
		err = errors.Join(err, mef.Close())
		require.NoError(t, err)
	}()

	table, err := gotype.NewTable(mef)
	require.NoError(t, err)
	defer func() {
		err = errors.Join(err, table.Close())
		require.NoError(t, err)
	}()

	tlSection := mef.Section(".typelink")
	require.NotNil(t, tlSection)
	tlMap, err := mef.SectionData(tlSection)
	require.NoError(t, err)
	defer func() { err = errors.Join(err, tlMap.Close()); require.NoError(t, err) }()

	tl := gotype.ParseTypeLinks(tlMap.Data())
	// Make sure we can walk all the types.
	entries, errs := gotypeprinter.WalkTypes(table, slices.Collect(tl.TypeIDs()))
	require.Empty(t, errs)

	// Only actually serialize the types that are in the main package or in
	// our testprogs module.
	entries = slices.DeleteFunc(entries, func(t gotype.Type) bool {
		pkg := t.PkgPath().UnsafeName()
		return pkg != "main" && !strings.HasPrefix(
			pkg, "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs",
		)
	})

	slices.SortFunc(entries, gotypeprinter.CompareType)
	marshaled, err := gotypeprinter.TypesToYAML(table, entries)
	require.NoError(t, err)

	if *rewrite {
		_ = os.MkdirAll(snapshotDir, 0o755)
		outputFile := path.Join(snapshotDir, prog+"."+cfg.String()+".yaml")
		tmpFile, err := os.CreateTemp(snapshotDir, "gotype.yaml")
		require.NoError(t, err)
		name := tmpFile.Name()
		_, err = tmpFile.Write(marshaled)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())
		require.NoError(t, os.Rename(name, outputFile))
	} else {
		outputFile := path.Join(snapshotDir, prog+"."+cfg.String()+".yaml")
		expected, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		require.YAMLEq(t, string(expected), string(marshaled))
	}
}
