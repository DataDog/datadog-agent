// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsdu

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/ntfsmft"
)

const (
	modeAllocated = "allocated"
	modeApparent  = "apparent"
)

// jsonTreeNode is one node in the depth-limited directory tree. The tree is
// emitted as a flat, pre-order list; Path is the full path to the node and
// Pruned marks a leaf at the requested depth that has undisplayed descendants.
type jsonTreeNode struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	SizeBytes   int64  `json:"sizeBytes"`
	Pruned      bool   `json:"pruned"`
	FileCount   int    `json:"fileCount"`
	FolderCount int    `json:"folderCount"`
}

// jsonFileEntry is one file in topFiles / find matches. Created/Modified are
// RFC 3339 UTC timestamps, omitted when unavailable (file not openable at
// resolution time).
type jsonFileEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Created   string `json:"created,omitempty"`
	Modified  string `json:"modified,omitempty"`
}

// jsonExtEntry is one aggregated file extension.
type jsonExtEntry struct {
	Ext       string `json:"ext"`
	SizeBytes int64  `json:"sizeBytes"`
	FileCount int    `json:"fileCount"`
}

// jsonFindQuery echoes the find predicate back in the output.
type jsonFindQuery struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Limit int    `json:"limit,omitempty"`
	Label string `json:"label,omitempty"`
}

// jsonFindBlock pairs a find query with its matches.
type jsonFindBlock struct {
	Query   jsonFindQuery   `json:"query"`
	Matches []jsonFileEntry `json:"matches"`
}

// jsonOutput is the top-level JSON document ntfs-du emits. The folder breakdown
// is the depth-limited Tree, whose depth-1 nodes are the target's immediate
// children; Tree is omitted entirely at --max-depth 0.
type jsonOutput struct {
	Target       string          `json:"target"`
	Mode         string          `json:"mode"`
	SubtreeBytes int64           `json:"subtreeBytes"`
	Tree         []jsonTreeNode  `json:"tree,omitempty"`
	TopFiles     []jsonFileEntry `json:"topFiles"`
	TopExt       []jsonExtEntry  `json:"topExt"`
	FindResults  []jsonFindBlock `json:"findResults"`
	// Emitted only when part of the MFT was missed, so JSON consumers can detect
	// that the totals undercount. ReadErrors/RecordsSkipped cover chunks that
	// failed to read; UnmappedRecords covers records whose location in the $MFT
	// was never determined, with UnreachableExtensions as the cause.
	ReadErrors            int `json:"readErrors,omitempty"`
	RecordsSkipped        int `json:"recordsSkipped,omitempty"`
	UnmappedRecords       int `json:"unmappedRecords,omitempty"`
	UnreachableExtensions int `json:"unreachableExtensions,omitempty"`
}

// run performs the scan on Windows and writes the JSON report to stdout.
func run(ctx context.Context, callCtx *builtins.CallContext, opts options) builtins.Result {
	target := opts.target
	if target == "" {
		target = driveRoot(callCtx.WorkDir())
	} else if !filepath.IsAbs(target) {
		// Resolve a relative operand against the shell's working directory, not
		// the host process cwd. Scan() requires an absolute path and will not
		// anchor one itself.
		target = filepath.Join(callCtx.WorkDir(), target)
	}

	finds := buildFinds(opts)

	// Resolve relative --exclude operands against the shell's working directory
	// for the same reason as target above: ntfsmft rejects a relative exclude,
	// and anchoring to anything but the shell's cwd would exclude the wrong
	// subtree after a `cd`.
	exclude := make([]string, 0, len(opts.exclude))
	for _, p := range opts.exclude {
		if p == "" {
			continue // an empty --exclude names nothing; skip rather than fail resolution
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(callCtx.WorkDir(), p)
		}
		exclude = append(exclude, p)
	}

	mode := modeAllocated
	if opts.apparent {
		mode = modeApparent
	}
	// A zero tree limit omits the tree just like --max-depth 0. Tell the scanner
	// that before it resolves immediate children, rather than building metadata
	// that the JSON renderer will discard.
	treeDepth := effectiveTreeDepth(opts.maxDepth, opts.treeLimit)

	res, err := ntfsmft.Scan(ctx, target, ntfsmft.Options{
		ShowApparent:  opts.apparent,
		TopFiles:      opts.topFiles,
		TopExtensions: opts.topExt,
		MinFileSize:   opts.minSize,
		Finds:         finds,
		Exclude:       exclude,
		TreeDepth:     treeDepth,
		TreeMinSize:   opts.minSize,
	})
	if err != nil {
		callCtx.Errf("ntfs-du: %s\n", err)
		return builtins.Result{Code: 1}
	}

	out := buildOutput(res, mode, opts.maxDepth, opts.treeLimit)
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		callCtx.Errf("ntfs-du: encoding output: %s\n", err)
		return builtins.Result{Code: 1}
	}
	callCtx.Out(string(enc))
	callCtx.Out("\n")

	// A partial scan still emits the results it gathered, but the totals
	// undercount — warn on stderr so the caller knows the report is incomplete.
	// We deliberately do NOT fail the command: a real, healthy volume routinely
	// has some unreadable MFT chunks (transiently locked regions, records caught
	// mid-write, sectors the I/O path declines), so exiting non-zero would make
	// ntfs-du appear to fail on nearly every genuine scan. The undercount is also
	// surfaced structurally in the JSON for programmatic consumers.
	if res.ReadErrors > 0 {
		callCtx.Errf("ntfs-du: %d MFT chunk(s) unreadable (~%d records skipped); reported sizes undercount\n",
			res.ReadErrors, res.SkippedRecords)
	}
	// A separate cause: part of the $MFT had no known location, so those records
	// were never read. Reported only when records were actually lost — an
	// unreachable extension record that described an already-covered range costs
	// nothing and is not worth a warning.
	if res.UnmappedMFTRecords > 0 {
		callCtx.Errf("ntfs-du: %d $MFT extension record(s) unreachable, ~%d records not scanned; reported sizes undercount\n",
			res.UnreachableMFTExtensions, res.UnmappedMFTRecords)
	}
	return builtins.Result{}
}

// effectiveTreeDepth tells the scanner whether it needs any tree metadata.
// A zero output limit omits the tree entirely, so preserving a positive depth
// would only trigger unused immediate-child enumeration and tree assembly.
func effectiveTreeDepth(maxDepth, treeLimit int) int {
	if treeLimit == 0 {
		return 0
	}
	return maxDepth
}

// driveRoot returns the volume root ("C:\") for a drive-letter path. Non
// drive-shaped inputs are returned unchanged so Scan surfaces a clear error.
func driveRoot(wd string) string {
	if len(wd) >= 2 && wd[1] == ':' {
		return wd[:2] + `\`
	}
	return wd
}

// buildFinds translates the per-type find flags into ntfsmft.FindQuery values,
// preserving the order ext, glob, regex.
func buildFinds(opts options) []ntfsmft.FindQuery {
	total := len(opts.findExt) + len(opts.findGlob) + len(opts.findRegex)
	if total == 0 {
		return nil
	}
	finds := make([]ntfsmft.FindQuery, 0, total)
	for _, v := range opts.findExt {
		finds = append(finds, ntfsmft.FindQuery{Type: "ext", Value: v, Limit: opts.findLimit})
	}
	for _, v := range opts.findGlob {
		finds = append(finds, ntfsmft.FindQuery{Type: "glob", Value: v, Limit: opts.findLimit})
	}
	for _, v := range opts.findRegex {
		finds = append(finds, ntfsmft.FindQuery{Type: "regex", Value: v, Limit: opts.findLimit})
	}
	return finds
}

// buildOutput maps an ntfsmft.Result into the JSON document.
// depth is the requested tree depth, used to mark pruned leaves.
func buildOutput(res *ntfsmft.Result, mode string, depth, treeLimit int) jsonOutput {
	out := jsonOutput{
		Target:                res.Target,
		Mode:                  mode,
		SubtreeBytes:          res.Subtree,
		TopFiles:              make([]jsonFileEntry, 0, len(res.TopFiles)),
		TopExt:                make([]jsonExtEntry, 0, len(res.TopExtensions)),
		FindResults:           make([]jsonFindBlock, 0, len(res.FindResults)),
		ReadErrors:            res.ReadErrors,
		RecordsSkipped:        res.SkippedRecords,
		UnmappedRecords:       res.UnmappedMFTRecords,
		UnreachableExtensions: res.UnreachableMFTExtensions,
	}

	// The engine only builds a tree at TreeDepth > 0; at --max-depth 0 it is
	// nil, so Tree stays empty and (via omitempty) is dropped from the output.
	if res.Tree != nil {
		out.Tree = flattenTree(res.Tree, depth, treeLimit)
	}

	for _, f := range res.TopFiles {
		out.TopFiles = append(out.TopFiles, fileEntry(f))
	}
	for _, e := range res.TopExtensions {
		out.TopExt = append(out.TopExt, jsonExtEntry{Ext: e.Ext, SizeBytes: e.Size, FileCount: e.Count})
	}
	for _, blk := range res.FindResults {
		matches := make([]jsonFileEntry, 0, len(blk.Matches))
		for _, m := range blk.Matches {
			matches = append(matches, fileEntry(m))
		}
		out.FindResults = append(out.FindResults, jsonFindBlock{
			Query: jsonFindQuery{
				Type:  blk.Query.Type,
				Value: blk.Query.Value,
				Limit: blk.Query.Limit,
				Label: blk.Query.Label,
			},
			Matches: matches,
		})
	}

	return out
}

// flattenTree walks an ntfsmft.TreeNode subtree in pre-order and returns a flat
// list of nodes. The root node carries its full path; descendants carry a path
// joined from the parent's path and the node basename. A node at the requested
// depth with no displayed children is marked Pruned.
func flattenTree(root *ntfsmft.TreeNode, depth, limit int) []jsonTreeNode {
	if limit == 0 {
		return nil
	}
	var nodes []jsonTreeNode
	var walk func(n *ntfsmft.TreeNode, parentPath string)
	walk = func(n *ntfsmft.TreeNode, parentPath string) {
		if len(nodes) == limit {
			return
		}
		path := n.Name
		if n.Depth != 0 {
			path = filepath.Join(parentPath, n.Name)
		}
		nodes = append(nodes, jsonTreeNode{
			Path:        path,
			Kind:        dirKind(n.Reparse),
			SizeBytes:   n.Size,
			Pruned:      n.Depth == depth && len(n.Children) == 0 && n.Dirs > 0,
			FileCount:   n.Files,
			FolderCount: n.Dirs,
		})
		for _, c := range n.Children {
			if len(nodes) == limit {
				return
			}
			walk(c, path)
		}
	}
	walk(root, "")
	return nodes
}

// fileEntry maps an engine FileEntry to its JSON form, formatting the
// creation / modification times as RFC 3339 UTC (omitted when unavailable).
func fileEntry(f ntfsmft.FileEntry) jsonFileEntry {
	return jsonFileEntry{
		Path:      f.Path,
		SizeBytes: f.Size,
		Created:   rfc3339(f.Created),
		Modified:  rfc3339(f.Modified),
	}
}

// rfc3339 formats t as RFC 3339 UTC, or "" if t is the zero value.
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func dirKind(reparse bool) string {
	if reparse {
		return "reparse"
	}
	return "dir"
}
