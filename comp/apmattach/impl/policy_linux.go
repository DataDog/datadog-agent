// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func policyFiles(confDir string) []string {
	return []string{
		filepath.Join(confDir, "user-wls-policy.bin"),
		filepath.Join(confDir, "managed", "rc-orgwide-wls-policy.bin"),
		filepath.Join(confDir, "managed", "rc-targeted-wls-policy.bin"),
	}
}

// policyWatcher detects policy file changes by polling content hashes.
type policyWatcher struct {
	paths []string
	last  string
}

func newPolicyWatcher(paths []string) *policyWatcher {
	w := &policyWatcher{paths: paths}
	w.last = w.fingerprint()
	return w
}

func (w *policyWatcher) changed() bool {
	fp := w.fingerprint()
	if fp == w.last {
		return false
	}
	w.last = fp
	return true
}

func (w *policyWatcher) fingerprint() string {
	parts := make([]string, 0, len(w.paths))
	for _, path := range w.paths {
		parts = append(parts, fileFingerprint(path))
	}
	return strings.Join(parts, "|")
}

func fileFingerprint(path string) string {
	data, err := os.ReadFile(path)
	if err == nil {
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	if os.IsNotExist(err) {
		return "absent"
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return "error"
	}
	var ino uint64
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		ino = st.Ino
	}
	return fmt.Sprintf("stat:%d:%d:%d", ino, info.Size(), info.ModTime().UnixNano())
}
