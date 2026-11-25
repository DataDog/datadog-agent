// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usage

import (
	"cmp"
	"slices"
	"time"
)

// Class says what the absence of usage of a component of an index tells.
type Class uint8

const (
	// ClassOS is an OS package. The index holds the files its package
	// database lists, so a package that no key of a complete report, one
	// with Truncated unset, reaches did not run. A file whose key another
	// path shares, which the builder drops, is the exception.
	ClassOS Class = iota + 1
	// ClassLanguage is an application package, which the index knows by
	// some of its files alone.
	ClassLanguage
)

// A Component is a component of the SBOM that an Index describes.
type Component struct {
	BOMRef string
	Class  Class
	PURL   string
}

// An Index maps the keys of the files of an SBOM to the components that own
// them. It is immutable and safe for concurrent use.
type Index struct {
	serial    string
	scanStart time.Time
	scanEnd   time.Time

	// keys holds the anchors in increasing order, a key once per owner, and
	// owners[i] is the index in components of the owner of keys[i].
	keys       []uint64
	owners     []uint32
	components []Component
}

// DeepCopy returns x itself, which is immutable. workloadmeta deep copies the
// entities it hands out, and the SBOM of an image holds its index.
func (x *Index) DeepCopy() interface{} {
	return x
}

// Serial returns the serial number of the BOM that x describes.
func (x *Index) Serial() string {
	return x.serial
}

// ScanStart returns the time the scan that built x started.
func (x *Index) ScanStart() time.Time {
	return x.scanStart
}

// ScanEnd returns the time the scan that built x ended.
func (x *Index) ScanEnd() time.Time {
	return x.scanEnd
}

// Len returns the number of components of x.
func (x *Index) Len() int {
	return len(x.components)
}

// Component returns the component i of x.
func (x *Index) Component(i int) Component {
	return x.components[i]
}

// Anchors returns the number of keys of x, a key counted once per owner.
func (x *Index) Anchors() int {
	return len(x.keys)
}

// Size returns the bytes x holds.
func (x *Index) Size() int {
	n := len(x.serial) + 12*len(x.keys)
	for _, c := range x.components {
		n += len(c.BOMRef) + len(c.PURL) + 1
	}
	return n
}

// A Usage is the usage of a component that a report gives.
type Usage struct {
	LastSeen int64 // unix seconds, 0 when no key of the report reaches the component
	Flags    uint8
}

// Join returns the usage that the keys of r give each component of x, by the
// index of the component: the latest time one of its keys ran, and the flags
// of all of them. A key with several owners credits each.
func Join(x *Index, r *Report) []Usage {
	if x == nil {
		return nil
	}
	u := make([]Usage, len(x.components))
	for i, key := range r.Keys {
		j, _ := slices.BinarySearch(x.keys, key)
		for ; j < len(x.keys) && x.keys[j] == key; j++ {
			o := x.owners[j]
			u[o].LastSeen = max(u[o].LastSeen, r.LastSeen[i])
			u[o].Flags |= r.Flags[i]
		}
	}
	return u
}

// A Builder builds an Index. The zero value is ready to use.
type Builder struct {
	components []Component
	keys       []uint64
	owners     []uint32
	// paths holds the path of each key added, and "" for a key two paths
	// share.
	paths map[uint64]string
}

// Add adds c with the paths through which a process uses it, and returns the
// index of c. It cleans the paths and leaves out documentation.
func (b *Builder) Add(c Component, paths ...string) int {
	owner := uint32(len(b.components))
	b.components = append(b.components, c)
	for _, p := range paths {
		p = Clean(p)
		if !isDoc(p) {
			b.add(Key(p), p, owner)
		}
	}
	return int(owner)
}

func (b *Builder) add(key uint64, p string, owner uint32) {
	if b.paths == nil {
		b.paths = make(map[uint64]string)
	}
	if prev, ok := b.paths[key]; !ok {
		b.paths[key] = p
	} else if prev != p {
		b.paths[key] = ""
	}
	b.keys = append(b.keys, key)
	b.owners = append(b.owners, owner)
}

// Build returns the index of the components added, for the BOM with the serial
// number serial that a scan from scanStart to scanEnd produced. It leaves out
// a key that two paths share, which would credit a component for the run of
// another path. A path under one of mergedDirs also gets the key of its
// usr-merged alias under /usr, and back, when no component claims the alias,
// since processes reach either through the other on a usr-merged root.
func (b *Builder) Build(serial string, scanStart, scanEnd time.Time) *Index {
	for i, n := 0, len(b.keys); i < n; i++ {
		p := b.paths[b.keys[i]]
		alias := pathAlias(p)
		if p == "" || alias == "" {
			continue
		}
		key := Key(alias)
		if _, claimed := b.paths[key]; !claimed {
			b.keys = append(b.keys, key)
			b.owners = append(b.owners, b.owners[i])
		}
	}

	type anchor struct {
		key   uint64
		owner uint32
	}
	anchors := make([]anchor, 0, len(b.keys))
	for i, key := range b.keys {
		if p, ok := b.paths[key]; ok && p == "" {
			continue
		}
		anchors = append(anchors, anchor{key, b.owners[i]})
	}
	slices.SortFunc(anchors, func(a, b anchor) int {
		return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.owner, b.owner))
	})
	anchors = slices.Compact(anchors)

	x := &Index{
		serial:     serial,
		scanStart:  scanStart,
		scanEnd:    scanEnd,
		keys:       make([]uint64, len(anchors)),
		owners:     make([]uint32, len(anchors)),
		components: slices.Clip(b.components),
	}
	for i, a := range anchors {
		x.keys[i], x.owners[i] = a.key, a.owner
	}
	*b = Builder{}
	return x
}
