//go:build windows

package com_datadoghq_remoteaction_diskusage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/diskusage/du"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// FindQuery is one filename-based finder. Type is "ext" or "glob".
//   - "ext":  Value is comma-separated extensions (e.g. ".dmp,.etl").
//   - "glob": Value is a filepath.Match pattern (e.g. "*.log").
type FindQuery struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Limit int    `json:"limit,omitempty"`
	Label string `json:"label,omitempty"`
}

// AnalyzeInputs must match the TypeScript schema in
// domains/actionplatform/apps/wf-actions-worker/src/runner/bundles/com.datadoghq.remoteaction.diskusage/actions/analyze.ts
type AnalyzeInputs struct {
	Target       string      `json:"target"`
	Mode         string      `json:"mode,omitempty"`
	ExcludePaths []string    `json:"excludePaths,omitempty"`
	Depth        int         `json:"depth,omitempty"`
	MinBytes     int64       `json:"minBytes,omitempty"`
	TopFiles     int         `json:"topFiles,omitempty"`
	TopExt       int         `json:"topExt,omitempty"`
	Find         []FindQuery `json:"find,omitempty"`
}

type Bucket struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"sizeBytes"`
}

type TreeNode struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"sizeBytes"`
	Pruned    bool   `json:"pruned"`
}

type FileEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
}

type ExtensionEntry struct {
	Ext       string `json:"ext"`
	SizeBytes int64  `json:"sizeBytes"`
	FileCount int    `json:"fileCount"`
}

type FindResultBlock struct {
	Query   FindQuery   `json:"query"`
	Matches []FileEntry `json:"matches"`
}

type AnalyzeOutputs struct {
	Target       string            `json:"target"`
	Mode         string            `json:"mode"`
	SubtreeBytes int64             `json:"subtreeBytes"`
	Buckets      []Bucket          `json:"buckets"`
	Tree         []TreeNode        `json:"tree"`
	TopFiles     []FileEntry       `json:"topFiles"`
	TopExt       []ExtensionEntry  `json:"topExt"`
	FindResults  []FindResultBlock `json:"findResults"`
}

type AnalyzeHandler struct{}

func NewAnalyzeHandler() *AnalyzeHandler {
	return &AnalyzeHandler{}
}

const (
	modeAllocated = "allocated"
	modeApparent  = "apparent"
	maxDepth      = 16
	// maxFindLimit caps the per-query Limit. Requests exceeding it return an
	// error rather than silently truncating.
	maxFindLimit = 1000
)

func (h *AnalyzeHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	in, err := types.ExtractInputs[AnalyzeInputs](task)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(in.Target) == "" {
		return nil, errors.New("target is required")
	}

	mode := in.Mode
	if mode == "" {
		mode = modeAllocated
	}
	if mode != modeAllocated && mode != modeApparent {
		return nil, fmt.Errorf("mode must be %q or %q, got %q", modeAllocated, modeApparent, mode)
	}

	depth := in.Depth
	if depth < 0 {
		depth = 0
	}
	if depth > maxDepth {
		depth = maxDepth
	}

	// Translate each FindQuery directly to du.FindQuery. The du package
	// gives each query its own slot + heap + limit, so per-query results
	// are independent and we don't need post-scan attribution.
	finds := make([]du.FindQuery, 0, len(in.Find))
	for i, q := range in.Find {
		if q.Limit < 0 {
			return nil, fmt.Errorf("find[%d]: limit must be >= 0, got %d", i, q.Limit)
		}
		if q.Limit > maxFindLimit {
			return nil, fmt.Errorf("find[%d]: limit %d exceeds maximum of %d", i, q.Limit, maxFindLimit)
		}
		switch q.Type {
		case "ext", "glob":
			// pass through; du.newMatchSet handles empty Value
		default:
			return nil, fmt.Errorf("find[%d]: unsupported type %q (expected \"ext\" or \"glob\")", i, q.Type)
		}
		if q.Value == "" {
			return nil, fmt.Errorf("find[%d]: value must not be empty", i)
		}
		finds = append(finds, du.FindQuery{
			Type:  q.Type,
			Value: q.Value,
			Limit: q.Limit,
			Label: q.Label,
		})
	}

	opts := du.Options{
		ShowApparent:  mode == modeApparent,
		TopFiles:      in.TopFiles,
		TopExtensions: in.TopExt,
		MinFileSize:   in.MinBytes,
		Exclude:       in.ExcludePaths,
		TreeDepth:     depth,
		TreeMinSize:   in.MinBytes,
		Finds:         finds,
	}

	r, err := du.Scan(ctx, in.Target, opts)
	if err != nil {
		return nil, err
	}

	out := &AnalyzeOutputs{
		Target:       r.Target,
		Mode:         mode,
		SubtreeBytes: r.Subtree,
		Buckets:      make([]Bucket, 0, len(r.Buckets)),
		Tree:         []TreeNode{},
		TopFiles:     make([]FileEntry, 0, len(r.TopFiles)),
		TopExt:       make([]ExtensionEntry, 0, len(r.TopExtensions)),
		FindResults:  make([]FindResultBlock, 0, len(r.FindResults)),
	}

	for _, b := range r.Buckets {
		kind := "dir"
		if b.Reparse {
			kind = "reparse"
		}
		out.Buckets = append(out.Buckets, Bucket{
			Name:      b.Name,
			Kind:      kind,
			SizeBytes: b.Size,
		})
	}

	if r.Tree != nil {
		flatten := func() []TreeNode {
			var nodes []TreeNode
			var walk func(n *du.TreeNode, parentPath string)
			walk = func(n *du.TreeNode, parentPath string) {
				var path string
				if n.Depth == 0 {
					path = n.Name
				} else {
					path = filepath.Join(parentPath, n.Name)
				}
				kind := "dir"
				if n.Reparse {
					kind = "reparse"
				}
				nodes = append(nodes, TreeNode{
					Path:      path,
					Kind:      kind,
					SizeBytes: n.Size,
					Pruned:    n.Depth == depth && len(n.Children) == 0,
				})
				for _, c := range n.Children {
					walk(c, path)
				}
			}
			walk(r.Tree, "")
			return nodes
		}
		out.Tree = flatten()
	}

	for _, f := range r.TopFiles {
		out.TopFiles = append(out.TopFiles, FileEntry{Path: f.Path, SizeBytes: f.Size})
	}
	for _, e := range r.TopExtensions {
		out.TopExt = append(out.TopExt, ExtensionEntry{
			Ext:       e.Ext,
			SizeBytes: e.Size,
			FileCount: e.Count,
		})
	}

	// Project du's per-query result blocks back to the action's wire types,
	// preserving the order of the input queries.
	for i, block := range r.FindResults {
		matches := make([]FileEntry, 0, len(block.Matches))
		for _, m := range block.Matches {
			matches = append(matches, FileEntry{Path: m.Path, SizeBytes: m.Size})
		}
		out.FindResults = append(out.FindResults, FindResultBlock{
			Query:   in.Find[i], // use the action-layer FindQuery so JSON tags are preserved in output
			Matches: matches,
		})
	}

	return out, nil
}
