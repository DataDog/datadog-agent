// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"path/filepath"
	"testing"
)

// Child file/dir counts follow the same walk-up attribution as byte totals:
// a file anywhere in a child's subtree counts toward that child, and Dirs is
// the number of descendant directories (excluding the child itself).
func TestScan_CountsTreeChildren(t *testing.T) {
	root := scanTempDir(t)
	writeFile(t, filepath.Join(root, "A", "f1.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "f2.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "sub", "f3.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "B", "f4.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	a := findTreeChild(t, res.Tree, "A")
	if a.Files != 3 {
		t.Errorf("child A Files = %d, want 3 (f1, f2, sub/f3)", a.Files)
	}
	if a.Dirs != 1 {
		t.Errorf("child A Dirs = %d, want 1 (sub)", a.Dirs)
	}
	b := findTreeChild(t, res.Tree, "B")
	if b.Files != 1 {
		t.Errorf("child B Files = %d, want 1", b.Files)
	}
	if b.Dirs != 0 {
		t.Errorf("child B Dirs = %d, want 0", b.Dirs)
	}
}

// In tree mode the counts are cumulative like Size: deep files/dirs roll up into
// their in-tree ancestors, and the root node totals the whole in-scope subtree.
func TestScan_CountsTreeRollup(t *testing.T) {
	root := scanTempDir(t)
	writeFile(t, filepath.Join(root, "A", "f1.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "sub", "deep", "f2.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "B", "f3.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "loose.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})
	if res.Tree == nil {
		t.Fatal("Tree is nil at TreeDepth 1")
	}

	// Root totals: all 4 files (incl. loose) and all 4 descendant dirs
	// (A, A/sub, A/sub/deep, B).
	if res.Tree.Files != 4 {
		t.Errorf("root Files = %d, want 4", res.Tree.Files)
	}
	if res.Tree.Dirs != 4 {
		t.Errorf("root Dirs = %d, want 4 (A, A/sub, A/sub/deep, B)", res.Tree.Dirs)
	}

	a := findTreeChild(t, res.Tree, "A")
	if a.Files != 2 {
		t.Errorf("A Files = %d, want 2 (f1 + rolled-up deep f2)", a.Files)
	}
	if a.Dirs != 2 {
		t.Errorf("A Dirs = %d, want 2 (sub, sub/deep)", a.Dirs)
	}
	b := findTreeChild(t, res.Tree, "B")
	if b.Files != 1 || b.Dirs != 0 {
		t.Errorf("B Files/Dirs = %d/%d, want 1/0", b.Files, b.Dirs)
	}
}

// A hardlinked file within a single directory is counted once for that child,
// mirroring the byte-total dedup.
func TestScan_CountsHardlinkSameChild(t *testing.T) {
	root := scanTempDir(t)
	primary := filepath.Join(root, "A", "primary.bin")
	writeFile(t, primary, make([]byte, 4096))
	createHardLink(t, filepath.Join(root, "A", "alias.bin"), primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})
	if got := findTreeChild(t, res.Tree, "A").Files; got != 1 {
		t.Errorf("child A Files = %d, want 1 (hardlink counted once)", got)
	}
}

// A file hardlinked across two children counts once in each, mirroring how its
// bytes are attributed to both.
func TestScan_CountsHardlinkAcrossChildren(t *testing.T) {
	root := scanTempDir(t)
	primary := filepath.Join(root, "A", "primary.bin")
	writeFile(t, primary, make([]byte, 4096))
	createHardLink(t, filepath.Join(root, "B", "alias.bin"), primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})
	if got := findTreeChild(t, res.Tree, "A").Files; got != 1 {
		t.Errorf("child A Files = %d, want 1", got)
	}
	if got := findTreeChild(t, res.Tree, "B").Files; got != 1 {
		t.Errorf("child B Files = %d, want 1", got)
	}
}

// Excluded subtrees are omitted from counts exactly as they are from byte totals.
func TestScan_CountsExcludeRespected(t *testing.T) {
	root := scanTempDir(t)
	writeFile(t, filepath.Join(root, "A", "keep.bin"), make([]byte, 4096))
	drop := filepath.Join(root, "A", "skip")
	writeFile(t, filepath.Join(drop, "gone.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1, Exclude: []string{drop}})
	a := findTreeChild(t, res.Tree, "A")
	if a.Files != 1 {
		t.Errorf("child A Files = %d, want 1 (excluded file omitted)", a.Files)
	}
	if a.Dirs != 0 {
		t.Errorf("child A Dirs = %d, want 0 (excluded 'skip' dir omitted)", a.Dirs)
	}
}

// The fast-path depth-1 tree and the general-path depth-2 scan must agree on
// the depth-1 children's size and counts — the two code paths attribute the
// same way. (Depth 1 takes the fast path; depth >= 2 takes the general path.)
func TestScan_CountsDepth1MatchesDepth2(t *testing.T) {
	root := scanTempDir(t)
	writeFile(t, filepath.Join(root, "A", "f1.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "sub", "f2.bin"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "B", "sub2", "deep", "f3.bin"), make([]byte, 4096))

	d1 := scanOrSkip(t, root, Options{TreeDepth: 1})
	d2 := scanOrSkip(t, root, Options{TreeDepth: 2})

	// Root totals are the headline numbers, and they come from different code
	// on each path (subtreeFiles/subtreeDirs on the fast path vs
	// anchorFiles/anchorDirs on the general path), so assert they agree.
	if d1.Tree.Files != d2.Tree.Files || d1.Tree.Dirs != d2.Tree.Dirs || d1.Tree.Size != d2.Tree.Size {
		t.Errorf("root differs: depth1 %d/%d/%d vs depth2 %d/%d/%d",
			d1.Tree.Files, d1.Tree.Dirs, d1.Tree.Size, d2.Tree.Files, d2.Tree.Dirs, d2.Tree.Size)
	}

	for _, name := range []string{"A", "B"} {
		c1 := findTreeChild(t, d1.Tree, name)
		c2 := findTreeChild(t, d2.Tree, name)
		if c1.Files != c2.Files || c1.Dirs != c2.Dirs || c1.Size != c2.Size {
			t.Errorf("child %s differs: depth1 %d/%d/%d vs depth2 %d/%d/%d",
				name, c1.Files, c1.Dirs, c1.Size, c2.Files, c2.Dirs, c2.Size)
		}
	}
}
