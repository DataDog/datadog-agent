// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taglist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTagList(t *testing.T) {
	list := NewTagList()
	require.NotNil(t, list)
	require.NotNil(t, list.lowCardTags)
	require.NotNil(t, list.highCardTags)
	low, orchestrator, high, standard := list.Compute()
	require.NotNil(t, low)
	require.Empty(t, low)
	require.NotNil(t, orchestrator)
	require.Empty(t, orchestrator)
	require.NotNil(t, high)
	require.Empty(t, high)
	require.NotNil(t, standard)
	require.Empty(t, standard)
}

func TestAddLow(t *testing.T) {
	list := NewTagList()
	list.splitList = map[string]string{
		"values":  ",",
		"missing": " ",
	}
	list.AddLow("foo", "bar")
	list.AddLow("faa", "baz")
	list.AddLow("empty", "")
	require.Empty(t, list.highCardTags)
	require.Len(t, list.lowCardTags, 2)
	require.True(t, list.lowCardTags["foo:bar"])
	require.True(t, list.lowCardTags["faa:baz"])

	require.False(t, list.lowCardTags["empty:"])
	require.False(t, list.lowCardTags["empty"])

	list.AddLow("values", "1")
	require.Contains(t, list.lowCardTags, "values:1")
	list.AddLow("values", "2,3")
	require.Contains(t, list.lowCardTags, "values:1")
	require.Contains(t, list.lowCardTags, "values:2")
	require.Contains(t, list.lowCardTags, "values:3")
}

func TestAddHigh(t *testing.T) {
	list := NewTagList()
	list.splitList = map[string]string{
		"values":  ",",
		"missing": " ",
	}
	list.AddHigh("foo", "bar")
	list.AddHigh("faa", "baz")
	list.AddHigh("empty", "")
	require.Empty(t, list.lowCardTags)
	require.Len(t, list.highCardTags, 2)
	require.True(t, list.highCardTags["foo:bar"])
	require.True(t, list.highCardTags["faa:baz"])

	require.False(t, list.highCardTags["empty:"])
	require.False(t, list.highCardTags["empty"])

	list.AddHigh("values", "1")
	require.Contains(t, list.highCardTags, "values:1")
	list.AddHigh("values", "2,3")
	require.Contains(t, list.highCardTags, "values:1")
	require.Contains(t, list.highCardTags, "values:2")
	require.Contains(t, list.highCardTags, "values:3")
}

func TestAddHighOrLow(t *testing.T) {
	list := NewTagList()
	list.AddAuto("foo", "bar")
	list.AddAuto("+faa", "baz")
	list.AddAuto("+", "baz")
	list.AddAuto("+empty", "")
	require.Len(t, list.lowCardTags, 1)
	require.Len(t, list.highCardTags, 1)
	require.True(t, list.lowCardTags["foo:bar"])
	require.True(t, list.highCardTags["faa:baz"])

	require.False(t, list.highCardTags["empty:"])
	require.False(t, list.highCardTags["empty"])
}

func TestAddStandard(t *testing.T) {
	list := NewTagList()
	list.splitList = map[string]string{
		"values":  ",",
		"missing": " ",
	}
	list.AddStandard("env", "dev")
	list.AddStandard("version", "foo")
	list.AddStandard("service", "")
	require.Empty(t, list.highCardTags)
	require.Empty(t, list.orchestratorCardTags)

	require.Len(t, list.standardTags, 2)
	require.True(t, list.standardTags["env:dev"])
	require.True(t, list.standardTags["version:foo"])

	require.Len(t, list.lowCardTags, 2)
	require.True(t, list.lowCardTags["env:dev"])
	require.True(t, list.lowCardTags["version:foo"])

	require.False(t, list.standardTags["service:"])
	require.False(t, list.standardTags["service"])

	list.AddStandard("values", "1")
	require.Contains(t, list.standardTags, "values:1")
	list.AddStandard("values", "2,3")
	require.Contains(t, list.standardTags, "values:1")
	require.Contains(t, list.standardTags, "values:2")
	require.Contains(t, list.standardTags, "values:3")
}

func TestCompute(t *testing.T) {
	list := NewTagList()
	list.AddHigh("foo", "bar")
	list.AddOrchestrator("pod", "redis")
	list.AddLow("faa", "baz")
	list.AddLow("low", "yes")
	list.AddAuto("+high", "yes-high")
	list.AddAuto("lowlow", "yes-low")
	list.AddAuto("empty", "")
	list.AddAuto("+empty", "")
	list.AddAuto("+", "")
	list.AddAuto("+", "empty")
	list.AddAuto("", "")
	list.AddStandard("env", "dev")

	low, orchestrator, high, standard := list.Compute()
	require.Len(t, low, 4)
	require.Contains(t, low, "faa:baz")
	require.Contains(t, low, "low:yes")
	require.Contains(t, low, "lowlow:yes-low")
	require.Contains(t, low, "env:dev")
	require.Len(t, orchestrator, 1)
	require.Contains(t, orchestrator, "pod:redis")
	require.Len(t, high, 2)
	require.Contains(t, high, "foo:bar")
	require.Contains(t, high, "high:yes-high")
	require.Len(t, standard, 1)
	require.Contains(t, standard, "env:dev")
}

func TestCopy(t *testing.T) {
	list := NewTagList()
	list.AddHigh("foo", "bar")
	list.AddOrchestrator("pod", "redis")
	list.AddLow("faa", "baz")
	list.AddLow("low", "yes")
	list.AddStandard("env", "dev")

	list2 := list.Copy()
	list2.AddHigh("foo2", "bar2")
	list2.AddOrchestrator("pod2", "redis2")
	list2.AddLow("faa2", "baz2")
	list2.AddStandard("version", "alpha")

	list3 := list.Copy()
	list3.AddHigh("foo3", "bar3")
	list3.AddOrchestrator("pod3", "redis3")
	list3.AddLow("faa3", "baz3")
	list3.AddStandard("service", "foo")

	low, orchestrator, high, standard := list.Compute()
	require.Len(t, low, 3)
	require.Contains(t, low, "faa:baz")
	require.Contains(t, low, "low:yes")
	require.Contains(t, low, "env:dev")
	require.Len(t, orchestrator, 1)
	require.Contains(t, orchestrator, "pod:redis")
	require.Len(t, high, 1)
	require.Contains(t, high, "foo:bar")
	require.Len(t, standard, 1)
	require.Contains(t, standard, "env:dev")

	low2, orchestrator2, high2, standard2 := list2.Compute()
	require.Len(t, low2, 5)
	require.Contains(t, low2, "faa:baz")
	require.Contains(t, low2, "low:yes")
	require.Contains(t, low2, "faa2:baz2")
	require.Contains(t, low2, "env:dev")
	require.Contains(t, low2, "version:alpha")
	require.Len(t, orchestrator2, 2)
	require.Contains(t, orchestrator2, "pod:redis")
	require.Contains(t, orchestrator2, "pod2:redis2")
	require.Len(t, high2, 2)
	require.Contains(t, high2, "foo:bar")
	require.Contains(t, high2, "foo2:bar2")
	require.Len(t, standard2, 2)
	require.Contains(t, standard2, "env:dev")
	require.Contains(t, standard2, "version:alpha")

	low3, orchestrator3, high3, standard3 := list3.Compute()
	require.Len(t, low3, 5)
	require.Contains(t, low3, "faa:baz")
	require.Contains(t, low3, "low:yes")
	require.Contains(t, low3, "faa3:baz3")
	require.Contains(t, low3, "env:dev")
	require.Contains(t, low3, "service:foo")
	require.Len(t, orchestrator3, 2)
	require.Contains(t, orchestrator3, "pod:redis")
	require.Contains(t, orchestrator3, "pod3:redis3")
	require.Len(t, high3, 2)
	require.Contains(t, high3, "foo:bar")
	require.Contains(t, high3, "foo3:bar3")
	require.Len(t, standard3, 2)
	require.Contains(t, standard3, "env:dev")
	require.Contains(t, standard3, "service:foo")
}
