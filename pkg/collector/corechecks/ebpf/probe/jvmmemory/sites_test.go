// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/usdt"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func hotspotTestProbe(t *testing.T, name, args string, offset uint64) usdt.Probe {
	t.Helper()
	spec, err := usdt.ParseArgs(safeelf.EM_X86_64, args)
	require.NoError(t, err)
	return usdt.Probe{Provider: hotspotProvider, Name: name, Args: args, FileOffset: offset, Spec: spec}
}

func TestSelectHotSpotSites(t *testing.T) {
	// Temurin 21 x86-64 sites
	begin1 := hotspotTestProbe(t, "gc__begin", "1@$1", 0x100)
	begin2 := hotspotTestProbe(t, "gc__begin", "1@%dil", 0x200)
	end := hotspotTestProbe(t, "gc__end", "", 0x300)
	pool := hotspotTestProbe(t, "mem__pool__gc__end", "8@%rbx 8@%r13 8@%r14 8@%rax 8@%rcx 8@%rsi 8@%rdi 8@%r8", 0x400)

	ignored := []usdt.Probe{
		// other providers and probes
		{Provider: "libc", Name: "gc__begin", FileOffset: 0x500, Spec: begin1.Spec},
		hotspotTestProbe(t, "mem__pool__gc__begin", "8@%rbx 8@%r13 8@%r14 8@%rax 8@%rcx 8@%rsi 8@%rdi 8@%r8", 0x600),
		// unusable sites
		{Provider: hotspotProvider, Name: "gc__end", FileOffset: 0x700, Err: errors.New("not on a nop instruction")},
		{Provider: hotspotProvider, Name: "gc__end", FileOffset: 0x800, Semaphore: 0x1234, SemaphoreFileOffset: 0x234},
		// missing arguments
		hotspotTestProbe(t, "gc__begin", "", 0x900),
		hotspotTestProbe(t, "mem__pool__gc__end", "8@%rbx 8@%r13", 0xa00),
	}

	sites, err := selectHotSpotSites(append([]usdt.Probe{begin1, begin2, end, pool}, ignored...))
	require.NoError(t, err)
	assert.Equal(t, []hotspotSite{
		{name: "gc__begin", probe: hotspotProbes["gc__begin"], offset: 0x100, spec: begin1.Spec},
		{name: "gc__begin", probe: hotspotProbes["gc__begin"], offset: 0x200, spec: begin2.Spec},
		{name: "gc__end", probe: hotspotProbes["gc__end"], offset: 0x300, spec: end.Spec},
		{name: "mem__pool__gc__end", probe: hotspotProbes["mem__pool__gc__end"], offset: 0x400, spec: pool.Spec},
	}, sites)

	_, err = selectHotSpotSites(ignored)
	assert.ErrorIs(t, err, errNoHotSpotProbes)
	_, err = selectHotSpotSites(nil)
	assert.ErrorIs(t, err, errNoHotSpotProbes)

	tooMany := make([]usdt.Probe, maxSitesPerFile+1)
	for i := range tooMany {
		tooMany[i] = end
	}
	_, err = selectHotSpotSites(tooMany)
	assert.ErrorIs(t, err, errTooManySites)
}

func filePath(t *testing.T, path string) utils.FilePath {
	t.Helper()
	id, err := utils.NewPathIdentifier(path)
	require.NoError(t, err)
	return utils.FilePath{HostPath: path, ID: id}
}

func TestProducesData(t *testing.T) {
	for _, tc := range []struct {
		attached []string
		expected bool
	}{
		{attached: nil, expected: false},
		{attached: []string{"gc__end"}, expected: false},
		{attached: []string{"gc__begin"}, expected: false},
		{attached: []string{"gc__begin", "gc__end"}, expected: true},
		{attached: []string{"mem__pool__gc__end"}, expected: true},
		{attached: []string{"gc__end", "mem__pool__gc__end"}, expected: true},
	} {
		attached := make(map[string]bool)
		for _, name := range tc.attached {
			attached[name] = true
		}
		assert.Equal(t, tc.expected, producesData(attached), tc.attached)
	}
}

func TestOpenLibJVM(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "libjvm.so")
	require.NoError(t, os.WriteFile(lib, []byte("not an ELF file"), 0o644))
	other := filepath.Join(dir, "other.so")
	require.NoError(t, os.WriteFile(other, []byte("another file"), 0o644))

	fp := filePath(t, lib)
	f, err := openLibJVM(fp)
	require.NoError(t, err)
	_, err = readHotSpotSites(f, safeelf.EM_X86_64)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, errNoHotSpotProbes)
	require.NoError(t, f.Close())

	// the file was replaced since the registry looked it up
	require.NoError(t, os.Rename(other, lib))
	_, err = openLibJVM(fp)
	assert.ErrorIs(t, err, utils.ErrEnvironment)
	assert.ErrorIs(t, err, errRetryLater)

	// symbolic links and non-regular files are rejected
	link := filepath.Join(dir, "link.so")
	require.NoError(t, os.Symlink(lib, link))
	_, err = openLibJVM(utils.FilePath{HostPath: link, ID: filePath(t, lib).ID})
	// the registry identified the target of the link: it must not be blocked
	assert.ErrorIs(t, err, utils.ErrEnvironment)
	_, err = openLibJVM(filePath(t, dir))
	assert.Error(t, err)

	// gone
	require.NoError(t, os.Remove(lib))
	_, err = openLibJVM(fp)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.NotErrorIs(t, err, errRetryLater)

	// resource exhaustion is retried at the next scan
	err = openError("open", lib, unix.EMFILE)
	assert.ErrorIs(t, err, unix.EMFILE)
	assert.ErrorIs(t, err, utils.ErrEnvironment)
	assert.ErrorIs(t, err, errRetryLater)
}

func TestReadHotSpotSitesWithoutProbes(t *testing.T) {
	// the test binary is an ELF file of the host architecture without HotSpot probes
	exe, err := os.Executable()
	require.NoError(t, err)
	fp := filePath(t, exe)
	f, err := openLibJVM(fp)
	require.NoError(t, err)
	defer f.Close()

	hostMachine, ok := usdt.HostMachine()
	if !ok {
		t.Skip("unsupported architecture")
	}
	_, err = readHotSpotSites(f, hostMachine)
	assert.ErrorIs(t, err, errNoHotSpotProbes)

	foreign := safeelf.EM_X86_64
	if hostMachine == safeelf.EM_X86_64 {
		foreign = safeelf.EM_AARCH64
	}
	_, err = readHotSpotSites(f, foreign)
	assert.ErrorIs(t, err, errForeignArch)
}
