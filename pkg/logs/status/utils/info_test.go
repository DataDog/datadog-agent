// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoRegistryReplace(t *testing.T) {

	reg := NewInfoRegistry()
	info1 := NewCountInfo("1")
	info1.Add(1)

	reg.Register(info1)

	all := reg.All()

	assert.Equal(t, "1", all[0].InfoKey())
	assert.Equal(t, "1", all[0].Info()[0])

	info2 := NewCountInfo("1")
	info2.Add(10)
	reg.Register(info2)

	all = reg.All()

	assert.Equal(t, "1", all[0].InfoKey())
	assert.Equal(t, "10", all[0].Info()[0])
}

// fakeVerboseInfo is a minimal VerboseInfoProvider for exercising RenderedVerbose.
type fakeVerboseInfo struct {
	key     string
	info    []string
	verbose bool
}

func (f *fakeVerboseInfo) InfoKey() string { return f.key }
func (f *fakeVerboseInfo) Info() []string  { return f.info }
func (f *fakeVerboseInfo) IsVerbose() bool { return f.verbose }

func TestRenderedVerboseSkipsVerboseProviderWhenNotVerbose(t *testing.T) {
	reg := NewInfoRegistry()
	reg.Register(&fakeVerboseInfo{key: "Verbose Only", info: []string{"a"}, verbose: true})

	assert.Empty(t, reg.RenderedVerbose(false))
}

func TestRenderedVerboseIncludesVerboseProviderWhenVerbose(t *testing.T) {
	reg := NewInfoRegistry()
	reg.Register(&fakeVerboseInfo{key: "Verbose Only", info: []string{"a"}, verbose: true})

	rendered := reg.RenderedVerbose(true)
	assert.Equal(t, map[string][]string{"Verbose Only": {"a"}}, rendered)
}

func TestRenderedVerboseAlwaysIncludesPlainProvider(t *testing.T) {
	reg := NewInfoRegistry()
	info := NewCountInfo("Plain")
	info.Add(5)
	reg.Register(info)

	assert.Contains(t, reg.RenderedVerbose(false), "Plain")
	assert.Contains(t, reg.RenderedVerbose(true), "Plain")
}

func TestRenderedVerboseSkipsProvidersRenderingNothing(t *testing.T) {
	reg := NewInfoRegistry()
	reg.Register(&fakeVerboseInfo{key: "Empty Verbose", info: nil, verbose: true})
	reg.Register(&fakeVerboseInfo{key: "Empty Non Verbose", info: nil, verbose: false})

	assert.Empty(t, reg.RenderedVerbose(false))
	assert.Empty(t, reg.RenderedVerbose(true))
}

// TestRenderedUnchanged is a regression test: Rendered() must keep its pre-VerboseInfoProvider
// behavior of including plain providers and skipping ones that render nothing.
func TestRenderedUnchanged(t *testing.T) {
	reg := NewInfoRegistry()
	info1 := NewCountInfo("HasValue")
	info1.Add(3)
	reg.Register(info1)
	reg.Register(NewMappedInfo("Empty"))

	assert.Equal(t, map[string][]string{"HasValue": {"3"}}, reg.Rendered())
}

// TestRenderedSkipsVerboseOnlyProviders pins Rendered() as a thin wrapper over
// RenderedVerbose(false), so verbose-only providers never appear in the non-verbose view.
func TestRenderedSkipsVerboseOnlyProviders(t *testing.T) {
	reg := NewInfoRegistry()
	reg.Register(&fakeVerboseInfo{key: "Verbose Only", info: []string{"a"}, verbose: true})

	assert.Empty(t, reg.Rendered())
}
