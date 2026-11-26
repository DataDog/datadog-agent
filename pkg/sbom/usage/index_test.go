// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usage

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"
)

var (
	bash  = Component{BOMRef: "pkg:deb/debian/bash@5.2.15-2", Class: ClassOS, PURL: "pkg:deb/debian/bash@5.2.15-2"}
	coreu = Component{BOMRef: "pkg:deb/debian/coreutils@9.1-1", Class: ClassOS, PURL: "pkg:deb/debian/coreutils@9.1-1"}
	expr  = Component{BOMRef: "pkg:npm/express@4.18.2", Class: ClassLanguage, PURL: "pkg:npm/express@4.18.2"}
)

// owners returns the components of x that the file at p credits.
func owners(x *Index, p string) []string {
	r := &Report{}
	for _, key := range AppendKeys(nil, Clean(p)) {
		r.Keys = append(r.Keys, key)
		r.LastSeen = append(r.LastSeen, 1)
		r.Flags = append(r.Flags, 0)
	}
	var refs []string
	for i, u := range Join(x, r) {
		if u.LastSeen != 0 {
			refs = append(refs, x.Component(i).BOMRef)
		}
	}
	return refs
}

func TestBuild(t *testing.T) {
	var b Builder
	b.Add(bash, "bin/bash", "/usr/share/doc/bash/copyright", "/usr/share/man/man1/bash.1.gz")
	b.Add(coreu, "/usr/bin/ls", "/bin/ls", "/usr/bin/env")
	b.Add(expr, "app/node_modules/express/package.json")
	// A second package claims /usr/bin/env.
	b.Add(Component{BOMRef: "env", Class: ClassOS}, "/usr/bin/env")
	start := time.Unix(1000, 0)
	x := b.Build("urn:uuid:1", start, start.Add(time.Second))

	if x.Serial() != "urn:uuid:1" || !x.ScanStart().Equal(start) || x.Len() != 4 {
		t.Fatalf("Build = serial %q, start %v, %d components", x.Serial(), x.ScanStart(), x.Len())
	}
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/bin/bash", []string{bash.BOMRef}},
		// The alias of /bin/bash, which no component claims.
		{"/usr/bin/bash", []string{bash.BOMRef}},
		// Both forms are claimed, so neither gets an alias.
		{"/usr/bin/ls", []string{coreu.BOMRef}},
		{"/bin/ls", []string{coreu.BOMRef}},
		{"/usr/bin/env", []string{coreu.BOMRef, "env"}},
		{"/bin/env", []string{coreu.BOMRef, "env"}},
		{"/usr/share/doc/bash/copyright", nil},
		{"/app/node_modules/express/lib/router/index.js", []string{expr.BOMRef}},
		{"/usr/bin/zsh", nil},
	} {
		if got := owners(x, tc.path); !slices.Equal(got, tc.want) {
			t.Errorf("owners of %s = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestBuildDropsCollisions checks that a key two paths share credits nothing,
// and that a path listed twice keeps its key.
func TestBuildDropsCollisions(t *testing.T) {
	var b Builder
	a := b.Add(bash)
	c := b.Add(coreu)
	b.add(1, "/opt/a", uint32(a))
	b.add(1, "/opt/b", uint32(c))
	b.add(2, "/opt/c", uint32(a))
	b.add(2, "/opt/c", uint32(a))
	x := b.Build("", time.Time{}, time.Time{})
	if want := []uint64{2}; !slices.Equal(x.keys, want) {
		t.Errorf("keys = %v, want %v", x.keys, want)
	}
}

func TestJoin(t *testing.T) {
	var b Builder
	b.Add(bash, "/usr/bin/bash", "/usr/lib/bash/libbash.so")
	b.Add(coreu, "/usr/bin/ls")
	x := b.Build("", time.Time{}, time.Time{})

	r := &Report{}
	for _, k := range []struct {
		path     string
		lastSeen int64
		flags    uint8
	}{
		{"/usr/bin/bash", 10, FlagRoot},
		{"/usr/lib/bash/libbash.so", 20, 0},
		{"/usr/bin/su", 30, FlagSetuid | FlagRoot},
	} {
		r.Keys = append(r.Keys, Key(k.path))
		r.LastSeen = append(r.LastSeen, k.lastSeen)
		r.Flags = append(r.Flags, k.flags)
	}
	sortReport(r)

	got := Join(x, r)
	want := []Usage{{LastSeen: 20, Flags: FlagRoot}, {}}
	if !slices.Equal(got, want) {
		t.Errorf("Join = %+v, want %+v", got, want)
	}
	if got := Join(nil, r); got != nil {
		t.Errorf("Join(nil) = %+v, want nil", got)
	}
}

// sortReport sorts the keys of r, moving their times and flags with them.
func sortReport(r *Report) {
	order := make([]int, len(r.Keys))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		switch {
		case r.Keys[a] < r.Keys[b]:
			return -1
		case r.Keys[a] > r.Keys[b]:
			return 1
		}
		return 0
	})
	keys, lastSeen, flags := slices.Clone(r.Keys), slices.Clone(r.LastSeen), slices.Clone(r.Flags)
	for i, j := range order {
		r.Keys[i], r.LastSeen[i], r.Flags[i] = keys[j], lastSeen[j], flags[j]
	}
}

func TestIndexDeepCopy(t *testing.T) {
	var b Builder
	b.Add(bash, "/usr/bin/bash")
	x := b.Build("", time.Time{}, time.Time{})
	if got := x.DeepCopy(); got != x {
		t.Errorf("DeepCopy() = %p, want %p", got, x)
	}
	var none *Index
	if got := none.DeepCopy(); got.(*Index) != nil {
		t.Errorf("DeepCopy() of nil = %v, want nil", got)
	}
}

func ExampleJoin() {
	var b Builder
	b.Add(Component{BOMRef: "pkg:deb/debian/bash@5.2.15-2", Class: ClassOS}, "/bin/bash")
	b.Add(Component{BOMRef: "pkg:npm/express@4.18.2", Class: ClassLanguage}, "/app/node_modules/express/package.json")
	x := b.Build("urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79", time.Unix(1700000000, 0), time.Unix(1700000010, 0))

	// system-probe saw a process run /usr/bin/bash as root, and node load
	// a file of express.
	ran := map[uint64]Usage{}
	for _, run := range []struct {
		path  string
		at    int64
		flags uint8
	}{
		{"/usr/bin/bash", 1700000100, FlagRoot},
		{"/app/node_modules/express/lib/router/index.js", 1700000200, 0},
	} {
		for _, key := range AppendKeys(nil, Clean(run.path)) {
			ran[key] = Usage{LastSeen: run.at, Flags: run.flags}
		}
	}
	r := &Report{KeyKind: KeyImage, ImageID: "sha256:abc"}
	for _, key := range slices.Sorted(maps.Keys(ran)) {
		r.Keys = append(r.Keys, key)
		r.LastSeen = append(r.LastSeen, ran[key].LastSeen)
		r.Flags = append(r.Flags, ran[key].Flags)
	}
	if err := r.Validate(); err != nil {
		fmt.Println(err)
		return
	}

	for i, u := range Join(x, r) {
		fmt.Printf("%s last seen %d, root %v\n", x.Component(i).BOMRef, u.LastSeen, u.Flags&FlagRoot != 0)
	}
	// Output:
	// pkg:deb/debian/bash@5.2.15-2 last seen 1700000100, root true
	// pkg:npm/express@4.18.2 last seen 1700000200, root false
}
