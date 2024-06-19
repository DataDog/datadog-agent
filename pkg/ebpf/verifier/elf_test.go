// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package verifier

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
	"golang.org/x/exp/maps"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"

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

	fileCache := make(map[string][]string)

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
				var nonMatching []string

				// Iterate instructions in order so it's easier to debug later
				// if there are any mismatches
				insList := maps.Keys(progSourceMap)
				sort.Ints(insList)
				for _, ins := range insList {
					sl := progSourceMap[ins]
					if sl.LineInfo == "" {
						continue
					}
					hasSourceInfo = true

					// Compare the line info with the one from the actual file
					infoParts := strings.Split(sl.LineInfo, ":")
					require.Len(tt, infoParts, 2)
					line, err := strconv.Atoi(infoParts[1])
					require.NoError(tt, err)
					sourceFile := infoParts[0]

					if _, ok := fileCache[sourceFile]; !ok {
						// Read all the lines from the file
						lines, err := filesystem.ReadLines(sourceFile)
						require.NoError(tt, err, "cannot read file %s", sourceFile)
						fileCache[sourceFile] = lines
					}

					require.GreaterOrEqual(tt, line, 0, "invalid line %d, ins %d", line, ins)
					require.LessOrEqual(tt, line, len(fileCache[sourceFile]), "line %d not found in %s, ins %d", line, sourceFile, ins)
					expectedLine := fileCache[sourceFile][line-1]
					if expectedLine != sl.Line {
						nonMatching = append(nonMatching, fmt.Sprintf("ins %d, %s, expected '%s', got '%s'", ins, sl.LineInfo, expectedLine, sl.Line))
					}
				}

				maxAllowedNonMatchingPerc := 0.25
				maxAllowedNonMatching := int(float64(len(progSourceMap)) * maxAllowedNonMatchingPerc)
				require.LessOrEqual(tt, len(nonMatching), maxAllowedNonMatching, "too many non-matching lines (%d/%d) for program %s. Missing matches:\n%s", len(nonMatching), len(progSourceMap), prog, strings.Join(nonMatching, "\n- "))
				require.True(tt, hasSourceInfo, "no source info found for program %s", prog)
			}
		})
	}
}
