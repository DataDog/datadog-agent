// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

const credentialsConfigKey = "network_devices.discovery.credentials"

// credentialStore resolves the SNMP credentials referenced by a range config.
// Credentials arrive over Fleet Automation into the agent config, because
// Remote Configuration does not carry encrypted values yet.
type credentialStore interface {
	// Reload re-reads the credentials from their backing source.
	Reload() error
	// Get returns the credential with the given ID.
	Get(id string) (connectivity.SNMPCredential, bool)
}

// credentialEntry is the agent-config shape of one credential. The mapstructure
// tags match the naming already used by pkg/snmp.Authentication so that an
// operator writing both sections does not have to learn two spellings.
type credentialEntry struct {
	ID              string `mapstructure:"id"`
	Version         string `mapstructure:"snmp_version"`
	Community       string `mapstructure:"community_string"`
	User            string `mapstructure:"user"`
	AuthProtocol    string `mapstructure:"authProtocol"`
	AuthKey         string `mapstructure:"authKey"`
	PrivProtocol    string `mapstructure:"privProtocol"`
	PrivKey         string `mapstructure:"privKey"`
	ContextName     string `mapstructure:"context_name"`
	ContextEngineID string `mapstructure:"context_engine_id"`
}

// configCredentialStore reads credentials from the agent configuration.
type configCredentialStore struct {
	cfg model.Reader

	mu    sync.RWMutex
	creds map[string]connectivity.SNMPCredential
}

func newConfigCredentialStore(cfg model.Reader) *configCredentialStore {
	return &configCredentialStore{
		cfg:   cfg,
		creds: map[string]connectivity.SNMPCredential{},
	}
}

// Reload re-reads the credentials section. It is called at the start of every
// cycle so a Fleet-pushed rotation applies without an agent restart.
func (s *configCredentialStore) Reload() error {
	var entries []credentialEntry
	if err := structure.UnmarshalKey(s.cfg, credentialsConfigKey, &entries); err != nil {
		return fmt.Errorf("failed to read %s: %w", credentialsConfigKey, err)
	}

	creds := make(map[string]connectivity.SNMPCredential, len(entries))
	for _, e := range entries {
		if e.ID == "" {
			continue
		}
		creds[e.ID] = connectivity.SNMPCredential{
			ID:              e.ID,
			Version:         e.Version,
			Community:       e.Community,
			User:            e.User,
			AuthProtocol:    e.AuthProtocol,
			AuthKey:         e.AuthKey,
			PrivProtocol:    e.PrivProtocol,
			PrivKey:         e.PrivKey,
			ContextName:     e.ContextName,
			ContextEngineID: e.ContextEngineID,
		}
	}

	s.mu.Lock()
	s.creds = creds
	s.mu.Unlock()
	return nil
}

func (s *configCredentialStore) Get(id string) (connectivity.SNMPCredential, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.creds[id]
	return c, ok
}

// resolveCredentials maps a range's credential IDs to credentials, preserving
// the configured order so the most likely credential is tried first. The SNMP
// version is validated here: buildSNMPClient turns an unknown version into an
// error that cancels the whole chunk, so a bad credential must block the range
// before the sweep starts rather than half way through it.
func resolveCredentials(store credentialStore, ids []string) ([]connectivity.SNMPCredential, error) {
	if len(ids) == 0 {
		return nil, errors.New("the range references no credentials")
	}

	creds := make([]connectivity.SNMPCredential, 0, len(ids))
	for _, id := range ids {
		c, ok := store.Get(id)
		if !ok {
			return nil, fmt.Errorf("credential %q is not available on this agent", id)
		}
		switch c.Version {
		case "1", "2c", "3":
		default:
			return nil, fmt.Errorf("credential %q has an unknown SNMP version %q (expected 1, 2c, or 3)", id, c.Version)
		}
		creds = append(creds, c)
	}
	return creds, nil
}
