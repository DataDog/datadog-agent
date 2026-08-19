// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// test2jsonOutputGroup is the output group the test_log_to_json aspect publishes.
const test2jsonOutputGroup = "test2json"

// bepEvent is the subset of the Build Event Protocol needed to locate the
// fragments produced by the test_log_to_json aspect.
type bepEvent struct {
	ID struct {
		NamedSet        *bepSetID    `json:"namedSet"`
		TargetCompleted *bepTargetID `json:"targetCompleted"`
	} `json:"id"`
	NamedSetOfFiles *bepNamedSet  `json:"namedSetOfFiles"`
	Completed       *bepCompleted `json:"completed"`
	WorkspaceInfo   *bepWorkspace `json:"workspaceInfo"`
}

type bepSetID struct {
	ID string `json:"id"`
}

type bepTargetID struct {
	Label string `json:"label"`
}

type bepNamedSet struct {
	Files    []bepFile  `json:"files"`
	FileSets []bepSetID `json:"fileSets"`
}

type bepFile struct {
	Name       string   `json:"name"`
	URI        string   `json:"uri"`
	PathPrefix []string `json:"pathPrefix"`
}

type bepCompleted struct {
	OutputGroup []struct {
		Name     string     `json:"name"`
		FileSets []bepSetID `json:"fileSets"`
	} `json:"outputGroup"`
}

type bepWorkspace struct {
	LocalExecRoot string `json:"localExecRoot"`
}

// mergeBEP concatenates the test2json fragments a Bazel invocation reported in
// its BEP file.
func mergeBEP(bepPath, outputPath string) error {
	fragments, err := fragmentsFromBEP(bepPath)
	if err != nil {
		return err
	}
	return concatFragments(fragments, outputPath)
}

// fragmentsFromBEP returns fragment paths ordered by test label, so that a
// rerun of the same tests produces byte-identical output.
func fragmentsFromBEP(bepPath string) ([]string, error) {
	f, err := os.Open(bepPath)
	if err != nil {
		return nil, fmt.Errorf("open bep %q: %w", bepPath, err)
	}
	defer f.Close()

	type outputGroup struct {
		label   string
		fileSet []bepSetID
	}

	sets := map[string]*bepNamedSet{}
	execRoot := ""
	var groups []outputGroup

	dec := json.NewDecoder(f)
	for {
		var event bepEvent
		if err := dec.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode bep %q: %w", bepPath, err)
		}
		switch {
		case event.ID.NamedSet != nil && event.NamedSetOfFiles != nil:
			sets[event.ID.NamedSet.ID] = event.NamedSetOfFiles
		case event.WorkspaceInfo != nil:
			execRoot = event.WorkspaceInfo.LocalExecRoot
		case event.ID.TargetCompleted != nil && event.Completed != nil:
			for _, group := range event.Completed.OutputGroup {
				if group.Name == test2jsonOutputGroup {
					groups = append(groups, outputGroup{
						label:   event.ID.TargetCompleted.Label,
						fileSet: group.FileSets,
					})
				}
			}
		}
	}

	sort.SliceStable(groups, func(i, j int) bool { return groups[i].label < groups[j].label })

	var fragments []string
	for _, group := range groups {
		files, err := flattenFileSets(sets, group.fileSet)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", group.label, err)
		}
		for _, file := range files {
			path, err := file.localPath(execRoot)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", group.label, err)
			}
			fragments = append(fragments, path)
		}
	}
	return fragments, nil
}

// flattenFileSets walks the depset an output group refers to. Bazel posts every
// named set before the event referencing it, but sets may nest arbitrarily.
func flattenFileSets(sets map[string]*bepNamedSet, roots []bepSetID) ([]bepFile, error) {
	var files []bepFile
	visited := map[string]bool{}
	queue := append([]bepSetID(nil), roots...)
	for len(queue) > 0 {
		id := queue[0].ID
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true

		set, ok := sets[id]
		if !ok {
			return nil, fmt.Errorf("bep references unknown fileSet %q", id)
		}
		files = append(files, set.Files...)
		queue = append(queue, set.FileSets...)
	}
	return files, nil
}

// localPath resolves a BEP file entry to an on-disk path. Outputs kept in the
// remote cache are reported as bytestream:// rather than file://, so fall back
// to the exec-root-relative path Bazel reports alongside the uri.
func (f bepFile) localPath(execRoot string) (string, error) {
	if strings.HasPrefix(f.URI, "file://") {
		parsed, err := url.Parse(f.URI)
		if err != nil {
			return "", fmt.Errorf("parse uri %q: %w", f.URI, err)
		}
		return filepath.FromSlash(parsed.Path), nil
	}
	if execRoot == "" {
		return "", fmt.Errorf("cannot resolve %q: bep has no workspace event", f.Name)
	}
	parts := append([]string{execRoot}, f.PathPrefix...)
	return filepath.Join(append(parts, f.Name)...), nil
}

// concatFragments copies fragments verbatim: bzltestutil terminates every event
// with a newline, so the concatenation is already valid JSONL.
func concatFragments(fragments []string, outputPath string) error {
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output %q: %w", outputPath, err)
	}
	defer out.Close()

	for _, fragment := range fragments {
		in, err := os.Open(fragment)
		if err != nil {
			return fmt.Errorf("open fragment %q: %w", fragment, err)
		}
		_, copyErr := io.Copy(out, in)
		closeErr := in.Close()
		if copyErr != nil {
			return fmt.Errorf("copy fragment %q: %w", fragment, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close fragment %q: %w", fragment, closeErr)
		}
	}
	return nil
}
