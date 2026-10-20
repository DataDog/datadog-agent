// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"context"
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

// Result is the disk-usage breakdown for a target directory: the overall
// subtree total plus, when requested, a depth-limited directory tree, the
// largest files and extensions, any --find matches, and scan diagnostics.
type Result struct {
	// Target is the absolute path the scan was asked to analyze.
	Target string

	// Subtree is the deduplicated total of all bytes attributed to Target's
	// subtree. A file hardlinked into multiple directories counts once toward
	// Subtree, but contributes to each directory it is linked into.
	Subtree int64

	// MultiParentFiles counts in-scope files reachable via more than one
	// distinct parent directory within the scanned subtree (hardlinks spanning
	// multiple in-scope locations). Diagnostic only — not part of the ntfs-du
	// output.
	MultiParentFiles int

	// Pass diagnostics: parsed/error counts and durations.
	// Pass1 builds dirParent + extSize/extParents from a single MFT scan
	// (modeAll). Pass2 tallies file bases (modeFileBaseOnly).
	RecordsParsed int
	ParseErrors   int
	Pass1, Pass2  time.Duration
	Wall          time.Duration

	// Scan-completeness signals. ReadErrors counts MFT chunks that could not
	// be read from the raw volume (e.g. a bad sector or transient I/O error);
	// their records are skipped, so folder/file/size totals undercount.
	// SkippedRecords is the approximate number of MFT records in those chunks.
	// The command reports these on stderr and in its JSON, but still exits 0: a
	// healthy volume routinely has a few unreadable chunks, so failing would make
	// nearly every genuine scan look like an error.
	ReadErrors     int
	SkippedRecords int

	// UnmappedMFTRecords counts records whose location in the $MFT could not be
	// determined at all, so they were never read. This is a separate cause from
	// ReadErrors: the map itself is incomplete rather than a read having failed.
	// UnreachableMFTExtensions is the number of $MFT extension records behind it,
	// and may be nonzero while UnmappedMFTRecords is 0 when an unresolved record
	// turned out to describe an already-covered range.
	UnmappedMFTRecords       int
	UnreachableMFTExtensions int

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
	Files    int   // files in this subtree (same walk-up / hardlink-dedup rules as Size)
	Dirs     int   // descendant directories in this subtree (excludes this node)
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
	// found in the in-scope subtree. Tracked via a min-heap during pass 2;
	// hot-path cost is one int comparison per file plus, for the few that
	// qualify, basename decode. Path resolution happens once after the scan
	// via OpenFileByID.
	TopFiles int

	// TopExtensions, when > 0, populates Result.TopExtensions with the top-N
	// file extensions ranked by aggregated size. Extensions whose aggregated
	// total is below MinFileSize are dropped. Adds ~16 bytes of UTF-16 scanning
	// per file in pass 2 — opt-in for that reason.
	TopExtensions int

	// MinFileSize is the size floor. Files strictly smaller are not considered
	// for the TopFiles heap or for any Finds predicate; extensions whose
	// aggregated total is below it are dropped from TopExtensions. Useful to
	// focus on large space consumers. It does NOT affect Subtree. 0 = no floor.
	MinFileSize int64

	// Finds is the list of independent file-matching predicates to evaluate
	// during the scan. Each entry becomes its own per-query slot with its
	// own Limit and result block in Result.FindResults; queries do not
	// compete with each other for capacity. See FindQuery for the per-type
	// Value syntax (ext / glob / regex).
	Finds []FindQuery

	// Exclude is a list of absolute paths whose subtrees should be excluded
	// from the scan totals. Each path is resolved to an MFT idx upfront; any
	// directory whose ancestor chain includes one of these is treated as
	// out-of-scope. Files in excluded subtrees do not count toward the subtree
	// total, any node's totals, the top-files heap, or the extension
	// aggregation.
	Exclude []string

	// TreeDepth controls Result.Tree. 0 leaves Tree nil (only the Subtree total
	// and top-files/extensions are produced). 1 returns the root plus its
	// immediate children (the fast path). N >= 2 returns the root plus every
	// directory at depth 1..N. Files at depths beyond TreeDepth still count —
	// their bytes/counts roll up into the deepest in-tree ancestor.
	TreeDepth int

	// TreeMinSize hides any tree node whose cumulative size is below this
	// threshold from its parent's Children. Has no effect when TreeDepth is 0.
	// The root (target) is always included. 0 = show every populated node. The
	// threshold only affects which nodes are listed, not the totals on the
	// nodes that are.
	TreeMinSize int64
}

// bucketByIdx sentinel values for the fast path (TreeDepth <= 1): a directory
// resolves either to a child bucket index (>= 0), to the target itself, or to
// out-of-scope.
const (
	bucketOutside = -1
	bucketTarget  = -2
)

// scanState carries the working set shared across the phases of a single Scan.
// Scan constructs one and drives the phases as methods; the fields are the
// intermediate maps/slices that cross phase boundaries. Each map is set to nil
// as soon as a phase no longer needs it (see mapDirsToSizeAccumulatorsFast,
// buildTreeGeneral, and finalize) to bound peak memory — the same discipline
// the original monolithic Scan used.
type scanState struct {
	opts  Options
	res   *Result
	start time.Time

	abs   string // target: absolute, upcased drive, trailing '\'
	drive string // e.g. "C"

	hVol       windows.Handle
	vol        *volumeInfo
	mftExtents []extent

	targetIdx    uint64
	volumeSerial uint32              // target volume serial; exclusions must match it
	children     []childInfo         // immediate child dirs (sorted; name + idx)
	bucketByIdx  map[uint64]int      // child idx → bucket index
	excludedIdxs map[uint64]struct{} // --exclude paths resolved to idxs
	// enumerateChildren is nil in production, which selects
	// enumerateImmediateChildren. Tests inject it to pin tree-disabled behavior
	// without needing a raw-volume scan.
	enumerateChildren func(context.Context, string) ([]childInfo, error)

	// Pass 1 output.
	dirParent  map[uint64]uint64              // dir idx → parent idx
	extData    map[uint64]selectedDataSummary // base idx → selected $DATA sizes on ext records
	extParents map[uint64][]uint64            // base idx → $FILE_NAME parents from ext records

	// Fast path (TreeDepth <= 1).
	dirBucket   map[uint64]int // dir idx → bucket (child index, bucketTarget, or bucketOutside)
	bucketDirs  []int          // per-child descendant-dir count (incl. the child itself)
	subtreeDirs int            // total descendant dirs (root total)

	// General path (TreeDepth >= 2).
	anchorTotals  map[uint64]int64 // tree dir idx → cumulative bytes
	anchorFiles   map[uint64]int   // tree dir idx → cumulative file count
	treeDirsDepth map[uint64]int16 // tree dir idx → depth from target
	// dirName holds decoded names for displayed tree dirs only. It is
	// populated by pass 2's opportunistic name capture (keyed by the
	// placeholder entries mapDirsToSizeAccumulatorsTree pre-seeds), NOT for
	// every dir on the volume — the bulk walk never decodes UTF-16 names,
	// preserving the project's allocation discipline (saves ~25-30 MiB peak).
	dirName map[uint64]string

	// Pass 2 accumulators.
	bucketTotals []int64 // fast path: per-child byte totals
	bucketFiles  []int   // fast path: per-child file counts
	subtree      int64   // deduplicated in-scope byte total (root)
	subtreeFiles int     // fast path: in-scope file total (root)
	multiParent  int     // files reachable via >= 2 distinct in-scope parents

	// Finalize aggregators.
	topF    *topFiles
	extAgg  *extAggregator
	matcher *matchSet
}

// Scan computes disk usage per immediate child of targetDir on the volume
// containing targetDir. Requires Administrator privileges (raw \\.\<drive>:
// open). The context is honored between bounded MFT chunks and during
// directory-metadata preparation; cancellation aborts the scan with ctx.Err().
//
// targetDir and every Options.Exclude must be absolute; a relative path is an
// error rather than being resolved here. Anchoring belongs to the caller, which
// knows the authority to use — for the shell that is its own working directory,
// never the host process cwd.
//
// Scan is an orchestrator: it constructs a scanState and runs the pipeline
// phases in order (normalize → open → resolve → pass 1 → map dirs to size
// accumulators → pass 2 → build tree → finalize). Each phase is a method on
// scanState; see the method bodies for the details of each step.
func Scan(ctx context.Context, targetDir string, opts Options) (*Result, error) {
	s := &scanState{opts: opts, res: &Result{}}
	if err := s.normalizeTarget(targetDir); err != nil {
		return nil, err
	}
	s.start = time.Now()
	// Must precede openTargetVolume: it decides which volume gets opened.
	if err := s.resolveTargetVolume(); err != nil {
		return nil, err
	}
	if err := s.openTargetVolume(); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(s.hVol)
	if err := s.resolveScopeIndices(ctx); err != nil {
		return nil, err
	}
	if err := s.runPass1(ctx); err != nil {
		return nil, err
	}
	if err := s.mapDirsToSizeAccumulators(ctx); err != nil {
		return nil, err
	}
	if err := s.runPass2(ctx); err != nil {
		return nil, err
	}
	if err := s.buildTree(ctx); err != nil {
		return nil, err
	}
	s.finalize()
	return s.res, nil
}

// normalizeTarget cleans targetDir into an upcased-drive path with a trailing
// backslash and records it on the result.
func (s *scanState) normalizeTarget(targetDir string) error {
	if !filepath.IsAbs(targetDir) {
		return fmt.Errorf("target must be an absolute path: %q", targetDir)
	}
	abs := upcaseDriveLetter(filepath.Clean(targetDir))
	if !strings.HasSuffix(abs, `\`) {
		abs += `\`
	}
	if !isLocalDrivePath(abs) {
		return fmt.Errorf("target must be a local drive-letter path: %q", abs)
	}
	s.abs = abs
	s.drive = abs[:1]
	s.res.Target = abs
	return nil
}

// resolveTargetVolume decides which volume the scan actually reads, and resolves
// the target's index while it has the handle open.
//
// The drive letter in a path does not decide where that path lives: reparse points
// in intermediate components are always traversed, so `C:\link\sub` can sit on D:.
// Opening the letter's volume would then look the target's index up in the wrong
// MFT and match unrelated records. Derive the drive from the resolved path instead.
//
// The final component is still not followed, so a reparse point named directly as
// the target reports itself (with no children in this MFT) rather than its
// destination — matching du's default of not dereferencing operands.
func (s *scanState) resolveTargetVolume() error {
	idx, serial, resolved, err := resolvePathLocation(s.abs)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}
	resolved = upcaseDriveLetter(stripExtendedPathPrefix(resolved))
	if !isLocalDrivePath(resolved) {
		return fmt.Errorf("target %q resolves to %q, which is not a local NTFS volume", s.abs, resolved)
	}
	s.drive = resolved[:1]
	s.targetIdx = idx
	s.volumeSerial = serial
	return nil
}

// openTargetVolume opens the raw NTFS volume device for the target's drive and
// resolves the $MFT extents. The volume handle is closed by Scan (defer).
func (s *scanState) openTargetVolume() error {
	hVol, vol, err := openVolume(s.drive)
	if err != nil {
		return err
	}
	s.hVol = hVol
	s.vol = vol

	s.res.TotalMFTRecords = vol.mftValidBytes / int64(vol.recordSize)
	s.res.MFTBytes = vol.mftValidBytes

	mftExtents, gaps, err := getMFTExtents(func(buf []byte, off int64) error {
		return readAt(s.hVol, buf, off)
	}, vol)
	if err != nil {
		return fmt.Errorf("MFT extents: %w", err)
	}
	s.mftExtents = mftExtents
	s.res.UnreachableMFTExtensions = gaps.unreachableExtensions
	s.res.UnmappedMFTRecords = int(gaps.unmappedBytes / int64(vol.recordSize))
	return nil
}

// resolveScopeIndices resolves the scan target, its immediate children, and the
// exclusion paths to their MFT record indices — all via the Windows API. This is
// the only place names are touched in the entire scan; the bulk MFT walk never
// decodes UTF-16 names. The resolved indices are what every later phase keys on.
func (s *scanState) resolveScopeIndices(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Pass abs with its trailing backslash. Stripping it on a drive root
	// (e.g. "C:\" → "C:") is fatal: CreateFile("C:") opens the per-process
	// current directory on drive C:, not the volume root — every subtree
	// rooted at cwd then gets misattributed as "loose" during the C:\ scan.
	// CreateFile + FILE_FLAG_BACKUP_SEMANTICS handles "C:\" and "C:\dir\"
	// equivalently for non-root paths.
	// s.targetIdx and s.volumeSerial were established by resolveTargetVolume, which
	// had to resolve the path anyway to pick the volume.
	var children []childInfo
	if s.opts.TreeDepth > 0 {
		enumerate := s.enumerateChildren
		if enumerate == nil {
			enumerate = enumerateImmediateChildren
		}
		var err error
		children, err = enumerate(ctx, s.abs)
		if err != nil {
			return fmt.Errorf("enumerate children: %w", err)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return strings.ToLower(children[i].name) < strings.ToLower(children[j].name)
	})
	s.children = children

	s.bucketByIdx = make(map[uint64]int, len(children))
	for i, c := range children {
		s.bucketByIdx[c.idx] = i
	}

	// Resolve exclusion paths to MFT idxs. We do this BEFORE pass 1 so that
	// walkUp can short-circuit excluded subtrees as bucketOutside without
	// any per-file cost in passes 2/3.
	s.excludedIdxs = make(map[uint64]struct{}, len(s.opts.Exclude))
	for _, p := range s.opts.Exclude {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A relative exclude is a caller bug, not a maybe-missing path, so it is
		// rejected rather than skipped.
		if !filepath.IsAbs(p) {
			return fmt.Errorf("exclude must be an absolute path: %q", p)
		}
		ap := upcaseDriveLetter(filepath.Clean(p))

		// Refuse non-local paths before opening them (see isLocalDrivePath).
		if !isLocalDrivePath(ap) {
			return fmt.Errorf("exclude %q: only local drive-letter paths are supported", p)
		}
		// The drive letter is deliberately NOT compared here. It is not a reliable
		// proxy once reparse points are involved: an exclude typed under a junction
		// resolves onto the target's volume despite carrying a different letter, and
		// one typed with the resolved letter carries a letter the target's path never
		// had. The volume serial below is the real test.

		idx, serial, resolved, err := resolvePathLocation(ap)
		if err != nil {
			// A missing exclude is fine — pre-emptively excluding a path that may
			// or may not exist yet is a supported use.
			continue
		}
		// A file index means nothing outside its own volume: left unchecked it would
		// collide with an unrelated record here and exclude the WRONG directory.
		if serial != s.volumeSerial {
			return fmt.Errorf("exclude %q resolves to %q, which is not on the scanned volume (%s:)",
				p, upcaseDriveLetter(stripExtendedPathPrefix(resolved)), s.drive)
		}
		s.excludedIdxs[idx] = struct{}{}
	}
	s.res.ExcludedDirs = len(s.excludedIdxs)
	return nil
}

// runPass1 builds dirParent + extSize/extParents in one MFT scan (modeAll).
// For each in-use record:
//   - directory base: record idx → parent in dirParent
//   - directory whose $FILE_NAME spilled to an extension: stash via
//     pendingExtParent / dirsAwaitingParent and reconcile end-of-pass
//   - extension record: accumulate $DATA size into extSize[baseRef]
//     and append $FILE_NAME parents into extParents[baseRef]
//
// Folding extension-record accumulation into this pass costs nothing extra:
// modeAll already fully parses every record. It eliminates a second full MFT
// scan that would otherwise re-stream the same bytes.
func (s *scanState) runPass1(ctx context.Context) error {
	t1 := time.Now()

	// Map size hint: ~1 directory per ~5 records on typical Windows volumes.
	// Overestimating costs nothing (Go's map shrinks unused buckets); under-
	// estimating triggers ~lg(N) rehashes during pass 1.
	dirHint := int(s.res.TotalMFTRecords / 5)
	s.dirParent = make(map[uint64]uint64, dirHint)
	pendingExtParent := make(map[uint64]uint64)
	dirsAwaitingParent := make(map[uint64]struct{})

	// Hint: typical Windows volumes have ~1.3% of MFT records as extensions.
	extHint := int(s.res.TotalMFTRecords / 70)
	s.extData = make(map[uint64]selectedDataSummary, extHint)
	s.extParents = make(map[uint64][]uint64, extHint)

	parsed1, errs1, readErrs1, skipped1 := streamPipelined(ctx, s.hVol, s.mftExtents, s.vol.recordSize, modeAll, func(idx uint64, e *mftEntry, baseRef uint64) {
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
			s.addExtensionData(baseRef, e)
			s.extParents[baseRef] = append(s.extParents[baseRef], e.hardlinkParents...)

			// Reconcile dir parent when $FILE_NAME spilled to an extension record.
			if e.primaryParent == 0 {
				return // no parent on this extension; nothing to stash or satisfy
			}
			if _, awaiting := dirsAwaitingParent[baseRef]; awaiting {
				// Base dir was seen first without a parent — apply now.
				s.dirParent[baseRef] = e.primaryParent
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
			s.dirParent[idx] = e.primaryParent
			delete(pendingExtParent, idx)
			return
		}
		// Dir base with no $FILE_NAME (overflowed to ext) — recover from
		// stash if seen, else mark awaiting.
		if p, ok := pendingExtParent[idx]; ok {
			s.dirParent[idx] = p
			delete(pendingExtParent, idx)
			return
		}
		dirsAwaitingParent[idx] = struct{}{}
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	s.res.Pass1 = time.Since(t1)
	s.res.RecordsParsed += parsed1
	s.res.ParseErrors += errs1
	s.recordReadErrors(readErrs1, skipped1)

	// End-of-pass reconciliation for dirs whose ext arrived after the base.
	for idx := range dirsAwaitingParent {
		if p, ok := pendingExtParent[idx]; ok {
			s.dirParent[idx] = p
		}
	}
	// pendingExtParent and dirsAwaitingParent are function-local; they become
	// garbage-collectable when runPass1 returns (before pass 2 runs), so no
	// explicit nil is needed to bound peak memory.
	return nil
}

// mapDirsToSizeAccumulators assigns every directory on the volume to the size
// accumulator(s) that will tally its subtree's bytes and files during pass 2.
// An accumulator is just a running total keyed by a directory index: pass 2
// looks up (or walks to) the accumulator for each file's parent and adds the
// file's size there, so the per-file hot path never has to recompute where a
// file sits relative to the scan target. Building the mapping once here —
// instead of per file — is the whole point: pass 2 processes millions of records.
//
// The mapping takes one of two shapes depending on how deep a tree the caller
// asked for:
//   - TreeDepth <= 1 (fast path, mapDirsToSizeAccumulatorsFast): every dir maps
//     to exactly one accumulator — the top-level child of the target it descends
//     from (or the target itself / out-of-scope), recorded in dirBucket. Pass 2
//     attributes a file with a single O(1) dirBucket[parent] lookup, no per-file
//     ancestor walk.
//   - TreeDepth >= 2 (general path, mapDirsToSizeAccumulatorsTree): the
//     accumulators are the in-tree ancestor directories (depth <= TreeDepth),
//     held in anchorTotals. A file belongs to every such ancestor, so pass 2
//     walks the file's dirParent chain and adds its size to each anchor along
//     the way.
func (s *scanState) mapDirsToSizeAccumulators(ctx context.Context) error {
	if s.opts.TreeDepth <= 1 {
		return s.mapDirsToSizeAccumulatorsFast(ctx)
	} else {
		return s.mapDirsToSizeAccumulatorsTree(ctx)
	}
}

// mapDirsToSizeAccumulatorsFast is the TreeDepth <= 1 mapping. It resolves every
// dir to a single accumulator — the top-level child of the target it descends
// from (bucket index >= 0), the target itself (bucketTarget), or out-of-scope
// (bucketOutside) — by memoizing walkUp of each dir's parent chain into
// dirBucket. It also tallies the per-child and root descendant-directory counts.
// dirParent is freed on the way out: the fast path's pass 2 needs only dirBucket.
func (s *scanState) mapDirsToSizeAccumulatorsFast(ctx context.Context) error {
	s.dirBucket = make(map[uint64]int, len(s.dirParent))
	s.dirBucket[s.targetIdx] = bucketTarget
	for idx, b := range s.bucketByIdx {
		s.dirBucket[idx] = b
	}
	for idx := range s.excludedIdxs {
		if idx == s.targetIdx {
			continue
		}
		s.dirBucket[idx] = bucketOutside
	}
	var walkUp func(idx uint64, depth int) int
	walkUp = func(idx uint64, depth int) int {
		if depth > 512 {
			return bucketOutside
		}
		if b, ok := s.dirBucket[idx]; ok {
			return b
		}
		p, ok := s.dirParent[idx]
		if !ok {
			s.dirBucket[idx] = bucketOutside
			return bucketOutside
		}
		b := walkUp(p, depth+1)
		s.dirBucket[idx] = b
		return b
	}
	for idx := range s.dirParent {
		if err := ctx.Err(); err != nil {
			return err
		}
		walkUp(idx, 0)
	}
	// Tally directories per child, mirroring the byte walk-up: every dir
	// resolves (via dirBucket) to the top-level child it descends from.
	// bucketDirs[i] includes child i itself (assembly subtracts it so a
	// child's reported Dirs is descendants only); subtreeDirs is the total
	// descendant-dir count for the root.
	s.bucketDirs = make([]int, len(s.children))
	for idx, b := range s.dirBucket {
		if err := ctx.Err(); err != nil {
			return err
		}
		if idx == s.targetIdx {
			continue
		}
		if b >= 0 {
			s.bucketDirs[b]++
			s.subtreeDirs++
		}
	}
	s.dirParent = nil
	return nil
}

// mapDirsToSizeAccumulatorsTree is the TreeDepth >= 2 mapping. Here the
// accumulators are the in-tree ancestor dirs (depth <= TreeDepth from the
// target), because a file's bytes count toward every ancestor node shown in the
// tree. It first labels each dir with its depth from the target (or
// out-of-scope), then retains only the in-tree dirs as anchors in treeDirsDepth
// / anchorTotals / anchorFiles / dirName. dirParent stays alive — pass 2 walks
// each file's chain to reach these anchors.
func (s *scanState) mapDirsToSizeAccumulatorsTree(ctx context.Context) error {
	// depthByIdx labels every dir with its depth from the target (0 = target,
	// -1 = out-of-scope). It is intentionally short-lived — the same memory
	// shape as dirBucket would have been — but we keep only the small in-tree
	// subset afterward as anchors and let the rest be reclaimed when this
	// function returns, before pass 2 runs.
	depthByIdx := make(map[uint64]int16, len(s.dirParent))
	depthByIdx[s.targetIdx] = 0
	for idx := range s.excludedIdxs {
		if idx != s.targetIdx {
			depthByIdx[idx] = -1
		}
	}
	var walkDepth func(idx uint64, recurse int) int16
	walkDepth = func(idx uint64, recurse int) int16 {
		if d, ok := depthByIdx[idx]; ok {
			return d
		}
		if recurse > 512 {
			depthByIdx[idx] = -1
			return -1
		}
		p, ok := s.dirParent[idx]
		if !ok {
			depthByIdx[idx] = -1
			return -1
		}
		pd := walkDepth(p, recurse+1)
		if pd < 0 {
			depthByIdx[idx] = -1
			return -1
		}
		d := pd + 1
		depthByIdx[idx] = d
		return d
	}
	for idx := range s.dirParent {
		if err := ctx.Err(); err != nil {
			return err
		}
		walkDepth(idx, 0)
	}

	// Retain the in-tree dirs (depth <= TreeDepth) as anchors. Pre-seed
	// anchorTotals with zero entries — pass 2's chain walk uses map presence to
	// know whether a given ancestor is an anchor it should accumulate into.
	nTree := 0
	for _, d := range depthByIdx {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d >= 0 && d <= int16(s.opts.TreeDepth) {
			nTree++
		}
	}
	s.treeDirsDepth = make(map[uint64]int16, nTree)
	s.anchorTotals = make(map[uint64]int64, nTree)
	s.anchorFiles = make(map[uint64]int, nTree)
	s.dirName = make(map[uint64]string, nTree)
	for idx, d := range depthByIdx {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d >= 0 && d <= int16(s.opts.TreeDepth) {
			s.treeDirsDepth[idx] = d
			s.anchorTotals[idx] = 0
			s.dirName[idx] = ""
		}
	}
	return nil
}

// runPass2 streams file base records and tallies each in-scope file's size.
// modeFileBaseOnly skips dirs and extensions before the attribute walk; the
// tally happens immediately with no per-file map or slice allocation. The
// per-record callback resolves the file size then dispatches to tallyFileFast
// (depth <= 1) or tallyFileGeneral (depth >= 2).
func (s *scanState) runPass2(ctx context.Context) error {
	t2 := time.Now()

	// bucketTotals/bucketFiles are the per-child attribution slices, used only
	// by the fast path (depth <= 1). The general path (depth >= 2) accumulates
	// into anchorTotals/anchorFiles instead and leaves these nil.
	if s.opts.TreeDepth <= 1 {
		s.bucketTotals = make([]int64, len(s.children))
		s.bucketFiles = make([]int, len(s.children))
	}

	s.topF = newTopFiles(s.opts.TopFiles, s.opts.MinFileSize)
	s.extAgg = newExtAggregator(s.opts.TopExtensions > 0)
	matcher, err := newMatchSet(s.opts.Finds, s.opts.MinFileSize)
	if err != nil {
		return err
	}
	s.matcher = matcher

	// Pass 2 mode: when TreeDepth > 0 we use modeAll so dir base records flow
	// through the callback for opportunistic name capture. Otherwise
	// modeFileBaseOnly skips dirs/extensions before the attribute walk (saves
	// ~25-30% of pass 2 wall when name capture isn't needed).
	pass2Mode := modeFileBaseOnly
	if s.opts.TreeDepth >= 2 {
		pass2Mode = modeAll
	}
	fast := s.opts.TreeDepth <= 1

	parsed2, errs2, readErrs2, skipped2 := streamPipelined(ctx, s.hVol, s.mftExtents, s.vol.recordSize, pass2Mode, func(idx uint64, e *mftEntry, baseRef uint64) {
		if !e.isInUse || baseRef != 0 || idx <= maxMetafileMFTIndex {
			return
		}
		if e.isDir {
			// Dir base record: in tree mode, capture name only for tree dirs
			// (dirName has a placeholder entry pre-seeded). In depth=0 mode this
			// branch never fires (modeFileBaseOnly skips dirs at the parser level).
			if s.dirName != nil {
				if _, want := s.dirName[idx]; want && len(e.nameBytes) > 0 {
					s.dirName[idx] = decodeUTF16Name(e.nameBytes)
				}
			}
			return
		}
		sz, ok := s.resolveFileSize(idx, e)
		if !ok {
			return
		}

		if fast {
			s.tallyFileFast(idx, e, sz)
		} else {
			s.tallyFileGeneral(idx, e, sz)
		}
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	s.res.Pass2 = time.Since(t2)
	s.res.RecordsParsed += parsed2
	s.res.ParseErrors += errs2
	s.recordReadErrors(readErrs2, skipped2)
	return nil
}

// addExtensionData folds an extension record into its base file's summary.
// A valid file has at most one unnamed $DATA start across the entire chain.
// Conflicts are marked so pass 2 omits that file and accounts it as a parse
// error, matching the scan's existing treatment of untrustworthy metadata.
func (s *scanState) addExtensionData(baseRef uint64, e *mftEntry) {
	eData := e.data.selectSize(s.opts.ShowApparent)
	if eData.unnamedStarts == 0 && eData.named == 0 {
		return
	}
	d := s.extData[baseRef]
	d.named = saturatingAdd(d.named, eData.named)
	if d.unnamedStarts == 0 && eData.unnamedStarts != 0 {
		d.unnamed = eData.unnamed
	}
	if eData.unnamedStarts > ^uint8(0)-d.unnamedStarts {
		d.unnamedStarts = ^uint8(0)
	} else {
		d.unnamedStarts += eData.unnamedStarts
	}
	s.extData[baseRef] = d
}

// resolveFileSize merges the default stream separately from named streams.
// $FILE_NAME is only a fallback when no actual unnamed $DATA start was parsed
// from either the base record or its extensions.
func (s *scanState) resolveFileSize(idx uint64, e *mftEntry) (int64, bool) {
	ext := s.extData[idx]
	base := e.data.selectSize(s.opts.ShowApparent)
	fallback := e.fnAllocSize
	if s.opts.ShowApparent {
		fallback = e.fnDataSize
	}
	sz, ok := resolveSelectedDataSize(base, ext, fallback)
	if !ok {
		s.res.ParseErrors++
		return 0, false
	}
	return sz, true
}

// recordReadErrors folds one pass's unreadable-chunk counts into the result.
// Both passes traverse the identical MFT extents, so a chunk unreadable in one
// pass is unreadable in both; take the max rather than summing to avoid
// double-counting the same physical read failure across the two passes.
func (s *scanState) recordReadErrors(readErrs, skipped int) {
	if readErrs > s.res.ReadErrors {
		s.res.ReadErrors = readErrs
		s.res.SkippedRecords = skipped
	}
}

// tallyFileFast attributes one file's size via O(1) dirBucket lookups (the
// depth<=1 fast path). It collects the file's distinct in-scope parents (for
// the MultiParentFiles diagnostic) and distinct in-scope buckets (for per-child
// size attribution) in one pass. Files directly under the target map to
// bucketTarget: they count toward the root totals (subtree/subtreeFiles) but not
// toward any child bucket — there is no separate "loose" concept.
//
// The pInline/bInline scratch arrays are function-local so they stay on the
// stack (zero per-file heap allocation).
func (s *scanState) tallyFileFast(idx uint64, e *mftEntry, sz int64) {
	var pInline [8]uint64
	parents := pInline[:0]
	var bInline [8]int
	buckets := bInline[:0]
	visit := func(p uint64) {
		b, ok := s.dirBucket[p]
		if !ok || b == bucketOutside {
			return
		}
		seenP := false
		for _, x := range parents {
			if x == p {
				seenP = true
				break
			}
		}
		if !seenP {
			parents = append(parents, p)
		}
		seenB := false
		for _, x := range buckets {
			if x == b {
				seenB = true
				break
			}
		}
		if !seenB {
			buckets = append(buckets, b)
		}
	}
	for _, p := range e.hardlinkParents {
		visit(p)
	}
	if pp, ok := s.extParents[idx]; ok {
		for _, p := range pp {
			visit(p)
		}
	}
	if len(buckets) == 0 && e.primaryParent != 0 {
		visit(e.primaryParent)
	}
	if len(buckets) == 0 {
		return // not in scope — also skips top-N / extAgg / matcher
	}
	s.subtree = saturatingAdd(s.subtree, sz)
	s.subtreeFiles++
	if len(parents) > 1 {
		s.multiParent++
	}
	for _, b := range buckets {
		if b >= 0 {
			s.bucketTotals[b] = saturatingAdd(s.bucketTotals[b], sz)
			s.bucketFiles[b]++
		}
	}
	// Top-N / extAgg / matcher fire only for in-scope files.
	s.topF.consider(idx, e, sz)
	s.extAgg.addFromName(e.nameBytes, sz)
	s.matcher.consider(idx, e, sz)
}

// tallyFileGeneral walks dirParent per parent ref (the depth>=2 general path),
// accumulating the file's size into every tree-dir ancestor in the last
// TreeDepth+1 chain entries. The walk terminates at target (in scope) or by
// exhausting dirParent (out of scope). Dedup across hardlink parents via a
// small stack-allocated set.
//
// The seenInline/chainScratch/parentInline scratch arrays are function-local so
// they stay on the stack (zero per-file heap allocation).
func (s *scanState) tallyFileGeneral(idx uint64, e *mftEntry, sz int64) {
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
	var parentInline [16]uint64
	inScopeParents := parentInline[:0] // distinct in-scope parents (MultiParentFiles)
	anyInScope := false
	attribute := func(parentIdx uint64) {
		chain := chainScratch[:0]
		cur := parentIdx
		reached := false
		for steps := 0; steps < 512; steps++ {
			if _, ex := s.excludedIdxs[cur]; ex {
				return
			}
			chain = append(chain, cur)
			if cur == s.targetIdx {
				reached = true
				break
			}
			p, ok := s.dirParent[cur]
			if !ok {
				return
			}
			cur = p
		}
		if !reached {
			return
		}
		anyInScope = true
		// Track distinct in-scope parents for the MultiParentFiles diagnostic
		// (a file with two links in the same directory has one distinct parent
		// and is not multi-parent).
		seenParent := false
		for _, x := range inScopeParents {
			if x == parentIdx {
				seenParent = true
				break
			}
		}
		if !seenParent {
			inScopeParents = append(inScopeParents, parentIdx)
		}
		chainLen := len(chain)
		start := chainLen - 1 - s.opts.TreeDepth
		if start < 0 {
			start = 0
		}
		for i := start; i < chainLen; i++ {
			if addUnique(chain[i]) {
				s.anchorTotals[chain[i]] = saturatingAdd(s.anchorTotals[chain[i]], sz)
				s.anchorFiles[chain[i]]++
			}
		}
	}
	for _, p := range e.hardlinkParents {
		attribute(p)
	}
	if parents, ok := s.extParents[idx]; ok {
		for _, p := range parents {
			attribute(p)
		}
	}
	if !anyInScope {
		return // out of scope — skip top-N / extAgg / matcher
	}
	s.subtree = saturatingAdd(s.subtree, sz)
	if len(inScopeParents) > 1 {
		s.multiParent++
	}
	// Top-N / extAgg / matcher fire only for in-scope files.
	s.topF.consider(idx, e, sz)
	s.extAgg.addFromName(e.nameBytes, sz)
	s.matcher.consider(idx, e, sz)
}

// buildTree assembles Result.Tree. The general path (depth >= 2) builds it from
// anchorTotals by inverting dirParent over the tree dirs; the fast path
// synthesizes a root+children tree from the per-child tallies at depth 1, or
// leaves Tree nil at depth 0 (the subtree total is in Subtree).
func (s *scanState) buildTree(ctx context.Context) error {
	if s.opts.TreeDepth >= 2 {
		return s.buildTreeGeneral(ctx)
	} else if s.opts.TreeDepth == 1 {
		return s.buildTreeFastDepth1(ctx)
	}
	// TreeDepth == 0: res.Tree stays nil (the subtree total is in res.Subtree).
	return nil
}

// buildTreeGeneral builds the nested tree for depth >= 2 from anchorTotals /
// anchorFiles, tallying descendant-dir counts and inverting dirParent for
// parent → children. It frees dirParent when done.
func (s *scanState) buildTreeGeneral(ctx context.Context) error {
	// Tally directories per tree node, mirroring the per-file byte walk in
	// pass 2: each directory contributes to every in-tree ancestor in the last
	// TreeDepth+1 chain entries (dirs beyond TreeDepth roll up into the deepest
	// in-tree ancestor). Directories have a single parent, so no hardlink dedup
	// is needed; a dir counts toward its ancestors, not itself, so Dirs reports
	// descendants. dirParent is still alive here.
	anchorDirs := make(map[uint64]int, len(s.treeDirsDepth))
	var dirChain [32]uint64
	for d := range s.dirParent {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d == s.targetIdx {
			continue
		}
		if _, ex := s.excludedIdxs[d]; ex {
			continue // excluded dir itself is out of scope
		}
		cur, ok := s.dirParent[d]
		if !ok {
			continue
		}
		chain := dirChain[:0]
		reached := false
		for steps := 0; steps < 512; steps++ {
			if _, ex := s.excludedIdxs[cur]; ex {
				break // under an excluded subtree
			}
			chain = append(chain, cur)
			if cur == s.targetIdx {
				reached = true
				break
			}
			p, ok := s.dirParent[cur]
			if !ok {
				break
			}
			cur = p
		}
		if !reached {
			continue
		}
		chainLen := len(chain)
		start := chainLen - 1 - s.opts.TreeDepth
		if start < 0 {
			start = 0
		}
		for i := start; i < chainLen; i++ {
			anchorDirs[chain[i]]++
		}
	}

	childrenByParent := make(map[uint64][]uint64, len(s.treeDirsDepth))
	for idx := range s.treeDirsDepth {
		if err := ctx.Err(); err != nil {
			return err
		}
		if idx == s.targetIdx {
			continue
		}
		parent, ok := s.dirParent[idx]
		if !ok {
			continue
		}
		childrenByParent[parent] = append(childrenByParent[parent], idx)
	}
	reparseByIdx := make(map[uint64]bool, len(s.children))
	for _, c := range s.children {
		reparseByIdx[c.idx] = c.reparse
	}
	var build func(idx uint64, depth int) (*TreeNode, error)
	build = func(idx uint64, depth int) (*TreeNode, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := &TreeNode{
			Idx:     idx,
			Depth:   depth,
			Size:    s.anchorTotals[idx],
			Files:   s.anchorFiles[idx],
			Dirs:    anchorDirs[idx],
			Reparse: reparseByIdx[idx],
		}
		if idx == s.targetIdx {
			n.Name = s.abs
		} else {
			n.Name = s.dirName[idx]
		}
		kids := childrenByParent[idx]
		if len(kids) > 0 {
			n.Children = make([]*TreeNode, 0, len(kids))
			for _, kidIdx := range kids {
				if s.anchorTotals[kidIdx] < s.opts.TreeMinSize {
					continue
				}
				child, err := build(kidIdx, depth+1)
				if err != nil {
					return nil, err
				}
				n.Children = append(n.Children, child)
			}
			sort.SliceStable(n.Children, func(i, j int) bool {
				if n.Children[i].Size != n.Children[j].Size {
					return n.Children[i].Size > n.Children[j].Size
				}
				return n.Children[i].Name < n.Children[j].Name
			})
		}
		return n, nil
	}
	tree, err := build(s.targetIdx, 0)
	if err != nil {
		return err
	}
	s.res.Tree = tree
	s.dirParent = nil
	return nil
}

// buildTreeFastDepth1 synthesizes a root + immediate-child tree from the fast
// path's per-child tallies. Child names come from the API enumeration; the
// child totals are whole-subtree (walkUp attributes every descendant to its
// top-level child). TreeMinSize filters children; the root is always present.
func (s *scanState) buildTreeFastDepth1(ctx context.Context) error {
	root := &TreeNode{
		Idx:   s.targetIdx,
		Depth: 0,
		Name:  s.abs,
		Size:  s.subtree,
		Files: s.subtreeFiles,
		Dirs:  s.subtreeDirs,
	}
	for i, c := range s.children {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ex := s.excludedIdxs[c.idx]; ex {
			continue
		}
		if s.bucketTotals[i] < s.opts.TreeMinSize {
			continue
		}
		dirs := s.bucketDirs[i] - 1 // exclude the child directory itself
		if dirs < 0 {
			dirs = 0
		}
		root.Children = append(root.Children, &TreeNode{
			Idx:     c.idx,
			Depth:   1,
			Name:    c.name,
			Size:    s.bucketTotals[i],
			Files:   s.bucketFiles[i],
			Dirs:    dirs,
			Reparse: c.reparse,
		})
	}
	sort.SliceStable(root.Children, func(i, j int) bool {
		if root.Children[i].Size != root.Children[j].Size {
			return root.Children[i].Size > root.Children[j].Size
		}
		return root.Children[i].Name < root.Children[j].Name
	})
	s.res.Tree = root
	return nil
}

// finalize drops the remaining working maps, records the subtree totals, and
// resolves top-file / extension / find results into the Result.
func (s *scanState) finalize() {
	// Drop the remaining maps before formatting.
	s.extData = nil
	s.extParents = nil
	s.dirBucket = nil
	s.dirName = nil
	s.anchorTotals = nil

	s.res.Subtree = s.subtree
	s.res.MultiParentFiles = s.multiParent

	// Resolve top-file paths via OpenFileByID. Bounded by Options.TopFiles
	// — typically tens to hundreds of syscall pairs. Volume root path is
	// "C:\" form (not the raw \\.\C: device); CreateFile + BACKUP_SEMANTICS
	// is what OpenFileByID needs as its rootDir.
	if s.topF != nil {
		volumeRoot := s.abs[:3] // "C:\"
		s.res.TopFiles = resolveCandidatePaths(volumeRoot, s.topF.drained())
	}
	if s.extAgg != nil {
		s.res.TopExtensions = s.extAgg.topN(s.opts.TopExtensions, s.opts.MinFileSize)
	}
	if s.matcher != nil {
		volumeRoot := s.abs[:3]
		blocks := s.matcher.drained()
		queries := s.matcher.queries()
		s.res.FindResults = make([]FindResultBlock, len(blocks))
		for i, blk := range blocks {
			s.res.FindResults[i] = FindResultBlock{
				Query:   queries[i],
				Matches: resolveCandidatePaths(volumeRoot, blk),
			}
		}
	}

	s.res.Wall = time.Since(s.start)
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

	// Validate the volume layout against the streamer/parser's assumptions.
	// We reject unfamiliar configurations loudly rather than producing
	// silently-wrong byte totals — the failure modes are subtle (mis-aligned
	// record reads, miscounted fixups) and would not be caught by sanity
	// checks downstream.
	if err := validateNTFSLayout(&data); err != nil {
		windows.CloseHandle(h)
		return 0, nil, err
	}

	vol := &volumeInfo{
		recordSize:      int(data.BytesPerFileRecordSegment),
		bytesPerCluster: int64(data.BytesPerCluster),
		totalClusters:   data.TotalClusters,
		mftStartByte:    data.MftStartLcn * int64(data.BytesPerCluster),
		mftValidBytes:   data.MftValidDataLength,
	}
	return h, vol, nil
}

// validateNTFSLayout rejects volume configurations the scanner cannot
// safely handle. Anything we let through has to behave correctly end to
// end; producing wrong totals on an odd layout is worse than failing.
func validateNTFSLayout(data *ntfsVolumeData) error {
	// NTFS multi-sector transfer protection has a fixed 512-byte stride
	// per the on-disk format spec — see MULTI_SECTOR_HEADER in MSDN. It is
	// not derived from BytesPerSector, so 4Kn / Advanced Format volumes
	// use the same stride. We therefore don't validate BytesPerSector;
	// applyFixups' hardcoded 512 is correct on every NTFS volume.
	const mstpStride = 512

	if data.BytesPerCluster == 0 {
		return errors.New("FSCTL_GET_NTFS_VOLUME_DATA returned BytesPerCluster=0")
	}
	if data.TotalClusters <= 0 {
		return fmt.Errorf("FSCTL_GET_NTFS_VOLUME_DATA returned TotalClusters=%d", data.TotalClusters)
	}
	if data.BytesPerFileRecordSegment == 0 {
		return errors.New("FSCTL_GET_NTFS_VOLUME_DATA returned BytesPerFileRecordSegment=0")
	}

	// Upper bounds (defense in depth). These values come from the kernel, but
	// the per-chunk read buffers are sized from BytesPerFileRecordSegment and
	// the data-run decoder scales by BytesPerCluster, so cap both well above any
	// real NTFS geometry (records are 1 KiB, clusters at most 2 MiB) to reject a
	// bogus value before it drives a huge allocation.
	const maxRecordSize = 64 << 10  // 64 KiB
	const maxClusterSize = 16 << 20 // 16 MiB
	if data.BytesPerFileRecordSegment > maxRecordSize {
		return fmt.Errorf(
			"unsupported NTFS layout: BytesPerFileRecordSegment=%d exceeds %d",
			data.BytesPerFileRecordSegment, maxRecordSize,
		)
	}
	if data.BytesPerCluster > maxClusterSize {
		return fmt.Errorf(
			"unsupported NTFS layout: BytesPerCluster=%d exceeds %d",
			data.BytesPerCluster, maxClusterSize,
		)
	}

	// MFT records must be a multiple of the MSTP stride; otherwise
	// applyFixups' sector-end positions don't land inside the record.
	if data.BytesPerFileRecordSegment%mstpStride != 0 {
		return fmt.Errorf(
			"unsupported NTFS layout: BytesPerFileRecordSegment=%d is not a multiple of %d",
			data.BytesPerFileRecordSegment, mstpStride,
		)
	}

	// streamPipelined parses each chunk as a flat array of records and has
	// no machinery to carry partial bytes across extent boundaries. That
	// assumption holds when each MFT record fits within a single cluster
	// (default 4 KiB cluster, 1 KiB record). When clusters are smaller
	// than records, a run of an odd number of clusters would split a
	// record across extents and the streamer would silently miscount.
	if data.BytesPerCluster < data.BytesPerFileRecordSegment {
		return fmt.Errorf(
			"unsupported NTFS layout: BytesPerCluster (%d) < BytesPerFileRecordSegment (%d); "+
				"MFT records may span data run boundaries and cannot be safely read",
			data.BytesPerCluster, data.BytesPerFileRecordSegment,
		)
	}

	return nil
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
) (parsed, errs, readErrs, skipped int) {
	const chunkRecords = 4096
	chunkBytes := chunkRecords * recordSize

	type chunk struct {
		bufIdx      int
		n           int
		recs        int // records this chunk spans (skipped wholesale on read error)
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
			// A range whose disk location is unknown (see mergeMFTSegments). There
			// is nothing to read, but the index must still advance across it so
			// every later record keeps its true index. The loss itself is reported
			// from the extent map, not counted here.
			if ex.byteOffset == unmappedExtent {
				recordIndex += uint64(ex.byteLength / int64(recordSize))
				continue
			}
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

				// One aligned ReadFile per chunk. Windows treats a raw volume handle
				// as unbuffered regardless of the CreateFile flags: reads must begin
				// on a sector boundary and span a whole number of sectors, or the call
				// fails outright — it does not return a resumable partial. extOff is
				// cluster-aligned and toRead is a whole number of MFT records (each a
				// multiple of the 512-byte sector), so every request here is aligned by
				// construction. A short read therefore cannot be safely resumed (the
				// continuation offset would land mid-sector and fail), so we treat it
				// as a chunk error and recover-and-report its records like a bad sector
				// — the same short-read-is-error rule readAt uses above.
				// https://learn.microsoft.com/en-us/windows/win32/fileio/file-buffering
				var ol windows.Overlapped
				ol.Offset = uint32(extOff & 0xFFFFFFFF)
				ol.OffsetHigh = uint32(extOff >> 32)
				var n uint32
				rerr := windows.ReadFile(h, buf, &n, &ol)
				if rerr == nil && int64(n) < toRead {
					rerr = fmt.Errorf("short read: %d < %d", n, toRead)
				}
				recs := int(toRead / int64(recordSize))
				ready <- chunk{bufIdx: bi, n: int(n), recs: recs, recordIndex: recordIndex, err: rerr}
				recordIndex += uint64(recs)
				extOff += toRead
				rem -= toRead
			}
		}
	}()

	var entry mftEntry
	for ch := range ready {
		if ch.err != nil {
			// This chunk's records are unavailable: a raw-volume ReadFile failed or
			// short-read (bad sector, or a partial return that cannot be safely
			// resumed on an unbuffered volume handle), or the range has no mapped
			// disk location at all. Skip its records and keep scanning the rest of
			// the MFT, tracking how much was lost so the caller can report that the
			// totals undercount rather than aborting the whole scan.
			readErrs++
			skipped += ch.recs
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
	return parsed, errs, readErrs, skipped
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
	idx, _, err := getMFTIdxAndVolumeSerial(path)
	return idx, err
}

func getMFTIdxAndVolumeSerial(path string) (uint64, uint32, error) {
	idx, serial, _, err := resolvePathLocation(path)
	return idx, serial, err
}

// resolvePathLocation opens path for metadata only and reports its MFT record
// index, the serial of the volume holding it, and the path the open actually
// resolved to (still \\?\-prefixed).
//
// The resolved path may name a DIFFERENT volume than the drive letter in path.
// FILE_FLAG_OPEN_REPARSE_POINT suppresses reparse processing for the final
// component only; reparse points in intermediate components are always traversed.
// So `C:\link\sub`, where `link` is a junction to `D:\data`, resolves onto D: —
// which is why callers must derive the volume from the resolved path rather than
// from the letter they were given.
//
// A file index identifies a file only within its own volume, so a caller
// interpreting it against a specific volume's MFT must compare the serial too;
// Microsoft prescribes combining the two to decide whether handles name the same
// file. Callers must also reject non-local paths first (see isLocalDrivePath).
// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfileinformationbyhandle
func resolvePathLocation(path string) (idx uint64, serial uint32, resolved string, err error) {
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, "", err
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
		return 0, 0, "", fmt.Errorf("CreateFile(%q): %w", path, err)
	}
	defer windows.CloseHandle(h)

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return 0, 0, "", err
	}
	idx = MFTIndex(uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow))

	// Same three-way return convention as the top-files resolver: 0 is a failure
	// (reported as err), a value below the buffer size is the count written, and one
	// at or above it is the required size.
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return 0, 0, "", fmt.Errorf("GetFinalPathNameByHandle(%q): %w", path, err)
	}
	if n >= uint32(len(buf)) {
		return 0, 0, "", fmt.Errorf("resolved path for %q exceeds %d chars", path, len(buf))
	}
	return idx, info.VolumeSerialNumber, windows.UTF16ToString(buf[:n]), nil
}

// isLocalDrivePath reports whether an already-normalized (filepath.Abs) path names
// a local drive-letter volume ("X:\...").
//
// UNC is the reason this exists: passing \\host\share to CreateFile makes the SMB
// client dial out to a caller-supplied host and authenticate as the current user.
// ntfs-du reads local volumes only, so a non-local path could never be in the
// scanned MFT anyway.
//
// Requiring the drive-letter shape rather than blacklisting prefixes fails closed
// on every other path class:
//
//   - UNC, both spellings: \\host\share and //host/share
//   - device paths: \\?\C:\, \\.\C:\, \\?\UNC\host\share
//   - volume GUID paths: \\?\Volume{GUID}\
//   - legacy device names: Abs rewrites a bare CON to \\.\CON
//
// See learn.microsoft.com/en-us/dotnet/standard/io/file-path-formats.
func isLocalDrivePath(p string) bool {
	if len(p) < 3 {
		return false
	}
	c := p[0]
	if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
		return false
	}
	return p[1] == ':' && (p[2] == '\\' || p[2] == '/')
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
func enumerateImmediateChildren(ctx context.Context, targetDir string) ([]childInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
