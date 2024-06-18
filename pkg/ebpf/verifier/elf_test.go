// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package verifier

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/exp/maps"

	"github.com/cilium/ebpf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"

	"github.com/stretchr/testify/require"
)

func TestGetSourceMap(t *testing.T) {
	objectFiles := make(map[string]string)
	directory := ddebpf.NewConfig().BPFDir
	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if strings.Contains(path, "-debug") || !strings.HasSuffix(path, ".o") {
			return nil
		}

		if _, ok := objectFiles[d.Name()]; !ok {
			objectFiles[d.Name()] = path
		}
		return nil
	})
	require.NoError(t, err)

	for name, path := range objectFiles {
		t.Run(name, func(tt *testing.T) {
			spec, err := ebpf.LoadCollectionSpec(path)
			require.NoError(tt, err)
			sourceMap, funcsPerSection, err := getSourceMap(path, spec)

			require.NoError(tt, err)
			require.NotEmpty(tt, sourceMap)
			require.NotEmpty(tt, funcsPerSection)

			for _, funcs := range funcsPerSection {
				require.NotEmpty(tt, funcs)
				for _, f := range funcs {
					require.Contains(tt, sourceMap, f)
				}
			}

			for prog, progSourceMap := range sourceMap {
				require.NotEmpty(tt, progSourceMap)

				hasSourceInfo := false
				insnList := maps.Keys(progSourceMap)
				sort.Ints(insnList)
				for _, ins := range insnList {
					sl := progSourceMap[ins]
					if sl.LineInfo != "" {
						hasSourceInfo = true
					}
				}
				require.True(tt, hasSourceInfo, "no source info found for program %s", prog)
			}
		})
	}
}
