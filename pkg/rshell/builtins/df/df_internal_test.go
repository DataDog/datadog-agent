// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package df

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/diskstats"
)

func TestHumanBytes_1024(t *testing.T) {
	cases := []struct {
		v    uint64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{1023, "1023"},
		{1024, "1.0K"},
		{2047, "2.0K"}, // 2047/1024 = 1.999 → 2.0
		{2048, "2.0K"},
		{10 * 1024, "10K"}, // ≥10 drops decimal
		{1024 * 1024, "1.0M"},
		{500 * 1024 * 1024, "500M"},
		{1 << 30, "1.0G"},
		{1<<40 + 1<<39, "1.5T"},
		{1 << 50, "1.0P"},
		{1 << 60, "1.0E"},
		{^uint64(0), "16E"},
		// GNU df rounds non-integer remainders up so "Used" never
		// under-reports. 1,576,960 bytes is 385 × 4 KiB blocks; GNU
		// emits "1.6M" rather than the round-to-nearest "1.5M".
		{1_576_960, "1.6M"},
		// 2 KiB + 1 byte → just over 2.0K, must round up to 2.1K.
		{2*1024 + 1, "2.1K"},
		// Just under 1 MiB: rounds up at K-level to 1024K which is
		// awkward output, so promote to the next suffix and emit
		// "1.0M". Matches GNU df.
		{1<<20 - 1, "1.0M"},
		// Same promotion at every higher boundary.
		{1<<30 - 1, "1.0G"},
		{1<<40 - 1, "1.0T"},
		// Above 10K we drop the decimal — make sure the promotion
		// path through the >=10 branch also works. 9.7M = 10172724
		// rounds-up to 10M (no decimal). 10239*1024 = 10484736 is
		// just under 10M and rounds to 10M too, but 10240*1024 - 1
		// = 10485759 (just under 10M) similarly rounds to 10M.
		{10485759, "10M"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, humanBytes(c.v, 1024), "v=%d", c.v)
	}
}

func TestHumanBytes_1000(t *testing.T) {
	// SI mode uses lowercase "k" for the kilo suffix (matches GNU
	// df). Other suffixes stay uppercase.
	cases := []struct {
		v    uint64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{25_000, "25k"}, // Codex's scenario: small mount in SI mode
		{1_000_000, "1.0M"},
		{1_000_000_000, "1.0G"},
		{1_000_000_000_000, "1.0T"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, humanBytes(c.v, 1000), "v=%d", c.v)
	}
}

func TestPercentUsed(t *testing.T) {
	cases := []struct {
		used, free uint64
		want       string
	}{
		{0, 0, "-"},
		{0, 100, "0%"},
		{1, 99, "1%"},
		{50, 50, "50%"},
		{99, 1, "99%"},
		{1, 0, "100%"},
		// Round-up semantics: any non-zero remainder bumps the
		// percentage up.
		{1, 999, "1%"},
		{1, 9_999_999, "1%"},
		// Overflow guard: used * 100 would wrap; the implementation
		// right-shifts both sides until the multiplication fits.
		// used == free → 50%, even at extreme magnitudes.
		{^uint64(0) / 50, ^uint64(0) / 50, "50%"},
		// Used > free at extreme magnitudes → 100%.
		{^uint64(0), 1, "100%"},
		{^uint64(0) / 2, ^uint64(0) / 4, "67%"},
		// Sum-overflow case: with --total, each accumulator can
		// saturate to MaxUint64. Saturating the denominator would
		// misreport equal halves as 100%; the scale-down step
		// preserves the true ratio.
		{^uint64(0), ^uint64(0), "50%"},
		{^uint64(0), ^uint64(0) / 2, "67%"}, // used=MaxU, free=MaxU/2 → 2/3
		// Tiny-used / huge-available: the step-1 right-shift drops
		// used from 1 to 0, but the "any non-zero usage rounds up
		// to ≥1%" contract must still hold. Without the
		// nonzero-usage bump, this case under-reports as "0%".
		{1, ^uint64(0), "1%"},
		{1, ^uint64(0) - 1, "1%"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, percentUsed(c.used, c.free), "u=%d f=%d", c.used, c.free)
	}
}

func TestSaturatingAdd(t *testing.T) {
	maxU := ^uint64(0)
	assert.Equal(t, uint64(3), saturatingAdd(1, 2))
	assert.Equal(t, maxU, saturatingAdd(maxU, 0))
	assert.Equal(t, maxU, saturatingAdd(0, maxU))
	assert.Equal(t, maxU, saturatingAdd(maxU, 1))
	assert.Equal(t, maxU, saturatingAdd(maxU, maxU))
	assert.Equal(t, maxU-1, saturatingAdd(maxU/2, maxU/2))
}

func TestFormatCount(t *testing.T) {
	// Inode mode in fixed-block (POSIX / -k) units → raw integers.
	assert.Equal(t, "12345", formatCount(12345, unitsK, true))

	// `df -ih` and `df -iH`: GNU scales inode counts through the same
	// suffix machinery as block counts, so 4M inodes renders as "4.0M",
	// not "4194304".
	assert.Equal(t, "4.0M", formatCount(4*1024*1024, unitsHuman1024, true))
	assert.Equal(t, "1.0G", formatCount(1_000_000_000, unitsHuman1000, true))

	// 1K block mode: rounds up to the next 1024 boundary.
	assert.Equal(t, "0", formatCount(0, unitsK, false))
	assert.Equal(t, "1", formatCount(1, unitsK, false))
	assert.Equal(t, "1", formatCount(1024, unitsK, false))
	assert.Equal(t, "2", formatCount(1025, unitsK, false))

	// Saturated max value (e.g. an overflowed grand total) must not
	// wrap to 0 — must remain a sane integer count of 1K blocks.
	assert.Equal(t, "18014398509481984", formatCount(^uint64(0), unitsK, false))

	// Human modes delegate to humanBytes. SI mode uses lowercase "k"
	// for the kilo suffix; IEC keeps uppercase "K".
	assert.Equal(t, "1.0K", formatCount(1024, unitsHuman1024, false))
	assert.Equal(t, "1.0k", formatCount(1000, unitsHuman1000, false))
}

// TestPercentUsed_NoDivByZero — every combination of zero inputs and
// extreme magnitudes must produce a finite percentage string and never
// panic. percentUsed is called from a hot per-mount loop, so a panic
// would crash the entire df invocation.
func TestPercentUsed_NoDivByZero(t *testing.T) {
	maxU := ^uint64(0)
	cases := []struct{ used, free uint64 }{
		{0, 0},
		{0, maxU},
		{maxU, 0},
		{maxU, maxU},
		{maxU, 1},
		{1, maxU},
		{maxU - 1, 1},
		{maxU / 2, maxU / 2},
		{maxU / 200, maxU / 200},
	}
	for _, c := range cases {
		// Wrap in a func so a panic in any case fails the whole test
		// with a useful message instead of taking down the suite.
		assert.NotPanics(t, func() { _ = percentUsed(c.used, c.free) },
			"u=%d f=%d", c.used, c.free)
	}
}

// Mount paths can legally contain control characters (the kernel
// stores newlines, tabs, and other bytes in mountinfo using octal
// escapes such as \012). After unescapeMountField decodes them, the
// raw byte is fed to statfs(2), but the printer must replace these
// bytes with '?' or an attacker who can mount a filesystem at a
// crafted path could inject a fake row into df output that scripts
// and AI agents parse line-by-line. Mirrors GNU coreutils df, which
// also replaces unprintable bytes with '?'.
func TestReplaceUnprintable(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"/tmp/path with space", "/tmp/path with space"},
		{"/tmp/a\nb", "/tmp/a?b"},      // newline → ?
		{"/tmp/a\tb", "/tmp/a?b"},      // tab → ?
		{"/tmp/a\rb", "/tmp/a?b"},      // CR → ?
		{"/tmp/a\x00b", "/tmp/a?b"},    // NUL → ?
		{"/tmp/a\x01b", "/tmp/a?b"},    // SOH → ?
		{"/tmp/a\x1fb", "/tmp/a?b"},    // unit separator → ?
		{"/tmp/a\x7fb", "/tmp/a?b"},    // DEL → ?
		{"/tmp/a\x20b", "/tmp/a\x20b"}, // space (0x20) preserved
		{"/tmp/a\x7eb", "/tmp/a\x7eb"}, // ~ (0x7E) preserved
		// Multiple replacements in a row
		{"\n\n\n", "???"},
		// A control char at start, middle, end
		{"\nhead", "?head"},
		{"tail\n", "tail?"},
		{"mid\ndle", "mid?dle"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, replaceUnprintable(c.in), "in=%q", c.in)
	}
}

func TestStringSet(t *testing.T) {
	assert.Nil(t, stringSet(nil))
	assert.Nil(t, stringSet([]string{}))
	got := stringSet([]string{"ext4"})
	assert.Equal(t, map[string]struct{}{"ext4": {}}, got)
	got = stringSet([]string{"ext4", "tmpfs"})
	assert.Contains(t, got, "ext4")
	assert.Contains(t, got, "tmpfs")
	// Comma-separated values are kept literal (matches GNU df, which
	// requires multiple -t flags rather than comma-splitting one).
	got = stringSet([]string{"ext4,tmpfs", "xfs"})
	assert.Contains(t, got, "ext4,tmpfs")
	assert.Contains(t, got, "xfs")
	assert.NotContains(t, got, "ext4")
	assert.NotContains(t, got, "tmpfs")
}

// keep is a small helper that runs makePreStatFilter against a fixture
// slice and returns the survivors. Mirrors what diskstats.List does
// internally between mountinfo parsing and statfs.
func keep(in []diskstats.Mount, f *flags) []diskstats.Mount {
	pred := makePreStatFilter(f)
	out := make([]diskstats.Mount, 0, len(in))
	for _, m := range in {
		if pred(m) {
			out = append(out, m)
		}
	}
	return out
}

func TestPreStatFilter_DefaultDropsPseudo(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/", FSType: "ext4", Local: true},
		{MountPoint: "/proc", FSType: "proc", Pseudo: true},
		{MountPoint: "/dev", FSType: "devtmpfs", Pseudo: true},
		{MountPoint: "/mnt/nfs", FSType: "nfs", Local: false},
	}
	out := keep(in, &flags{
		all:          ptrBool(false),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string(nil)),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 2)
	assert.Equal(t, "/", out[0].MountPoint)
	assert.Equal(t, "/mnt/nfs", out[1].MountPoint)
}

func TestPreStatFilter_AllIncludesPseudo(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/", FSType: "ext4", Local: true},
		{MountPoint: "/proc", FSType: "proc", Pseudo: true},
	}
	out := keep(in, &flags{
		all:          ptrBool(true),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string(nil)),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 2)
}

func TestPreStatFilter_LocalDropsRemote(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/", FSType: "ext4", Local: true},
		{MountPoint: "/mnt/nfs", FSType: "nfs", Local: false},
	}
	out := keep(in, &flags{
		all:          ptrBool(true),
		local:        ptrBool(true),
		includeTypes: ptrSlice([]string(nil)),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 1)
	assert.Equal(t, "/", out[0].MountPoint)
}

// `df -al` must include local pseudo mounts. GNU df treats pseudo and
// remote as independent: -a re-enables pseudo, -l drops only remote
// (NFS / CIFS / fuse.sshfs), so pseudo mounts pass when -a is set even
// alongside -l.
func TestPreStatFilter_AllPlusLocalKeepsPseudo(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/", FSType: "ext4", Local: true},
		{MountPoint: "/proc", FSType: "proc", Pseudo: true, Local: true},
		{MountPoint: "/sys", FSType: "sysfs", Pseudo: true, Local: true},
		{MountPoint: "/sys/fs/cgroup", FSType: "cgroup2", Pseudo: true, Local: true},
		{MountPoint: "/mnt/nfs", FSType: "nfs", Local: false},
	}
	out := keep(in, &flags{
		all:          ptrBool(true),
		local:        ptrBool(true),
		includeTypes: ptrSlice([]string(nil)),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 4, "-al must keep ext4 + proc + sysfs + cgroup2; only nfs drops")
	for _, m := range out {
		assert.NotEqual(t, "nfs", m.FSType)
	}
}

// -t TYPE does NOT override the default pseudo-FS suppression — it
// just narrows the type filter independently. GNU df 9.4: `df -t proc`
// exits 1 with "no file systems processed" because proc is pseudo.
// Only -a exposes pseudo filesystems. (`df -t tmpfs` works in
// production because tmpfs is not classified as pseudo in our table —
// see pseudoTypes — not because -t overrides the suppression.)
func TestPreStatFilter_TypeIncludeRespectsPseudoSuppression(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/proc", FSType: "proc", Pseudo: true, Local: true},
		{MountPoint: "/sys", FSType: "sysfs", Pseudo: true, Local: true},
	}
	// -t proc without -a: pseudo filter still drops the proc mount,
	// so the include-set match doesn't help.
	out := keep(in, &flags{
		all:          ptrBool(false),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string{"proc"}),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Empty(t, out, "-t proc alone must NOT expose pseudo proc mounts")

	// -a -t proc: -a exempts pseudo, then -t filters by type.
	out = keep(in, &flags{
		all:          ptrBool(true),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string{"proc"}),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 1)
	assert.Equal(t, "proc", out[0].FSType)
}

// Non-pseudo types are unaffected: -t TYPE on ext4/tmpfs/etc. lists
// them as expected without -a.
func TestPreStatFilter_TypeIncludeForNonPseudoWorksWithoutA(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/", FSType: "ext4", Local: true},
		{MountPoint: "/dev/shm", FSType: "tmpfs", Local: true},
		{MountPoint: "/proc", FSType: "proc", Pseudo: true, Local: true},
	}
	out := keep(in, &flags{
		all:          ptrBool(false),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string{"tmpfs"}),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 1)
	assert.Equal(t, "tmpfs", out[0].FSType)
}

// At the filter layer, exclude wins over include for the same TYPE.
// In production this configuration is rejected upstream by the
// overlappingType check before makePreStatFilter ever runs (matching
// GNU df's "both selected and excluded" error), but the unit-level
// behaviour is still exercised here so the filter's exclude-precedence
// is locked in for future callers that bypass the top-level check.
func TestPreStatFilter_TypeExcludeWinsOverIncludeOnPseudo(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/dev/shm", FSType: "tmpfs", Pseudo: true},
	}
	out := keep(in, &flags{
		all:          ptrBool(false),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string{"tmpfs"}),
		excludeTypes: ptrSlice([]string{"tmpfs"}),
	})
	assert.Empty(t, out)
}

// overlappingType returns the conflicting type, or "" when -t and -x
// are disjoint. Used by makeFlags to emit the GNU "both selected and
// excluded" error before any mounts are listed.
func TestOverlappingType(t *testing.T) {
	assert.Equal(t, "", overlappingType(nil, nil))
	assert.Equal(t, "", overlappingType([]string{"ext4"}, nil))
	assert.Equal(t, "", overlappingType(nil, []string{"ext4"}))
	assert.Equal(t, "", overlappingType([]string{"ext4"}, []string{"tmpfs"}))
	assert.Equal(t, "tmpfs", overlappingType([]string{"ext4", "tmpfs"}, []string{"tmpfs"}))
	assert.Equal(t, "tmpfs", overlappingType([]string{"tmpfs"}, []string{"ext4", "tmpfs"}))
	// Both lists name multiple overlapping types — first include match
	// is reported.
	assert.Equal(t, "ext4", overlappingType([]string{"ext4", "tmpfs"}, []string{"ext4", "tmpfs"}))
}

func TestPreStatFilter_TypeIncludeAndExclude(t *testing.T) {
	in := []diskstats.Mount{
		{MountPoint: "/a", FSType: "ext4", Local: true},
		{MountPoint: "/b", FSType: "ext4", Local: true},
		{MountPoint: "/c", FSType: "btrfs", Local: true},
		{MountPoint: "/d", FSType: "xfs", Local: true},
	}
	out := keep(in, &flags{
		all:          ptrBool(true),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string{"ext4", "xfs"}),
		excludeTypes: ptrSlice([]string(nil)),
	})
	assert.Len(t, out, 3) // both ext4 + xfs

	out = keep(in, &flags{
		all:          ptrBool(true),
		local:        ptrBool(false),
		includeTypes: ptrSlice([]string{"ext4", "xfs"}),
		excludeTypes: ptrSlice([]string{"ext4"}),
	})
	assert.Len(t, out, 1)
	assert.Equal(t, "xfs", out[0].FSType)
}

// filterMounts dedups by DevID and keeps the entry with the shortest
// mount point. Order in the input slice does not influence the choice
// of representative — matches GNU df, which on Kata containers picks
// /etc/hosts over /etc/hostname or /etc/resolv.conf even though the
// kernel reports them in arbitrary order.
func TestFilterMounts_DedupByDevicePicksShortestMountpoint(t *testing.T) {
	in := []diskstats.Mount{
		// Three bind-mounts of the same device, in non-shortest-first
		// order. The shortest mount point is /etc/hosts.
		{Source: "kataShared", DevID: "0:25", MountPoint: "/etc/resolv.conf", FSType: "9p"},
		{Source: "kataShared", DevID: "0:25", MountPoint: "/etc/hostname", FSType: "9p"},
		{Source: "kataShared", DevID: "0:25", MountPoint: "/etc/hosts", FSType: "9p"},
		// Distinct device — must pass through.
		{Source: "/dev/sda1", DevID: "8:1", MountPoint: "/", FSType: "ext4"},
	}
	out := filterMounts(append([]diskstats.Mount(nil), in...), &flags{
		all: ptrBool(false),
	})
	assert.Len(t, out, 2, "duplicate kataShared mounts collapsed to one")
	// Find the kataShared survivor and confirm it is /etc/hosts.
	for _, m := range out {
		if m.DevID == "0:25" {
			assert.Equal(t, "/etc/hosts", m.MountPoint,
				"shortest mount point of duplicates must be the representative")
		}
	}
}

// Two mounts sharing a Source string but with distinct DevIDs are NOT
// duplicates. The dedup key is device identity, not source name —
// otherwise unrelated overlay mounts (e.g. multiple Kubernetes pods
// each named "overlay") would be wrongly collapsed.
func TestFilterMounts_DistinctDeviceSameSourceNotDeduped(t *testing.T) {
	in := []diskstats.Mount{
		{Source: "overlay", DevID: "0:30", MountPoint: "/var/lib/pod-a"},
		{Source: "overlay", DevID: "0:31", MountPoint: "/var/lib/pod-b"},
	}
	out := filterMounts(append([]diskstats.Mount(nil), in...), &flags{
		all: ptrBool(false),
	})
	assert.Len(t, out, 2, "different DevIDs must not be collapsed")
}

// With -a, dedup is disabled (matches GNU df --all).
func TestFilterMounts_AllPreservesDuplicates(t *testing.T) {
	in := []diskstats.Mount{
		{Source: "overlay", DevID: "0:25", MountPoint: "/etc/hosts"},
		{Source: "overlay", DevID: "0:25", MountPoint: "/etc/hostname"},
	}
	out := filterMounts(append([]diskstats.Mount(nil), in...), &flags{
		all: ptrBool(true),
	})
	assert.Len(t, out, 2, "-a must preserve duplicates")
}

// Empty DevID disables dedup (used by platforms that do not expose a
// stable device identity, or as a graceful fallback). Mounts pass
// through untouched.
func TestFilterMounts_EmptyDevIDNotDeduped(t *testing.T) {
	in := []diskstats.Mount{
		{Source: "", DevID: "", MountPoint: "/a", FSType: "tmpfs"},
		{Source: "", DevID: "", MountPoint: "/b", FSType: "tmpfs"},
	}
	out := filterMounts(append([]diskstats.Mount(nil), in...), &flags{
		all: ptrBool(false),
	})
	assert.Len(t, out, 2)
}

func TestBuildHeader(t *testing.T) {
	// Default block mode: Filesystem 1K-blocks Used Available Use% Mounted on.
	h := buildHeader(false, false, false, unitsK)
	assert.Equal(t, []string{"Filesystem", "1K-blocks", "Used", "Available", "Use%", "Mounted on"}, h)

	// -P (block POSIX): "Capacity" replaces "Use%".
	h = buildHeader(true, false, false, unitsK)
	assert.Equal(t, []string{"Filesystem", "1024-blocks", "Used", "Available", "Capacity", "Mounted on"}, h)

	// -h: column 1 → "Size", and "Available" is compressed to "Avail"
	// to match GNU's compact human output.
	h = buildHeader(false, false, false, unitsHuman1024)
	assert.Equal(t, []string{"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted on"}, h)

	// -H: same compact "Avail" header.
	h = buildHeader(false, false, false, unitsHuman1000)
	assert.Equal(t, []string{"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted on"}, h)

	// -T: inserts Type column after Filesystem.
	h = buildHeader(false, true, false, unitsK)
	assert.Equal(t, "Type", h[1])

	// -i: inode columns.
	h = buildHeader(false, false, true, unitsK)
	assert.Equal(t, []string{"Filesystem", "Inodes", "IUsed", "IFree", "IUse%", "Mounted on"}, h)

	// -i -P: inode columns. GNU keeps "IUse%" — only the *block*
	// POSIX format substitutes "Capacity", so the inode header is
	// unchanged by -P.
	h = buildHeader(true, false, true, unitsK)
	assert.Equal(t, []string{"Filesystem", "Inodes", "IUsed", "IFree", "IUse%", "Mounted on"}, h)
	assert.NotContains(t, h, "Capacity")

	// -i -T: inode columns + Type column inserted after Filesystem.
	h = buildHeader(false, true, true, unitsK)
	assert.Equal(t, []string{"Filesystem", "Type", "Inodes", "IUsed", "IFree", "IUse%", "Mounted on"}, h)

	// -P -h: human suffix wins over the fixed-block POSIX label, so
	// "Size" + "Avail" appear even when -P is set. The percentage
	// column also drops back to "Use%" because GNU treats human mode
	// as overriding the strict POSIX block-size convention.
	h = buildHeader(true, false, false, unitsHuman1024)
	assert.Equal(t, "Size", h[1])
	assert.Contains(t, h, "Avail")
	assert.Contains(t, h, "Use%")
	assert.NotContains(t, h, "1024-blocks")
	assert.NotContains(t, h, "Capacity")

	// -P -H: same for SI mode.
	h = buildHeader(true, false, false, unitsHuman1000)
	assert.Equal(t, "Size", h[1])
	assert.Contains(t, h, "Avail")
	assert.Contains(t, h, "Use%")
	assert.NotContains(t, h, "Capacity")

	// -P -T (block POSIX with Type): keeps "Capacity" — only -h/-H
	// drops it, since -T does not change unit mode.
	h = buildHeader(true, true, false, unitsK)
	assert.Contains(t, h, "Capacity")
	assert.Equal(t, "Type", h[1])
}

func TestSelectColumns(t *testing.T) {
	m := diskstats.Mount{
		Total: 1000, Used: 200, Free: 800,
		Inodes: 50, InodesUsed: 10, InodesFree: 40,
	}
	// block mode
	a, b, c := selectColumns(m, false)
	assert.Equal(t, uint64(1000), a)
	assert.Equal(t, uint64(200), b)
	assert.Equal(t, uint64(800), c)

	// inode mode
	a, b, c = selectColumns(m, true)
	assert.Equal(t, uint64(50), a)
	assert.Equal(t, uint64(10), b)
	assert.Equal(t, uint64(40), c)
}

func ptrBool(v bool) *bool          { return &v }
func ptrSlice(v []string) *[]string { return &v }

// -h / -H share the unit-mode target via unitFlag, so argv order
// picks the winner (last-set wins). Verify every interleaving emits
// the expected mode.
func TestUnitFlag_LastFlagWins(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want unitMode
	}{
		{"no flag", nil, unitsK},
		{"-k only", []string{"-k"}, unitsK},
		{"-h only", []string{"-h"}, unitsHuman1024},
		{"-H only", []string{"-H"}, unitsHuman1000},
		{"-h then -H → SI", []string{"-h", "-H"}, unitsHuman1000},
		{"-H then -h → IEC", []string{"-H", "-h"}, unitsHuman1024},
		{"-hH (combined short) → SI", []string{"-hH"}, unitsHuman1000},
		{"-Hh (combined short) → IEC", []string{"-Hh"}, unitsHuman1024},
		// Non-unit flags interleaved must not change the answer.
		{"-h -P -H → SI", []string{"-h", "-P", "-H"}, unitsHuman1000},
		{"--si then --human-readable → IEC",
			[]string{"--si", "--human-readable"}, unitsHuman1024},
		// -k participates in the same last-flag-wins group; GNU df
		// treats `-h -k` as 1K-blocks (-k is "equivalent to
		// --block-size=1K", which is itself a unit override).
		{"-h then -k → 1K-blocks", []string{"-h", "-k"}, unitsK},
		{"-H then -k → 1K-blocks", []string{"-H", "-k"}, unitsK},
		{"-k then -h → IEC", []string{"-k", "-h"}, unitsHuman1024},
		{"-k then -H → SI", []string{"-k", "-H"}, unitsHuman1000},
		{"-hk (combined short) → 1K-blocks", []string{"-hk"}, unitsK},
		{"-kh (combined short) → IEC", []string{"-kh"}, unitsHuman1024},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("df", pflag.ContinueOnError)
			handler := makeFlags(fs)
			_ = handler // exercise the same flag wiring df uses at runtime
			require.NoError(t, fs.Parse(c.argv))
			// Look up the human-readable Var to access its target.
			// Both -h and -H point to the same shared target via
			// unitFlag, so reading either reveals the final mode.
			fl := fs.Lookup("human-readable")
			require.NotNil(t, fl)
			uf, ok := fl.Value.(*unitFlag)
			require.True(t, ok, "expected unitFlag value type")
			assert.Equal(t, c.want, *uf.target)
		})
	}
}
