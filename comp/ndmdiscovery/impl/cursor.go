// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
)

const cursorKeyPrefix = "ndmdiscovery"

// cursorState is the resumable progress of one autodiscovery cycle. A /16
// cycle takes hours, so an agent restart must continue where it stopped
// instead of starting the cycle again.
type cursorState struct {
	RunID        string `json:"run_id"`
	NextChunk    int    `json:"next_chunk"`
	Scanned      int64  `json:"scanned"`
	StartedAtMs  int64  `json:"started_at_ms"`
	ConfigDigest string `json:"config_digest"`
	// Failed marks a cycle that already reported a terminal failed status for
	// RunID. The backend keeps one terminal record per run, so resuming such a
	// cursor must open a new run rather than complete the failed one. An older
	// cursor without the field decodes to false, which is the pre-existing
	// behaviour.
	Failed bool `json:"failed"`
}

// cursorStore persists the cycle progress of each configured range.
type cursorStore interface {
	Load(autodiscoveryID string) (cursorState, bool)
	Save(autodiscoveryID string, s cursorState) error
	Clear(autodiscoveryID string) error
}

// persistentCursorStore stores cursors under the agent run_path. The sweeper
// that drives a discovery cycle (a later component) depends on the
// cursorStore interface rather than this concrete type, so this assertion is
// what ties the two together until that consumer exists.
var _ cursorStore = (*persistentCursorStore)(nil)

type persistentCursorStore struct{}

func newPersistentCursorStore() *persistentCursorStore {
	return &persistentCursorStore{}
}

func cursorKey(autodiscoveryID string) string {
	// persistentcache splits the key on ":" and uses the first part as the
	// directory name, so every cursor lands in one ndmdiscovery directory.
	return fmt.Sprintf("%s:%s", cursorKeyPrefix, autodiscoveryID)
}

func (s *persistentCursorStore) Load(autodiscoveryID string) (cursorState, bool) {
	raw, err := persistentcache.Read(cursorKey(autodiscoveryID))
	if err != nil || raw == "" {
		return cursorState{}, false
	}

	var state cursorState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		// A corrupt cursor is treated as no cursor: the cycle restarts.
		return cursorState{}, false
	}
	return state, true
}

func (s *persistentCursorStore) Save(autodiscoveryID string, state cursorState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return persistentcache.Write(cursorKey(autodiscoveryID), string(raw))
}

func (s *persistentCursorStore) Clear(autodiscoveryID string) error {
	return persistentcache.Write(cursorKey(autodiscoveryID), "")
}

// rangeDigest fingerprints the parts of a range config whose change makes a
// partial cycle meaningless: the addresses probed and the credentials used.
// The interval is deliberately excluded, so retiming a range keeps its
// progress.
func rangeDigest(cfg rangeConfig, creds []connectivity.SNMPCredential) string {
	h := sha256.New()
	fmt.Fprintf(h, "cidr=%s\n", cfg.CIDR)

	ignored := append([]string(nil), cfg.IgnoredIPAddresses...)
	sort.Strings(ignored)
	for _, ip := range ignored {
		fmt.Fprintf(h, "ignored=%s\n", ip)
	}

	fingerprints := make([]string, 0, len(creds))
	for _, c := range creds {
		fingerprints = append(fingerprints, fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			c.ID, c.Version, c.Community, c.User, c.AuthProtocol, c.AuthKey,
			c.PrivProtocol, c.PrivKey, c.ContextName, c.ContextEngineID))
	}
	sort.Strings(fingerprints)
	for _, f := range fingerprints {
		fmt.Fprintf(h, "cred=%s\n", f)
	}

	return hex.EncodeToString(h.Sum(nil))
}
