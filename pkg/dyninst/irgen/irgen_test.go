// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"debug/dwarf"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestCompileUnitFromNameCases(t *testing.T) {
	type testCase struct {
		name string
		want string
	}

	testCases := []testCase{
		{
			"github.com/DataDog/datadog-agent/pkg/dyninst/irgen.Foo",
			"github.com/DataDog/datadog-agent/pkg/dyninst/irgen",
		},
		{
			"a/b.Foo.Bar.Baz",
			"a/b",
		},
		{
			"github.com/pkg/errors.(*withStack).Format",
			"github.com/pkg/errors",
		},
		{
			"int",
			"runtime",
		},
		{
			"int",
			"runtime",
		},

		{
			"sync/atomic.(*Pointer[go.shape.struct { gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.point gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.statsPoint; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.kafkaOffset gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.kafkaOffset; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.typ gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.pointType; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.queuePos int64 }]).Swap",
			"sync/atomic",
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case_%d:%s", i, tc.name), func(t *testing.T) {
			require.Equal(t, tc.want, compileUnitFromName(tc.name))
		})
	}

}

func TestCompileUnitFromNameDwarf(t *testing.T) {
	binary := testutil.BuildSampleService(t)

	elf, err := safeelf.Open(binary)
	require.NoError(t, err)
	d, err := elf.DWARF()
	require.NoError(t, err)

	reader := d.Reader()

	var currentCompileUnit string
	checkSubprogram := func(entry *dwarf.Entry) {
		subprogramName, ok := entry.Val(dwarf.AttrName).(string)
		if !ok {
			return
		}
		if strings.HasPrefix(subprogramName, "type:.eq.") ||
			strings.HasPrefix(subprogramName, "sync/atomic.(*Pointer") {
			return
		}
		compileUnit := compileUnitFromName(subprogramName)
		require.Equal(t,
			currentCompileUnit, compileUnit,
			"subprogram %s has wrong compile unit: %s != %s: %v (%v)",
			subprogramName, currentCompileUnit, compileUnit, entry.Offset,
			entry.Field,
		)
	}
	for {
		entry, err := reader.Next()
		if err != nil {
			break
		}
		if entry == nil {
			break
		}
		if entry.Tag == dwarf.TagCompileUnit {
			currentCompileUnit = entry.Val(dwarf.AttrName).(string)
			continue
		}
		if entry.Tag == dwarf.TagSubprogram {
			checkSubprogram(entry)
		}
		if entry.Children {
			reader.SkipChildren()
		}
	}
}
