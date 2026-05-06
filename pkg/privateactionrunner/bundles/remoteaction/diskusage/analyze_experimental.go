//go:build private_runner_experimental && windows

package com_datadoghq_remoteaction_diskusage

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
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

func (h *AnalyzeHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if _, err := types.ExtractInputs[AnalyzeInputs](task); err != nil {
		return nil, err
	}
	return nil, errors.New("com.datadoghq.remoteaction.diskusage.analyze: implementation pending")
}
