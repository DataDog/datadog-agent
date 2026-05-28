//go:build windows

package du

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// -------------------------------------------------------------------------
// Public API
// -------------------------------------------------------------------------

// Result is the per-immediate-child disk-usage breakdown for a target
// directory, plus the overall subtree total and scan diagnostics.
type Result struct {
	// Target is the absolute path the scan was asked to analyze.
	Target string

	// Buckets is one entry per immediate child directory of Target, sorted by
	// Size descending. Files directly under Target (not in any child) are
	// reported separately as Loose.
	Buckets []Bucket

	// Subtree is the deduplicated total of all bytes attributed to Target's
	// subtree. Files with hardlinks across multiple buckets count once toward
	// Subtree, but contribute to each bucket's Size.
	Subtree int64

	// Loose is the bytes from files directly under Target (not in any child
	// directory).
	Loose int64

	// MultiBucketFiles counts files attributed to >1 bucket (cross-bucket
	// hardlinks).
	MultiBucketFiles int

	// Pass diagnostics: parsed/error counts and durations.
	// Pass1 builds dirParent + extSize/extParents from a single MFT scan
	// (modeAll). Pass2 tallies file bases (modeFileBaseOnly).
	RecordsParsed int
	ParseErrors   int
	Pass1, Pass2  time.Duration
	Wall          time.Duration

	// Volume info reported back for the CLI.
	TotalMFTRecords int64
	MFTBytes        int64

	// TopFiles is the N largest in-scope files, sorted descending by Size.
	// Populated only when Options.TopFiles > 0. Paths are resolved post-scan
	// via OpenFileByID; on resolution failure the entry's Path is
	// "?\<basename>".
	TopFiles []FileEntry

	// TopExtensions is the N largest file extensions by aggregated in-scope
	// size, sorted descending. Populated only when Options.TopExtensions > 0.
	TopExtensions []ExtensionEntry

	// FindResults is one block per Options.Finds entry, in input order.
	// Each block carries the originating FindQuery and the matched files
	// sorted by size descending. Populated only when Options.Finds is
	// non-empty.
	FindResults []FindResultBlock

	// ExcludedDirs is the number of distinct directories that were marked
	// out-of-scope by the Exclude option. Reported for diagnostics.
	ExcludedDirs int

	// Tree, when Options.TreeDepth > 0, is the depth-limited subtree
	// rooted at the scan target. Cumulative sizes (subtree totals) are
	// computed for every node at depth 0..TreeDepth from target.
	// nil when Options.TreeDepth == 0.
	Tree *TreeNode
}

// Bucket is one immediate child directory of the target with its computed
// total size (including all transitively-contained files / hardlinks that
// fall under this bucket).
type Bucket struct {
	Name string
	Size int64
	// Reparse is true when the child is a reparse point (directory junction,
	// symlink, or volume mount point). Its size reflects only what lives on
	// the scanned volume's MFT — for cross-volume mount points the actual
	// contents live on a different volume and are not visible to this scan.
	Reparse bool
}

// TreeNode is one directory entry in the depth-limited tree returned by
// Scan when Options.TreeDepth > 0. Size is the cumulative subtree total
// for everything under this directory on the scanned volume — including
// dirs at depths beyond TreeDepth (their bytes roll up to the deepest
// in-tree ancestor). Children are sorted by Size descending then Name
// ascending; they are filtered by Options.TreeMinSize at the leaves but
// the root and all directories at depth ≤ TreeDepth that have any
// in-scope content are present.
type TreeNode struct {
	Name     string // target's full path on the root node; basename otherwise
	Idx      uint64
	Depth    int   // 0 = target, increasing toward leaves
	Size     int64 // cumulative subtree total
	Reparse  bool  // dir is a reparse point (junction / symlink / mount point)
	Children []*TreeNode
}

// Options configures a scan.
type Options struct {
	// ShowApparent reports logical (apparent) sizes instead of disk
	// allocation. Default false: report on-disk allocation, which matches
	// Windows Explorer "Size on disk" for sparse and compressed files.
	ShowApparent bool

	// TopFiles, when > 0, populates Result.TopFiles with the N largest files
	// found in the in-scope subtree. Tracked via a min-heap during pass 3;
	// hot-path cost is one int comparison per file plus, for the few that
	// qualify, basename decode. Path resolution happens once after the scan
	// via OpenFileByID.
	TopFiles int

	// TopExtensions, when > 0, populates Result.TopExtensions with the top-N
	// file extensions ranked by aggregated size. Adds ~16 bytes of UTF-16
	// scanning per file in pass 3 — opt-in for that reason.
	TopExtensions int

	// MinFileSize sets a floor for the TopFiles heap: candidates strictly
	// smaller are not considered. Useful to focus on large files only.
	// 0 = no floor.
	MinFileSize int64

	// Finds is the list of independent file-matching predicates to evaluate
	// during the scan. Each entry becomes its own per-query slot with its
	// own Limit and result block in Result.FindResults; queries do not
	// compete with each other for capacity. See FindQuery for the per-type
	// Value syntax (ext / glob / regex).
	Finds []FindQuery

	// FindFastNameDecode enables a zero-allocation in-place UTF-16 → ASCII
	// decode for glob / regex predicate evaluation, with a fallback to the
	// allocating decoder for non-ASCII filenames. Has no effect when every
	// configured Find is an "ext" query (extension matching is already
	// allocation-free).
	FindFastNameDecode bool

	// Exclude is a list of absolute paths whose subtrees should be excluded
	// from the scan totals. Each path is resolved to an MFT idx upfront; any
	// directory whose ancestor chain includes one of these is treated as
	// out-of-scope (bucketOutside). Files in excluded subtrees do not count
	// toward bucket totals, the subtree total, the top-files heap, or the
	// extension aggregation.
	Exclude []string

	// TreeDepth, when > 0, populates Result.Tree with cumulative subtree
	// sizes for every directory at depth 1..TreeDepth from target (in
	// addition to the existing depth-1 Buckets output). Files at depths
	// beyond TreeDepth still count — their bytes roll up to the deepest
	// in-tree ancestor. Adds ~one decoded name per directory in pass 1
	// (~30 MiB on a typical Windows volume).
	TreeDepth int

	// TreeMinSize hides any tree node whose cumulative size is below
	// this threshold from Result.Tree's Children — and consequently
	// from Result.Buckets, which in tree mode is the depth-1
	// projection of Result.Tree.Children. Has no effect when TreeDepth
	// is 0. The root (target) is always included. 0 = show every
	// populated node. Result.Loose is unaffected: it is always the
	// total bytes of files whose immediate parent is target,
	// independent of this threshold.
	TreeMinSize int64
}

// Scan computes disk usage per immediate child of targetDir on the volume
// containing targetDir. Requires Administrator privileges (raw \\.\<drive>:
// open). The context is honored between MFT chunks; cancellation aborts the
// scan with ctx.Err().
func Scan(ctx context.Context, targetDir string, opts Options) (*Result, error) {
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", targetDir, err)
	}
	abs = upcaseDriveLetter(abs)
	if !strings.HasSuffix(abs, `\`) {
		abs += `\`
	}
	if len(abs) < 3 || abs[1] != ':' {
		return nil, fmt.Errorf("target must be an absolute Windows path: %q", abs)
	}

	t0 := time.Now()
	res := &Result{Target: abs}

	drive := abs[:1]
	hVol, vol, err := openVolume(drive)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(hVol)

	res.TotalMFTRecords = vol.mftValidBytes / int64(vol.recordSize)
	res.MFTBytes = vol.mftValidBytes

	mftExtents, err := getMFTExtents(hVol, vol)
	if err != nil {
		return nil, fmt.Errorf("MFT extents: %w", err)
	}

	// Resolve target idx + immediate children via Windows API. This is the
	// only place names are touched in the entire scan; the bulk MFT walk
	// never decodes UTF-16 names.
	// Pass abs with its trailing backslash. Stripping it on a drive root
	// (e.g. "C:\" → "C:") is fatal: CreateFile("C:") opens the per-process
	// current directory on drive C:, not the volume root — every subtree
	// rooted at cwd then gets misattributed as "loose" during the C:\ scan.
	// CreateFile + FILE_FLAG_BACKUP_SEMANTICS handles "C:\" and "C:\dir\"
	// equivalently for non-root paths.
	targetIdx, err := getMFTIdxFromPath(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve target idx: %w", err)
	}
	children, err := enumerateImmediateChildren(abs)
	if err != nil {
		return nil, fmt.Errorf("enumerate children: %w", err)
	}
	sort.Slice(children, func(i, j int) bool {
		return strings.ToLower(children[i].name) < strings.ToLower(children[j].name)
	})

	const (
		bucketOutside = -1
		bucketTarget  = -2
	)

	bucketByIdx := make(map[uint64]int, len(children))
	for i, c := range children {
		bucketByIdx[c.idx] = i
	}

	// Resolve exclusion paths to MFT idxs. We do this BEFORE pass 1 so that
	// walkUp can short-circuit excluded subtrees as bucketOutside without
	// any per-file cost in passes 2/3.
	excludedIdxs := make(map[uint64]struct{}, len(opts.Exclude))
	for _, p := range opts.Exclude {
		ap, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		ap = upcaseDriveLetter(ap)
		// Skip exclusions on a different volume — they cannot be in this MFT.
		if len(ap) >= 2 && ap[1] == ':' && ap[0] != abs[0] {
			continue
		}
		idx, err := getMFTIdxFromPath(ap)
		if err != nil {
			continue
		}
		excludedIdxs[idx] = struct{}{}
	}

	// ===== Pass 1: build dirParent + extSize/extParents in one scan =====
	// Reads every in-use record (modeAll). For each:
	//   - directory base: record idx → parent in dirParent
	//   - directory whose $FILE_NAME spilled to an extension: stash via
	//     pendingExtParent / dirsAwaitingParent and reconcile end-of-pass
	//   - extension record: accumulate $DATA size into extSize[baseRef]
	//     and append $FILE_NAME parents into extParents[baseRef]
	//
	// Folding extension-record accumulation into this pass costs nothing
	// extra: modeAll already fully parses every record. It eliminates a
	// second full MFT scan that would otherwise re-stream the same bytes.
	t1 := time.Now()

	// Map size hint: ~1 directory per ~5 records on typical Windows volumes.
	// Overestimating costs nothing (Go's map shrinks unused buckets); under-
	// estimating triggers ~lg(N) rehashes during pass 1.
	dirHint := int(res.TotalMFTRecords / 5)
	dirParent := make(map[uint64]uint64, dirHint)
	pendingExtParent := make(map[uint64]uint64)
	dirsAwaitingParent := make(map[uint64]struct{})

	// Hint: typical Windows volumes have ~1.3% of MFT records as extensions.
	extHint := int(res.TotalMFTRecords / 70)
	extSize := make(map[uint64]int64, extHint)
	extParents := make(map[uint64][]uint64, extHint)

	// dirName: populated by a focused post-pass scan after walkUp
	// identifies which dirs are at depth ≤ TreeDepth. Captures decoded
	// names for the (~10K-50K) displayed dirs only — NOT every dir on
	// the volume. The bulk pass 1 walk stays name-free, preserving the
	// project's "no UTF-16 decoding in the bulk walk" allocation
	// discipline (saves ~25-30 MiB peak vs decoding every dir).
	var dirName map[uint64]string

	parsed1, errs1 := streamPipelined(ctx, hVol, mftExtents, vol.recordSize, modeAll, func(idx uint64, e *mftEntry, baseRef uint64) {
		// Skip deleted / unallocated MFT slots.
		if !e.isInUse {
			return
		}
		// Extension records belong to the base file; use its index for filtering.
		check := idx
		if baseRef != 0 {
			check = baseRef
		}
		// Skip NTFS system metafiles ($MFT, $Bitmap, root, …) in slots 0–15.
		if check <= maxMetafileMFTIndex {
			return
		}

		if baseRef != 0 {
			// Extension record. Accumulate per-base $DATA size and
			// $FILE_NAME parents for use by pass 2's tally.
			var sz int64
			if opts.ShowApparent {
				sz = e.dataSize
			} else {
				sz = e.allocatedSize
			}
			if sz > 0 {
				extSize[baseRef] += sz
			}
			extParents[baseRef] = append(extParents[baseRef], e.hardlinkParents...)

			// Reconcile dir parent when $FILE_NAME spilled to an extension record.
			if e.primaryParent == 0 {
				return // no parent on this extension; nothing to stash or satisfy
			}
			if _, awaiting := dirsAwaitingParent[baseRef]; awaiting {
				// Base dir was seen first without a parent — apply now.
				dirParent[baseRef] = e.primaryParent
				delete(dirsAwaitingParent, baseRef)
				return
			}
			// Base not seen yet (or not awaiting): stash parent for when base is visited.
			if _, exists := pendingExtParent[baseRef]; !exists {
				pendingExtParent[baseRef] = e.primaryParent
			}
			return
		}

		// Base record.
		if !e.isDir {
			delete(pendingExtParent, idx) // wasn't a dir; drop the stash
			return
		}
		if e.primaryParent != 0 {
			dirParent[idx] = e.primaryParent
			delete(pendingExtParent, idx)
			return
		}
		// Dir base with no $FILE_NAME (overflowed to ext) — recover from
		// stash if seen, else mark awaiting.
		if p, ok := pendingExtParent[idx]; ok {
			dirParent[idx] = p
			delete(pendingExtParent, idx)
			return
		}
		dirsAwaitingParent[idx] = struct{}{}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res.Pass1 = time.Since(t1)
	res.RecordsParsed += parsed1
	res.ParseErrors += errs1

	// End-of-pass reconciliation for dirs whose ext arrived after the base.
	for idx := range dirsAwaitingParent {
		if p, ok := pendingExtParent[idx]; ok {
			dirParent[idx] = p
		}
	}
	pendingExtParent = nil
	dirsAwaitingParent = nil

	res.ExcludedDirs = len(excludedIdxs)

	// Two paths from here:
	// - TreeDepth == 0: build dirBucket via walkUp (legacy depth=1
	//   behavior). Pass 2 looks up dirBucket[parent] in O(1).
	// - TreeDepth  > 0: skip dirBucket entirely. Identify only the
	//   dirs at depth ≤ TreeDepth, drop everything else. Pass 2 walks
	//   dirParent per file and accumulates into anchorTotals at each
	//   tree-dir step.
	var dirBucket map[uint64]int       // depth=0 path
	var anchorTotals map[uint64]int64  // depth>0 path
	var treeDirsDepth map[uint64]int16 // idx → depth, only for tree dirs
	if opts.TreeDepth == 0 {
		dirBucket = make(map[uint64]int, len(dirParent))
		dirBucket[targetIdx] = bucketTarget
		for idx, b := range bucketByIdx {
			dirBucket[idx] = b
		}
		for idx := range excludedIdxs {
			if idx == targetIdx {
				continue
			}
			dirBucket[idx] = bucketOutside
		}
		var walkUp func(idx uint64, depth int) int
		walkUp = func(idx uint64, depth int) int {
			if depth > 512 {
				return bucketOutside
			}
			if b, ok := dirBucket[idx]; ok {
				return b
			}
			p, ok := dirParent[idx]
			if !ok {
				dirBucket[idx] = bucketOutside
				return bucketOutside
			}
			b := walkUp(p, depth+1)
			dirBucket[idx] = b
			return b
		}
		for idx := range dirParent {
			walkUp(idx, 0)
		}
		dirParent = nil
	} else {
		// Tree mode: classify each dir as depth-from-target or -1 (out
		// of scope). The classify map is intentionally short-lived —
		// it's the same memory shape as dirBucket would have been,
		// but we only retain the small subset (tree dirs) afterward
		// and free the rest. dirParent stays alive for the per-file
		// walks in pass 2.
		classify := make(map[uint64]int16, len(dirParent))
		classify[targetIdx] = 0
		for idx := range excludedIdxs {
			if idx != targetIdx {
				classify[idx] = -1
			}
		}
		var walkDepth func(idx uint64, recurse int) int16
		walkDepth = func(idx uint64, recurse int) int16 {
			if d, ok := classify[idx]; ok {
				return d
			}
			if recurse > 512 {
				classify[idx] = -1
				return -1
			}
			p, ok := dirParent[idx]
			if !ok {
				classify[idx] = -1
				return -1
			}
			pd := walkDepth(p, recurse+1)
			if pd < 0 {
				classify[idx] = -1
				return -1
			}
			d := pd + 1
			classify[idx] = d
			return d
		}
		for idx := range dirParent {
			walkDepth(idx, 0)
		}

		// Extract the tree dirs (depth ≤ TreeDepth). Pre-seed
		// anchorTotals with zero entries — pass 2's chain walk uses
		// map presence to know whether to accumulate.
		nTree := 0
		for _, d := range classify {
			if d >= 0 && d <= int16(opts.TreeDepth) {
				nTree++
			}
		}
		treeDirsDepth = make(map[uint64]int16, nTree)
		anchorTotals = make(map[uint64]int64, nTree)
		dirName = make(map[uint64]string, nTree)
		for idx, d := range classify {
			if d >= 0 && d <= int16(opts.TreeDepth) {
				treeDirsDepth[idx] = d
				anchorTotals[idx] = 0
				dirName[idx] = ""
			}
		}
		classify = nil
		// dirParent stays alive — pass 2 needs it to walk chains.
	}

	// ===== Pass 2: file base records, immediate tally =====
	// modeFileBaseOnly skips dirs and extensions before the attribute walk.
	// We immediately add into bucketTotals — no per-file map, no per-file
	// slice allocation.
	t2 := time.Now()

	// bucketTotals is the depth-1 attribution slice. Only allocated in
	// depth=0 mode; tree mode derives Buckets from anchorTotals at the
	// end and doesn't tally here.
	var bucketTotals []int64
	if opts.TreeDepth == 0 {
		bucketTotals = make([]int64, len(children))
	}
	var subtree, loose int64
	var multiBucket int

	topF := newTopFiles(opts.TopFiles, opts.MinFileSize)
	extAgg := newExtAggregator(opts.TopExtensions > 0)
	matcher, err := newMatchSet(opts.Finds, opts.FindFastNameDecode)
	if err != nil {
		return nil, err
	}

	// Pass 2 mode: when TreeDepth > 0 we use modeAll so dir base records
	// flow through the callback for opportunistic name capture.
	// Otherwise modeFileBaseOnly skips dirs/extensions before the
	// attribute walk (saves ~25-30% of pass 2 wall when name capture
	// isn't needed).
	pass2Mode := modeFileBaseOnly
	if opts.TreeDepth > 0 {
		pass2Mode = modeAll
	}

	parsed2, errs2 := streamPipelined(ctx, hVol, mftExtents, vol.recordSize, pass2Mode, func(idx uint64, e *mftEntry, baseRef uint64) {
		if !e.isInUse || baseRef != 0 || idx <= maxMetafileMFTIndex {
			return
		}
		if e.isDir {
			// Dir base record: in tree mode, capture name only for
			// tree dirs (dirName has a placeholder entry pre-seeded).
			// In depth=0 mode this branch never fires (modeFileBaseOnly
			// skips dirs at the parser level).
			if dirName != nil {
				if _, want := dirName[idx]; want && len(e.nameBytes) > 0 {
					dirName[idx] = decodeUTF16Name(e.nameBytes)
				}
			}
			return
		}
		// Resolve size. Prefer $DATA; fall back to $FILE_NAME cached size
		// when $DATA is missing.
		var sz int64
		if opts.ShowApparent {
			sz = e.dataSize
			if sz == 0 {
				sz = e.fnDataSize
			}
		} else {
			sz = e.allocatedSize
			if sz == 0 {
				sz = e.fnAllocSize
			}
		}
		if extra, ok := extSize[idx]; ok {
			sz += extra
		}

		if opts.TreeDepth == 0 {
			// Depth=0 path: existing dirBucket attribution.
			var inline [8]int
			buckets := inline[:0]
			add := func(b int) {
				if b == bucketOutside {
					return
				}
				for _, x := range buckets {
					if x == b {
						return
					}
				}
				buckets = append(buckets, b)
			}
			for _, p := range e.hardlinkParents {
				if b, ok := dirBucket[p]; ok {
					add(b)
				}
			}
			if parents, ok := extParents[idx]; ok {
				for _, p := range parents {
					if b, ok := dirBucket[p]; ok {
						add(b)
					}
				}
			}
			if len(buckets) == 0 && e.primaryParent != 0 {
				if b, ok := dirBucket[e.primaryParent]; ok {
					add(b)
				}
			}
			if len(buckets) == 0 {
				return // not in scope — also skips top-N / extAgg / matcher
			}
			subtree += sz
			if len(buckets) > 1 {
				multiBucket++
			}
			for _, b := range buckets {
				switch {
				case b == bucketTarget:
					loose += sz
				case b >= 0:
					bucketTotals[b] += sz
				}
			}
			// Top-N / extAgg / matcher fire only for in-scope files.
			topF.consider(idx, e, sz)
			extAgg.addFromName(e.nameBytes, sz)
			matcher.consider(idx, e, sz)
			return
		}

		// Tree-mode path: walk dirParent per parent ref, accumulating
		// the file's size into every tree-dir ancestor in the last
		// TreeDepth+1 chain entries. The walk terminates at target
		// (in scope) or by exhausting dirParent (out of scope). Dedup
		// across hardlink parents via a small stack-allocated set.
		var seenInline [16]uint64
		seen := seenInline[:0]
		addUnique := func(a uint64) bool {
			for _, x := range seen {
				if x == a {
					return false
				}
			}
			seen = append(seen, a)
			return true
		}
		var chainScratch [32]uint64
		var topAnchors [4]uint64 // depth-1 entries for multiBucket detection
		topAnchorsLen := 0
		anyInScope := false
		looseFile := false // any chain has parent == target (file directly under target)
		attribute := func(parentIdx uint64) {
			chain := chainScratch[:0]
			cur := parentIdx
			reached := false
			for steps := 0; steps < 512; steps++ {
				if _, ex := excludedIdxs[cur]; ex {
					return
				}
				chain = append(chain, cur)
				if cur == targetIdx {
					reached = true
					break
				}
				p, ok := dirParent[cur]
				if !ok {
					return
				}
				cur = p
			}
			if !reached {
				return
			}
			anyInScope = true
			chainLen := len(chain)
			if chainLen == 1 {
				// Parent is target itself — file is directly under target.
				// Mark for Loose tally; don't add to topAnchors (target is
				// not a depth-1 anchor for the cross-bucket counter).
				looseFile = true
			} else {
				// chain[chainLen-2] is the depth-1 anchor (chain[chainLen-1]
				// is target). Track distinct depth-1 anchors across all
				// hardlink chains for the MultiBucketFiles counter.
				d1 := chain[chainLen-2]
				present := false
				for i := 0; i < topAnchorsLen; i++ {
					if topAnchors[i] == d1 {
						present = true
						break
					}
				}
				if !present && topAnchorsLen < len(topAnchors) {
					topAnchors[topAnchorsLen] = d1
					topAnchorsLen++
				}
			}
			start := chainLen - 1 - opts.TreeDepth
			if start < 0 {
				start = 0
			}
			for i := start; i < chainLen; i++ {
				if addUnique(chain[i]) {
					anchorTotals[chain[i]] += sz
				}
			}
		}
		for _, p := range e.hardlinkParents {
			attribute(p)
		}
		if parents, ok := extParents[idx]; ok {
			for _, p := range parents {
				attribute(p)
			}
		}
		if !anyInScope {
			return // out of scope — skip top-N / extAgg / matcher
		}
		subtree += sz
		if looseFile {
			loose += sz
		}
		// File is multi-bucket if it spans 2+ distinct top-level
		// destinations. "loose" (parent == target) counts as one such
		// destination, matching the depth=0 semantic where bucketTarget
		// + a real bucket triggered MultiBucketFiles.
		distinctTops := topAnchorsLen
		if looseFile {
			distinctTops++
		}
		if distinctTops > 1 {
			multiBucket++
		}
		// Top-N / extAgg / matcher fire only for in-scope files.
		topF.consider(idx, e, sz)
		extAgg.addFromName(e.nameBytes, sz)
		matcher.consider(idx, e, sz)
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res.Pass2 = time.Since(t2)
	res.RecordsParsed += parsed2
	res.ParseErrors += errs2

	// Tree mode: build Result.Tree from anchorTotals + invert
	// dirParent (only over tree dirs) for parent → children. Then
	// derive Result.Buckets from the tree's depth-1 children.
	// Depth=0 mode: populate Result.Buckets from bucketTotals.
	if opts.TreeDepth > 0 {
		childrenByParent := make(map[uint64][]uint64, len(treeDirsDepth))
		for idx := range treeDirsDepth {
			if idx == targetIdx {
				continue
			}
			parent, ok := dirParent[idx]
			if !ok {
				continue
			}
			childrenByParent[parent] = append(childrenByParent[parent], idx)
		}
		reparseByIdx := make(map[uint64]bool, len(children))
		for _, c := range children {
			reparseByIdx[c.idx] = c.reparse
		}
		var build func(idx uint64, depth int) *TreeNode
		build = func(idx uint64, depth int) *TreeNode {
			n := &TreeNode{
				Idx:     idx,
				Depth:   depth,
				Size:    anchorTotals[idx],
				Reparse: reparseByIdx[idx],
			}
			if idx == targetIdx {
				n.Name = abs
			} else {
				n.Name = dirName[idx]
			}
			kids := childrenByParent[idx]
			if len(kids) > 0 {
				n.Children = make([]*TreeNode, 0, len(kids))
				for _, kidIdx := range kids {
					if anchorTotals[kidIdx] < opts.TreeMinSize {
						continue
					}
					n.Children = append(n.Children, build(kidIdx, depth+1))
				}
				sort.SliceStable(n.Children, func(i, j int) bool {
					if n.Children[i].Size != n.Children[j].Size {
						return n.Children[i].Size > n.Children[j].Size
					}
					return n.Children[i].Name < n.Children[j].Name
				})
			}
			return n
		}
		res.Tree = build(targetIdx, 0)

		// Derive Buckets as the tree's depth-1 children projection.
		// Loose was tallied accurately during pass 2 from chains where
		// the file's parent is target itself — independent of
		// TreeMinSize, so the value is always "files directly under
		// target", consistent with the depth=0 semantic.
		if res.Tree != nil {
			res.Buckets = make([]Bucket, 0, len(res.Tree.Children))
			for _, c := range res.Tree.Children {
				if c.Depth != 1 {
					continue
				}
				res.Buckets = append(res.Buckets, Bucket{
					Name:    c.Name,
					Size:    c.Size,
					Reparse: c.Reparse,
				})
			}
		}
		sort.SliceStable(res.Buckets, func(i, j int) bool {
			if res.Buckets[i].Size != res.Buckets[j].Size {
				return res.Buckets[i].Size > res.Buckets[j].Size
			}
			return res.Buckets[i].Name < res.Buckets[j].Name
		})
		dirParent = nil
	} else {
		res.Buckets = make([]Bucket, 0, len(children))
		for i, c := range children {
			if _, ex := excludedIdxs[c.idx]; ex {
				continue
			}
			res.Buckets = append(res.Buckets, Bucket{Name: c.name, Size: bucketTotals[i], Reparse: c.reparse})
		}
		sort.SliceStable(res.Buckets, func(i, j int) bool {
			if res.Buckets[i].Size != res.Buckets[j].Size {
				return res.Buckets[i].Size > res.Buckets[j].Size
			}
			return res.Buckets[i].Name < res.Buckets[j].Name
		})
	}

	// Drop the remaining maps before formatting.
	extSize = nil
	extParents = nil
	dirBucket = nil
	dirName = nil
	anchorTotals = nil

	res.Subtree = subtree
	res.Loose = loose
	res.MultiBucketFiles = multiBucket

	// Resolve top-file paths via OpenFileByID. Bounded by Options.TopFiles
	// — typically tens to hundreds of syscall pairs. Volume root path is
	// "C:\" form (not the raw \\.\C: device); CreateFile + BACKUP_SEMANTICS
	// is what OpenFileByID needs as its rootDir.
	if topF != nil {
		volumeRoot := abs[:3] // "C:\"
		res.TopFiles = resolveCandidatePaths(volumeRoot, topF.drained())
	}
	if extAgg != nil {
		res.TopExtensions = extAgg.topN(opts.TopExtensions, 0)
	}
	if matcher != nil {
		volumeRoot := abs[:3]
		blocks := matcher.drained()
		queries := matcher.queries()
		res.FindResults = make([]FindResultBlock, len(blocks))
		for i, blk := range blocks {
			res.FindResults[i] = FindResultBlock{
				Query:   queries[i],
				Matches: resolveCandidatePaths(volumeRoot, blk),
			}
		}
	}

	res.Wall = time.Since(t0)
	return res, nil
}

// upcaseDriveLetter uppercases the drive letter on a Windows path, leaving
// the rest unchanged. Match the existing PoC's case-folding so paths that
// differ only in drive case still resolve identically.
func upcaseDriveLetter(p string) string {
	if len(p) >= 2 && p[1] == ':' && p[0] >= 'a' && p[0] <= 'z' {
		return strings.ToUpper(p[:1]) + p[1:]
	}
	return p
}

// -------------------------------------------------------------------------
// Volume open + NTFS volume data
// -------------------------------------------------------------------------

const fsctlGetNTFSVolumeData = 0x00090064

type ntfsVolumeData struct {
	VolumeSerialNumber        int64
	NumberSectors             int64
	TotalClusters             int64
	FreeClusters              int64
	TotalReserved             int64
	BytesPerSector            uint32
	BytesPerCluster           uint32
	BytesPerFileRecordSegment uint32
	ClustersPerFRS            uint32
	MftValidDataLength        int64
	MftStartLcn               int64
	Mft2StartLcn              int64
	MftZoneStart              int64
	MftZoneEnd                int64
}

type volumeInfo struct {
	recordSize      int
	bytesPerCluster int64
	mftStartByte    int64
	mftValidBytes   int64
}

func openVolume(drive string) (windows.Handle, *volumeInfo, error) {
	volPath := `\\.\` + drive + ":"
	pw, err := windows.UTF16PtrFromString(volPath)
	if err != nil {
		return 0, nil, err
	}
	h, err := windows.CreateFile(
		pw,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("open %s (need admin): %w", volPath, err)
	}

	var data ntfsVolumeData
	var n uint32
	err = windows.DeviceIoControl(
		h,
		fsctlGetNTFSVolumeData,
		nil, 0,
		(*byte)(unsafe.Pointer(&data)), uint32(unsafe.Sizeof(data)),
		&n, nil,
	)
	if err != nil {
		windows.CloseHandle(h)
		return 0, nil, fmt.Errorf("FSCTL_GET_NTFS_VOLUME_DATA: %w", err)
	}

	vol := &volumeInfo{
		recordSize:      int(data.BytesPerFileRecordSegment),
		bytesPerCluster: int64(data.BytesPerCluster),
		mftStartByte:    data.MftStartLcn * int64(data.BytesPerCluster),
		mftValidBytes:   data.MftValidDataLength,
	}
	return h, vol, nil
}

// -------------------------------------------------------------------------
// MFT extents (record 0 + $ATTRIBUTE_LIST chasing)
// -------------------------------------------------------------------------

// extent is one contiguous on-disk byte range of the $MFT.
type extent struct {
	byteOffset int64
	byteLength int64
}

// getMFTExtents reads MFT record 0, decodes its $DATA data runs, and chases
// $ATTRIBUTE_LIST entries to find extension records that hold additional
// $DATA runs (the $MFT itself is typically heavily fragmented).
func getMFTExtents(hVol windows.Handle, vol *volumeInfo) ([]extent, error) {
	rec0 := make([]byte, vol.recordSize)
	if err := readAt(hVol, rec0, vol.mftStartByte); err != nil {
		return nil, fmt.Errorf("read record 0: %w", err)
	}
	if binary.LittleEndian.Uint32(rec0[0:4]) != mftSignature {
		return nil, errors.New("record 0 bad signature")
	}
	applyFixups(rec0, vol.recordSize)

	firstAttrOff := int(binary.LittleEndian.Uint16(rec0[0x14:0x16]))
	var inline []extent
	var attrList []attrListEntry

	for off := firstAttrOff; off+8 <= vol.recordSize; {
		t := binary.LittleEndian.Uint32(rec0[off : off+4])
		if t == attrEndMarker || t == 0 {
			break
		}
		al := int(binary.LittleEndian.Uint32(rec0[off+4 : off+8]))
		if al < 16 || off+al > vol.recordSize {
			break
		}
		switch t {
		case attrData:
			if rec0[off+8] == 1 && off+0x22 <= vol.recordSize {
				drOff := int(binary.LittleEndian.Uint16(rec0[off+0x20 : off+0x22]))
				inline = decodeDataRuns(rec0[off+drOff:off+al], vol.bytesPerCluster)
			}
		case attrAttributeList:
			attrList = parseAttributeList(rec0[off : off+al])
		}
		off += al
	}

	if len(attrList) == 0 {
		if len(inline) == 0 {
			return nil, errors.New("no $DATA in record 0")
		}
		return inline, nil
	}

	all := append([]extent(nil), inline...)
	readByMFTIdx := func(mftIdx uint64) ([]byte, error) {
		bo := int64(mftIdx) * int64(vol.recordSize)
		var cum int64
		for _, ex := range all {
			if bo < cum+ex.byteLength {
				disk := ex.byteOffset + (bo - cum)
				buf := make([]byte, vol.recordSize)
				if err := readAt(hVol, buf, disk); err != nil {
					return nil, err
				}
				return buf, nil
			}
			cum += ex.byteLength
		}
		return nil, fmt.Errorf("MFT idx %d not in known extents", mftIdx)
	}

	seen := map[uint64]bool{0: true}
	for _, e := range attrList {
		if e.attrType != attrData || seen[e.mftRef] {
			continue
		}
		seen[e.mftRef] = true
		extRec, err := readByMFTIdx(e.mftRef)
		if err != nil {
			continue
		}
		if binary.LittleEndian.Uint32(extRec[0:4]) != mftSignature {
			continue
		}
		applyFixups(extRec, vol.recordSize)

		efa := int(binary.LittleEndian.Uint16(extRec[0x14:0x16]))
		for off := efa; off+8 <= vol.recordSize; {
			t := binary.LittleEndian.Uint32(extRec[off : off+4])
			if t == attrEndMarker || t == 0 {
				break
			}
			al := int(binary.LittleEndian.Uint32(extRec[off+4 : off+8]))
			if al < 16 || off+al > vol.recordSize {
				break
			}
			if t == attrData && extRec[off+8] == 1 && off+0x22 <= vol.recordSize {
				drOff := int(binary.LittleEndian.Uint16(extRec[off+0x20 : off+0x22]))
				more := decodeDataRuns(extRec[off+drOff:off+al], vol.bytesPerCluster)
				all = append(all, more...)
			}
			off += al
		}
	}

	return all, nil
}

// readAt is a thin wrapper around ReadFile with explicit OVERLAPPED offset.
func readAt(h windows.Handle, buf []byte, offset int64) error {
	var ol windows.Overlapped
	ol.Offset = uint32(offset & 0xFFFFFFFF)
	ol.OffsetHigh = uint32(offset >> 32)
	var n uint32
	if err := windows.ReadFile(h, buf, &n, &ol); err != nil {
		return err
	}
	if int(n) < len(buf) {
		return fmt.Errorf("short read: %d < %d", n, len(buf))
	}
	return nil
}

// decodeDataRuns decodes an NTFS data run list into disk extents.
func decodeDataRuns(data []byte, bytesPerCluster int64) []extent {
	var ext []extent
	var lcn int64
	pos := 0
	for pos < len(data) {
		hdr := data[pos]
		if hdr == 0 {
			break
		}
		pos++
		lenSz := int(hdr & 0x0F)
		offSz := int((hdr >> 4) & 0x0F)
		if lenSz == 0 || pos+lenSz+offSz > len(data) {
			break
		}
		var runLen int64
		for i := 0; i < lenSz; i++ {
			runLen |= int64(data[pos+i]) << (uint(i) * 8)
		}
		pos += lenSz
		if offSz == 0 {
			continue // sparse run, no offset
		}
		var runOff int64
		for i := 0; i < offSz; i++ {
			runOff |= int64(data[pos+i]) << (uint(i) * 8)
		}
		if data[pos+offSz-1]&0x80 != 0 {
			for i := offSz; i < 8; i++ {
				runOff |= int64(0xFF) << (uint(i) * 8)
			}
		}
		pos += offSz
		lcn += runOff
		ext = append(ext, extent{byteOffset: lcn * bytesPerCluster, byteLength: runLen * bytesPerCluster})
	}
	return ext
}

// attrListEntry is one entry from a resident $ATTRIBUTE_LIST attribute.
type attrListEntry struct {
	attrType uint32
	mftRef   uint64
}

// parseAttributeList decodes the resident form. Non-resident $ATTRIBUTE_LIST
// is rare and not handled here; the rest of the chain (extension records
// holding $DATA fragments) usually fits in a resident attr list.
func parseAttributeList(attr []byte) []attrListEntry {
	if len(attr) < 24 || attr[8] == 1 {
		return nil
	}
	contentOff := int(binary.LittleEndian.Uint16(attr[0x14:0x16]))
	contentLen := int(binary.LittleEndian.Uint32(attr[0x10:0x14]))
	if contentOff+contentLen > len(attr) {
		return nil
	}
	c := attr[contentOff : contentOff+contentLen]

	var out []attrListEntry
	pos := 0
	for pos+0x18 <= len(c) {
		entryType := binary.LittleEndian.Uint32(c[pos : pos+4])
		entryLen := int(binary.LittleEndian.Uint16(c[pos+4 : pos+6]))
		if entryLen < 0x18 || pos+entryLen > len(c) {
			break
		}
		mftRef := binary.LittleEndian.Uint64(c[pos+0x10 : pos+0x18])
		out = append(out, attrListEntry{attrType: entryType, mftRef: MFTIndex(mftRef)})
		pos += entryLen
	}
	return out
}

// -------------------------------------------------------------------------
// Pipelined ReadFile streamer
// -------------------------------------------------------------------------

// streamPipelined reads MFT bytes via a producer goroutine into one of two
// 4 MiB buffers while the consumer parses the other, then invokes cb for
// each in-buffer record. The pipeline overlaps disk I/O with parsing on cold
// passes (~33% wall reduction in the reflection's measurements).
//
// Single mftEntry reused across all parses; cb MUST NOT retain *mftEntry or
// its hardlinkParents slice past return — copy out anything needed.
func streamPipelined(
	ctx context.Context,
	h windows.Handle,
	extents []extent,
	recordSize int,
	mode parseMode,
	cb func(idx uint64, entry *mftEntry, baseRef uint64),
) (parsed, errs int) {
	const chunkRecords = 4096
	chunkBytes := chunkRecords * recordSize

	type chunk struct {
		bufIdx      int
		n           int
		recordIndex uint64
		err         error
	}
	bufs := [2][]byte{make([]byte, chunkBytes), make([]byte, chunkBytes)}
	free := make(chan int, 2)
	free <- 0
	free <- 1
	ready := make(chan chunk, 1)

	go func() {
		defer close(ready)
		recordIndex := uint64(0)
		for _, ex := range extents {
			extOff := ex.byteOffset
			rem := ex.byteLength
			for rem > 0 {
				if ctx.Err() != nil {
					return
				}
				toRead := int64(chunkBytes)
				if toRead > rem {
					toRead = rem
				}
				bi := <-free
				buf := bufs[bi][:toRead]
				var ol windows.Overlapped
				ol.Offset = uint32(extOff & 0xFFFFFFFF)
				ol.OffsetHigh = uint32(extOff >> 32)
				var n uint32
				rerr := windows.ReadFile(h, buf, &n, &ol)
				ready <- chunk{bufIdx: bi, n: int(n), recordIndex: recordIndex, err: rerr}
				if rerr != nil {
					nr := toRead / int64(recordSize)
					recordIndex += uint64(nr)
					extOff += toRead
					rem -= toRead
				} else {
					recordIndex += uint64(int64(n) / int64(recordSize))
					extOff += int64(n)
					rem -= int64(n)
				}
			}
		}
	}()

	var entry mftEntry
	for ch := range ready {
		if ch.err != nil {
			free <- ch.bufIdx
			continue
		}
		nRecs := ch.n / recordSize
		buf := bufs[ch.bufIdx]
		for i := 0; i < nRecs; i++ {
			rb := buf[i*recordSize : (i+1)*recordSize]
			idx := ch.recordIndex + uint64(i)
			baseRef, perr := parseInto(rb, recordSize, &entry, mode)
			if perr != nil {
				errs++
				continue
			}
			parsed++
			cb(idx, &entry, baseRef)
		}
		free <- ch.bufIdx
	}
	return parsed, errs
}

// -------------------------------------------------------------------------
// Windows API target / child resolution
// -------------------------------------------------------------------------

// getMFTIdxFromPath returns the MFT record index of the file or directory at
// path. CreateFile + GetFileInformationByHandle gives us the volume-internal
// identity in FileIndexLow/High; the lower 48 bits match the MFT index the
// raw $FILE_NAME parser would produce.
//
// FILE_FLAG_OPEN_REPARSE_POINT prevents CreateFile from following reparse
// points (junctions, symlinks, volume mount points). Without it, opening a
// volume mount point like C:\d-mount returns the file ID of the *target*
// (the root of the mounted volume), which lives in a different MFT — using
// that index against the source volume's MFT collides with arbitrary
// records and silently misattributes their sizes. We always want the
// placeholder's own idx on the volume being scanned.
func getMFTIdxFromPath(path string) (uint64, error) {
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(
		pw,
		0, // metadata only
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return 0, fmt.Errorf("CreateFile(%q): %w", path, err)
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return 0, err
	}
	return MFTIndex(uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)), nil
}

// childInfo pairs an immediate-child directory's display name with its MFT idx.
type childInfo struct {
	name    string
	idx     uint64
	reparse bool
}

// enumerateImmediateChildren returns the immediate child directories of
// targetDir with their MFT indices, via FindFirstFile + per-child handle
// lookup. ~80 syscalls for a typical Windows root — milliseconds total.
func enumerateImmediateChildren(targetDir string) ([]childInfo, error) {
	pattern := strings.TrimSuffix(targetDir, `\`) + `\*`
	pw, err := windows.UTF16PtrFromString(pattern)
	if err != nil {
		return nil, err
	}
	var fd windows.Win32finddata
	h, err := windows.FindFirstFile(pw, &fd)
	if err != nil {
		return nil, fmt.Errorf("FindFirstFile(%q): %w", pattern, err)
	}
	defer windows.FindClose(h)

	var out []childInfo
	for {
		if fd.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
			name := windows.UTF16ToString(fd.FileName[:])
			if name != "." && name != ".." {
				childPath := strings.TrimSuffix(targetDir, `\`) + `\` + name
				if idx, err := getMFTIdxFromPath(childPath); err == nil {
					reparse := fd.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
					out = append(out, childInfo{name: name, idx: idx, reparse: reparse})
				}
			}
		}
		err := windows.FindNextFile(h, &fd)
		if err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return out, err
		}
	}
	return out, nil
}
