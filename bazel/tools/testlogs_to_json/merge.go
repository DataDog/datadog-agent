// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type bepContext struct {
	localExecRoot  string
	configTestlogs map[string]string
	configBindirs  map[string]string
}

type bepTestAction struct {
	label  string
	cfgID  string
	logURI string
}

func mergeBEP(bepPath, outputPath string) error {
	actions, ctx, err := parseBEPTestLogs(bepPath)
	if err != nil {
		return err
	}
	var fragments []string
	for _, action := range actions {
		logPath, err := resolveBEPOutputPath(action.label, action.logURI, action.cfgID, &ctx, "test.log")
		if err != nil {
			return err
		}
		fragment, err := fragmentPathForLog(action.label, logPath)
		if err != nil {
			return err
		}
		fragments = append(fragments, fragment)
	}
	return concatFragments(fragments, outputPath)
}

func concatFragments(fragments []string, outputPath string) error {
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output %q: %w", outputPath, err)
	}
	defer out.Close()

	for _, fragment := range fragments {
		data, err := os.ReadFile(fragment)
		if err != nil {
			return fmt.Errorf("read fragment %q: %w", fragment, err)
		}
		if len(data) == 0 {
			continue
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("write fragment %q: %w", fragment, err)
		}
		if data[len(data)-1] != '\n' {
			if _, err := out.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseBEPTestLogs(bepPath string) ([]bepTestAction, bepContext, error) {
	f, err := os.Open(bepPath)
	if err != nil {
		return nil, bepContext{}, fmt.Errorf("open bep %q: %w", bepPath, err)
	}
	defer f.Close()

	ctx := bepContext{configTestlogs: map[string]string{}, configBindirs: map[string]string{}}
	var actions []bepTestAction

	dec := json.NewDecoder(f)
	for {
		var event map[string]any
		if err := dec.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return nil, ctx, fmt.Errorf("decode bep %q: %w", bepPath, err)
		}
		if observeBEPEvent(&ctx, event) {
			continue
		}
		id, _ := event["id"].(map[string]any)
		testResult, _ := id["testResult"].(map[string]any)
		label, _ := testResult["label"].(string)
		if label == "" {
			continue
		}
		tr, _ := event["testResult"].(map[string]any)
		logURI, err := testActionOutputURI(tr, "test.log")
		if err != nil {
			return nil, ctx, fmt.Errorf("bep testResult for %s: %w", label, err)
		}
		cfg, _ := testResult["configuration"].(map[string]any)
		cfgID, _ := cfg["id"].(string)
		actions = append(actions, bepTestAction{label: label, cfgID: cfgID, logURI: logURI})
	}
	return actions, ctx, nil
}

func observeBEPEvent(ctx *bepContext, event map[string]any) bool {
	id, _ := event["id"].(map[string]any)
	if _, ok := id["workspace"]; ok {
		wi, _ := event["workspaceInfo"].(map[string]any)
		ctx.localExecRoot, _ = wi["localExecRoot"].(string)
		return true
	}
	cfg, _ := id["configuration"].(map[string]any)
	cfgID, _ := cfg["id"].(string)
	if cfgID == "" {
		return false
	}
	conf, _ := event["configuration"].(map[string]any)
	mv, _ := conf["makeVariable"].(map[string]any)
	bindir, _ := mv["BINDIR"].(string)
	if bindir == "" {
		return false
	}
	ctx.configBindirs[cfgID] = bindir
	bindirPath := filepath.Clean(bindir)
	if filepath.Base(bindirPath) == "bin" {
		ctx.configTestlogs[cfgID] = filepath.Join(filepath.Dir(bindirPath), "testlogs")
	}
	return true
}

func testActionOutputURI(testResult map[string]any, name string) (string, error) {
	outputs, _ := testResult["testActionOutput"].([]any)
	for _, raw := range outputs {
		output, _ := raw.(map[string]any)
		if output["name"] == name {
			uri, _ := output["uri"].(string)
			if uri == "" {
				return "", fmt.Errorf("missing uri for %s", name)
			}
			return uri, nil
		}
	}
	return "", fmt.Errorf("missing %s in testActionOutput", name)
}

func resolveBEPOutputPath(label, uri, cfgID string, ctx *bepContext, outputName string) (string, error) {
	if strings.HasPrefix(uri, "file://") {
		path := strings.TrimPrefix(uri, "file://")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	testlogsDir := ctx.configTestlogs[cfgID]
	if ctx.localExecRoot != "" && testlogsDir != "" {
		labelRel := strings.TrimPrefix(label, "//")
		labelRel = strings.ReplaceAll(labelRel, ":", "/")
		candidate := filepath.Join(ctx.localExecRoot, testlogsDir, labelRel, outputName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not resolve %s for %s (uri=%q)", outputName, label, uri)
}

func fragmentPathForLog(label, logPath string) (string, error) {
	name := label
	if idx := strings.LastIndex(label, ":"); idx >= 0 {
		name = label[idx+1:]
	}
	pkg := strings.TrimPrefix(label, "//")
	if idx := strings.Index(pkg, ":"); idx >= 0 {
		pkg = pkg[:idx]
	}
	fname := fragmentBasename(name, logPath)

	// Prefer sibling bin/ next to the test.log config root.
	if strings.Contains(logPath, "/testlogs/") {
		prefix := logPath[:strings.Index(logPath, "/testlogs/")]
		candidate := filepath.Join(prefix, "bin", filepath.FromSlash(pkg), fname)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	matches, _ := filepath.Glob(filepath.Join("bazel-out", "*", "bin", filepath.FromSlash(pkg), fname))
	if len(matches) == 1 {
		return matches[0], nil
	}
	execRoot := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if execRoot != "" {
		matches, _ = filepath.Glob(filepath.Join(execRoot, "bazel-out", "*", "bin", filepath.FromSlash(pkg), fname))
		if len(matches) == 1 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("expected one test2json fragment for %s log %s (tried %s)", label, logPath, fname)
}

func fragmentBasename(targetName, logPath string) string {
	marker := targetName + "/"
	if idx := strings.Index(logPath, marker); idx >= 0 {
		suffix := logPath[idx+len(marker):]
		suffix = strings.ReplaceAll(suffix, "/", "_")
		suffix = strings.ReplaceAll(suffix, ":", "_")
		return targetName + "_" + suffix + ".test2json.jsonl"
	}
	base := filepath.Base(logPath)
	return targetName + "_" + base + ".test2json.jsonl"
}
