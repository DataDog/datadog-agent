// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// ErrNoProvisionedStack is returned by LoadProvisionedStack when no persisted
// state exists for the requested stack. Use errors.Is to test for it.
var ErrNoProvisionedStack = errors.New("no provisioned stack record found")

// ProvisionedStack holds everything that was recorded when a stack was created.
type ProvisionedStack struct {
	Scenario  string                     `json:"scenario"`
	Stack     string                     `json:"stack"`
	Config    map[string]string          `json:"config"`
	Resources map[string]json.RawMessage `json:"resources"` // provisioned outputs
	Keys      map[string]string          `json:"keys"`      // field-name → import key, captured at create time
	CreatedAt time.Time                  `json:"created_at"`
}

// stateDir returns the directory used to persist stack state files.
// It honours $SCENARIORUN_STATE_DIR; falls back to ~/.scenariorun/stacks,
// and to "." if the home directory cannot be determined.
func stateDir() string {
	if d := os.Getenv("SCENARIORUN_STATE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".scenariorun", "stacks")
}

// sanitizeStackName converts a stack name to a safe filename by replacing
// path-separating and whitespace characters with underscores.
func sanitizeStackName(stack string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		string(os.PathSeparator), "_",
		" ", "_",
	)
	return replacer.Replace(stack)
}

func stackFilePath(stack string) string {
	return filepath.Join(stateDir(), sanitizeStackName(stack)+".json")
}

// SaveProvisionedStack persists ps to disk under stateDir. The directory is
// created with 0700 if it does not already exist, and the file is written with
// 0600 permissions.
func SaveProvisionedStack(ps ProvisionedStack) error {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create state dir %s: %w", dir, err)
	}
	data, err := json.Marshal(ps)
	if err != nil {
		return fmt.Errorf("marshal provisioned stack: %w", err)
	}
	path := stackFilePath(ps.Stack)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write state file %s: %w", path, err)
	}
	return nil
}

// LoadProvisionedStack reads and unmarshals the persisted state for stack.
// Returns a wrapped ErrNoProvisionedStack when no record exists (use errors.Is).
func LoadProvisionedStack(stack string) (ProvisionedStack, error) {
	path := stackFilePath(stack)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProvisionedStack{}, fmt.Errorf("stack %q: %w", stack, ErrNoProvisionedStack)
		}
		return ProvisionedStack{}, fmt.Errorf("read state file %s: %w", path, err)
	}
	var ps ProvisionedStack
	if err := json.Unmarshal(data, &ps); err != nil {
		return ProvisionedStack{}, fmt.Errorf("unmarshal state file %s: %w", path, err)
	}
	return ps, nil
}

// ListProvisionedStacks returns all persisted stacks sorted by Stack name.
// Returns an empty slice (not an error) when the state directory does not exist.
// Unreadable or unparseable files are skipped with a warning to stderr.
func ListProvisionedStacks() ([]ProvisionedStack, error) {
	dir := stateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state dir %s: %w", dir, err)
	}

	var stacks []ProvisionedStack
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scenariorun: skipping unreadable state file %s: %v\n", path, err)
			continue
		}
		var ps ProvisionedStack
		if err := json.Unmarshal(data, &ps); err != nil {
			fmt.Fprintf(os.Stderr, "scenariorun: skipping unparseable state file %s: %v\n", path, err)
			continue
		}
		stacks = append(stacks, ps)
	}

	sort.Slice(stacks, func(i, j int) bool { return stacks[i].Stack < stacks[j].Stack })
	return stacks, nil
}

// DeleteProvisionedStack removes the persisted state for stack. A missing file
// is not treated as an error.
func DeleteProvisionedStack(stack string) error {
	path := stackFilePath(stack)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete state file %s: %w", path, err)
	}
	return nil
}

// toRawMessage converts provisioners.RawResources (map[string][]byte) to the
// JSON-serialisable map[string]json.RawMessage used in ProvisionedStack.
// json.RawMessage is []byte, so this is a shallow copy.
func toRawMessage(res provisioners.RawResources) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(res))
	for k, v := range res {
		out[k] = json.RawMessage(v)
	}
	return out
}

// fromRawMessage is the inverse of toRawMessage.
func fromRawMessage(m map[string]json.RawMessage) provisioners.RawResources {
	out := make(provisioners.RawResources, len(m))
	for k, v := range m {
		out[k] = []byte(v)
	}
	return out
}
