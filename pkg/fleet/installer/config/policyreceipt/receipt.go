// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package policyreceipt verifies applied files without publishing secret fingerprints.
package policyreceipt

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SecretKey is reserved for metadata delivered alongside encrypted policy secrets.
const SecretKey = "__datadog_config_policy_receipt"

// StoreName is private state, excluded from configuration operations and support archives.
const StoreName = ".fleet-policy-receipts.json"
const maxBytes = 1 << 20

// Receipt contains neither secret values nor fingerprints of those values.
type Receipt struct {
	SchemaVersion  int    `json:"schema_version"`
	OrgID          int64  `json:"org_id"`
	PolicyID       string `json:"policy_id"`
	PolicyVersion  int64  `json:"policy_version"`
	SecretRevision string `json:"secret_revision"`
	DeploymentID   string `json:"deployment_id"`
	VerifiedAt     int64  `json:"verified_at,omitempty"`
}

type entry struct {
	Receipt Receipt `json:"receipt"`
	Path    string  `json:"path"`
	MAC     []byte  `json:"mac"`
}
type store struct {
	Key     []byte  `json:"key"`
	Entries []entry `json:"entries"`
}

var errReceipt = errors.New("configuration policy receipt unavailable")

func (r Receipt) valid() bool {
	return r.SchemaVersion == 1 && r.OrgID > 0 && r.PolicyID != "" && r.PolicyVersion > 0 && r.SecretRevision != "" && r.DeploymentID != ""
}

// FromSecrets binds backend-authenticated identity to the exact config being applied.
func FromSecrets(secrets map[string]string, deploymentID string) (*Receipt, error) {
	payload, found := secrets[SecretKey]
	if !found {
		return nil, nil
	}
	if len(payload) > 4096 {
		return nil, errReceipt
	}
	data, err := base64.RawStdEncoding.DecodeString(payload)
	if err != nil {
		return nil, errReceipt
	}
	var r Receipt
	if json.Unmarshal(data, &r) != nil || !r.valid() || r.DeploymentID != deploymentID || r.VerifiedAt != 0 {
		return nil, errReceipt
	}
	return &r, nil
}

func readLimited(root *os.Root, path string) ([]byte, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, errReceipt
	}
	return data, nil
}

func readStore(root *os.Root) (*store, error) {
	data, err := readLimited(root, StoreName)
	if err != nil {
		return nil, err
	}
	var s store
	if json.Unmarshal(data, &s) != nil || len(s.Key) != 32 {
		return nil, errReceipt
	}
	return &s, nil
}

func digest(key []byte, path string, content []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(path))
	m.Write([]byte{0})
	m.Write(content)
	return m.Sum(nil)
}

// Record runs after all operations and secret substitutions succeed. Its private
// store moves with the configuration directory on Unix.
func Record(rootPath string, receipt *Receipt, paths []string) error {
	if receipt == nil {
		return nil
	}
	if !receipt.valid() {
		return errReceipt
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return errReceipt
	}
	defer root.Close()
	s, err := readStore(root)
	if os.IsNotExist(err) || errors.Is(err, errReceipt) {
		s = &store{Key: make([]byte, 32)}
		if _, err = rand.Read(s.Key); err != nil {
			return errReceipt
		}
	} else if err != nil {
		return errReceipt
	}
	seen := make(map[string]bool)
	for _, path := range paths {
		path = filepath.FromSlash(strings.TrimPrefix(path, "/"))
		if !filepath.IsLocal(path) || path == StoreName {
			return errReceipt
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		content, err := readLimited(root, path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return errReceipt
		}
		entries := s.Entries[:0]
		for _, previous := range s.Entries {
			if previous.Path != path || previous.Receipt.OrgID != receipt.OrgID || previous.Receipt.PolicyID != receipt.PolicyID {
				entries = append(entries, previous)
			}
		}
		s.Entries = append(entries, entry{Receipt: *receipt, Path: path, MAC: digest(s.Key, path, content)})
	}
	data, err := json.Marshal(s)
	if err != nil || len(data) > maxBytes {
		return errReceipt
	}
	nameBytes := make([]byte, 16)
	if _, err := rand.Read(nameBytes); err != nil {
		return errReceipt
	}
	name := StoreName + "." + hex.EncodeToString(nameBytes)
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return errReceipt
	}
	defer root.Remove(name)
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil || closeErr != nil || root.Rename(name, StoreName) != nil {
		return errReceipt
	}
	return nil
}

// Verify rereads the actual file on every report, independently of scrubbed-content
// caches. Missing, changed or unreadable files produce an explicit empty list.
func Verify(rootPath, filename string) []Receipt {
	verified := []Receipt{}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return verified
	}
	defer root.Close()
	s, err := readStore(root)
	if err != nil {
		return verified
	}
	path, err := filepath.Rel(rootPath, filename)
	if err != nil || !filepath.IsLocal(path) {
		return verified
	}
	content, err := readLimited(root, path)
	if err != nil {
		return verified
	}
	mac := digest(s.Key, path, content)
	for _, entry := range s.Entries {
		if entry.Path == path && entry.Receipt.valid() && hmac.Equal(entry.MAC, mac) {
			r := entry.Receipt
			r.VerifiedAt = time.Now().UnixMilli()
			verified = append(verified, r)
		}
	}
	return verified
}

// Invalidate discards evidence after a Windows rollback, whose backup copies YAML only.
func Invalidate(rootPath string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return errReceipt
	}
	defer root.Close()
	err = root.Remove(StoreName)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// KnownFiles lets inventory clear stale evidence even after a file disappears
// from the configuration provider's current file list. It never attests absence.
func KnownFiles(rootPath string) []string {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil
	}
	defer root.Close()
	state, err := readStore(root)
	if err != nil {
		return nil
	}
	paths := []string{}
	seen := map[string]bool{}
	for _, entry := range state.Entries {
		if !entry.Receipt.valid() || !filepath.IsLocal(entry.Path) || entry.Path == StoreName || seen[entry.Path] {
			continue
		}
		seen[entry.Path] = true
		paths = append(paths, filepath.Join(rootPath, entry.Path))
	}
	return paths
}
