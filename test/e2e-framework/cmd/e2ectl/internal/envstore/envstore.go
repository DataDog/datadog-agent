// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package envstore is e2ectl's named-environment store. Each environment lives
// in its own directory under $E2ECTL_HOME/envs (default ~/.e2ectl) and holds:
//
//	meta.json     identity, status, driver bookkeeping
//	snapshot.json the environment snapshot (the currency between commands)
//	config.yaml   a copy of the config file used at creation time
//	kubeconfig    the cluster kubeconfig, when the environment is a cluster
//
// The store is deliberately plain files: `e2ectl list` must be instant.
package envstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
)

// Status of an environment.
const (
	StatusProvisioning = "provisioning"
	StatusReady        = "ready"
	StatusError        = "error"
)

// Meta is the per-environment metadata.
type Meta struct {
	Name           string    `json:"name"`
	Base           string    `json:"base"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	FakeIntakeURL  string    `json:"fakeintake_url,omitempty"`
	FakeIntakePort int       `json:"fakeintake_port,omitempty"`
	AgentImage     string    `json:"agent_image,omitempty"`
	AgentVersion   string    `json:"agent_version,omitempty"`
	AgentInstalled bool      `json:"agent_installed"`
	// KindName is the kind cluster name (base kind only).
	KindName string `json:"kind_name,omitempty"`
	// StackName is the Pulumi stack name (base ec2-host only).
	StackName string `json:"stack_name,omitempty"`
}

// Entry is a stored environment.
type Entry struct {
	Name string
	Meta Meta
	Dir  string
}

// SnapshotPath returns the path of the environment snapshot.
func (e Entry) SnapshotPath() string { return filepath.Join(e.Dir, "snapshot.json") }

// KubeconfigPath returns the path of the kubeconfig, when the environment is a cluster.
func (e Entry) KubeconfigPath() string { return filepath.Join(e.Dir, "kubeconfig") }

// ConfigPath returns the path of the stored config copy.
func (e Entry) ConfigPath() string { return filepath.Join(e.Dir, "config.yaml") }

// LoadConfig parses the stored config copy.
func (e Entry) LoadConfig() (*config.File, error) {
	return config.Load(e.ConfigPath())
}

// Store is the root of the environment store.
type Store struct {
	root string
}

// New returns the store rooted at $E2ECTL_HOME (default ~/.e2ectl).
func New() (*Store, error) {
	root := os.Getenv("E2ECTL_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".e2ectl")
	}
	return &Store{root: root}, nil
}

// Root returns the store root directory.
func (s *Store) Root() string { return s.root }

// EnvsDir returns the directory holding the environment entries.
func (s *Store) EnvsDir() string { return filepath.Join(s.root, "envs") }

// Create creates the directory for a new environment and stores its metadata
// and config copy. Fails if the name already exists.
func (s *Store) Create(name string, cfg *config.File, meta Meta) (Entry, error) {
	if name == "" {
		return Entry{}, errors.New("environment name is required")
	}
	entry := Entry{
		Name: name,
		Dir:  filepath.Join(s.EnvsDir(), name),
	}
	if err := os.MkdirAll(entry.Dir, 0o755); err != nil {
		return entry, err
	}
	// refuse to silently reuse an existing environment
	if _, err := os.Stat(filepath.Join(entry.Dir, "meta.json")); err == nil {
		return entry, fmt.Errorf("environment %q already exists (use e2ectl stop first, or another name)", name)
	}

	meta.Name = name
	meta.Base = cfg.Environment.Base
	meta.CreatedAt = time.Now().UTC()
	if meta.Status == "" {
		meta.Status = StatusProvisioning
	}
	if err := writeJSON(filepath.Join(entry.Dir, "meta.json"), meta); err != nil {
		return entry, err
	}

	// store a copy of the config used at creation time: it is the source of
	// truth for teardown and later install/update runs
	cfgData, err := os.ReadFile(cfg.Path)
	if err != nil {
		return entry, fmt.Errorf("storing config copy: %w", err)
	}
	if err := os.WriteFile(entry.ConfigPath(), cfgData, 0o644); err != nil {
		return entry, err
	}

	entry.Meta = meta
	return entry, nil
}

// Get loads an existing environment.
func (s *Store) Get(name string) (Entry, error) {
	entry := Entry{
		Name: name,
		Dir:  filepath.Join(s.EnvsDir(), name),
	}
	data, err := os.ReadFile(filepath.Join(entry.Dir, "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return entry, fmt.Errorf("no environment named %q (see e2ectl list)", name)
		}
		return entry, err
	}
	if err := json.Unmarshal(data, &entry.Meta); err != nil {
		return entry, fmt.Errorf("reading metadata of %q: %w", name, err)
	}
	return entry, nil
}

// List returns every stored environment, sorted by name.
func (s *Store) List() ([]Entry, error) {
	entries, err := os.ReadDir(s.EnvsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		e, err := s.Get(d.Name())
		if err != nil {
			continue // half-created entries are skipped, not fatal
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// UpdateMeta rewrites the metadata of an entry.
func (s *Store) UpdateMeta(e Entry) error {
	return writeJSON(filepath.Join(e.Dir, "meta.json"), e.Meta)
}

// Delete removes an entry directory.
func (s *Store) Delete(name string) error {
	return os.RemoveAll(filepath.Join(s.EnvsDir(), name))
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
