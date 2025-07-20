// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object_test

import (
	"bufio"
	"bytes"
	"debug/dwarf"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"text/tabwriter"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// This is a very basic test of loading a Go elf object file
// and that it more or less works and sanely loads dwarf.
func TestElfObject(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		binaryPath := testprogs.MustGetBinary(t, "simple", cfg)
		obj, err := object.OpenElfFile(binaryPath)
		require.NoError(t, err)
		// Assert that some symbol we expect to exist is in there.
		const targetFunction = "main.main"
		findTargetSubprogram(t, obj.DwarfData(), targetFunction)
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

type testBinary struct {
	name string
	path string
}

func parseBinaries(s string) ([]testBinary, error) {
	var binaries []testBinary
	parts := strings.Split(s, ",")
	for _, part := range parts {
		parts := strings.Split(part, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid test binary: %s", part)
		}
		binaries = append(binaries, testBinary{name: parts[0], path: parts[1]})
	}
	return binaries, nil
}
func BenchmarkLoadElfFile(b *testing.B) {
	binaries, err := parseBinaries(os.Getenv("TEST_BINARIES"))
	require.NoError(b, err, "failed to parse TEST_BINARIES")
	if len(binaries) == 0 {
		b.Skip("no binaries provided via TEST_BINARIES")
	}
	for _, binary := range binaries {
		var logOnce sync.Once
		b.Run(binary.name, func(b *testing.B) {
			ds := benchmarkLoadElfFile(b, binary.path)
			logOnce.Do(func() {
				logDebugSections(b, ds)
			})
		})
	}
}

func benchmarkLoadElfFile(b *testing.B, binaryPath string) *object.DebugSections {
	f, err := os.Open(binaryPath)
	require.NoError(b, err)
	defer f.Close()
	var ds *object.DebugSections
	b.ResetTimer()
	for b.Loop() {
		obj, err := object.OpenElfFile(binaryPath)
		require.NoError(b, err)
		ds = obj.DwarfSections()
		var total int
		for _, data := range ds.Sections() {
			total += len(data)
		}
		b.SetBytes(int64(total))
		require.NoError(b, obj.Close())
	}
	b.StopTimer()
	return ds
}

func logDebugSections(b *testing.B, ds *object.DebugSections) {
	var tab bytes.Buffer
	tw := tabwriter.NewWriter(&tab, 0, 0, 1, ' ', 0)
	total := 0
	var maxNameLen int
	for name, data := range ds.Sections() {
		if len(data) == 0 {
			continue
		}
		maxNameLen = max(maxNameLen, len(name))
		fmt.Fprintf(tw, "%s\t%d\n", name, len(data))
		total += len(data)
	}
	fmt.Fprintf(tw, "----\t----\n")
	fmt.Fprintf(tw, "\t%d\n", total)
	require.NoError(b, tw.Flush())
	scanner := bufio.NewScanner(&tab)
	for scanner.Scan() {
		b.Log(scanner.Text())
	}
	require.NoError(b, scanner.Err())
}
