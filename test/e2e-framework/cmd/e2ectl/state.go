// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// envState is the on-disk state file's shape: top-level keys are
// RawResources keys (e.g. "kubernetesCluster", "agent"), exactly what
// SingleFileProvisioner[Env] and kind_nopulumi_test.go already read via
// E2E_ENV_FILE. e2ectl only adds a writer/inspector on top; the format
// itself is unchanged from what cmd/envctl wrote.
type envState map[string]json.RawMessage

// sourceKey is state.go's own reserved metadata entry recording the
// absolute path to the YAML TestDefinition that produced this state file,
// so the dashboard (dashboard.go, Task 5) can map a discovered state file
// back to its config without a separate registry.
const sourceKey = "_source"

// readStateFile reads the state file at path, returning an empty state if
// it doesn't exist yet (a fresh run before any stage has completed).
func readStateFile(path string) (envState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return envState{}, nil
	}
	if err != nil {
		return nil, err
	}

	var st envState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return st, nil
}

// writeStateFileAtomic writes st to path via a temp file + rename, so a
// crash mid-write never leaves a corrupted/partial state file behind.
func writeStateFileAtomic(path string, st envState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// stagesCompleted reports which of the two out-of-band stages (infra
// provisioning, agent install) the state file already reflects.
// "Provisioned" is any top-level entry other than "agent" and reserved
// metadata keys (recognized by a "_" prefix) — this works for any
// provisioner's resource key (e.g. "kubernetesCluster" for kind, or
// whatever a future non-Kubernetes provisioner writes) without e2ectl
// needing a lookup table from provisioner type to resource key name.
func stagesCompleted(st envState) (provisioned, installed bool) {
	for k := range st {
		if k == "agent" || strings.HasPrefix(k, "_") {
			continue
		}
		provisioned = true
	}
	_, installed = st["agent"]
	return provisioned, installed
}

// sourcePath returns st's recorded "_source" entry, if present.
func sourcePath(st envState) (string, bool) {
	raw, ok := st[sourceKey]
	if !ok {
		return "", false
	}
	var p string
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", false
	}
	return p, true
}

// setSourcePath records path as st's "_source" entry.
func setSourcePath(st envState, path string) error {
	raw, err := json.Marshal(path)
	if err != nil {
		return err
	}
	st[sourceKey] = raw
	return nil
}
