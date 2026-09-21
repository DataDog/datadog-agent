// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package credentials reads the SNMP credentials every NDM feature resolves
// the ids delivered over Remote Configuration against.
package credentials

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

// ConfigKey is the configuration key holding the credentials.
const ConfigKey = "network_devices.snmp_credentials"

// Credential is one entry of network_devices.snmp_credentials. The mapstructure
// tags match pkg/snmp.Authentication.
type Credential struct {
	ID              string `mapstructure:"id"`
	SNMPVersion     string `mapstructure:"snmp_version"`
	CommunityString string `mapstructure:"community_string"`
	User            string `mapstructure:"user"`
	AuthProtocol    string `mapstructure:"authProtocol"`
	AuthKey         string `mapstructure:"authKey"`
	PrivProtocol    string `mapstructure:"privProtocol"`
	PrivKey         string `mapstructure:"privKey"`
	ContextName     string `mapstructure:"context_name"`
	ContextEngineID string `mapstructure:"context_engine_id"`
}

// Store reads the credentials from the Agent configuration.
type Store struct {
	cfg model.Reader
}

// NewStore builds a store over the given configuration.
func NewStore(cfg model.Reader) *Store {
	return &Store{cfg: cfg}
}

// Load returns the configured credentials indexed by id, re-reading the
// configuration on every call. An entry with no id is skipped and the first
// of two entries sharing an id wins.
func (s *Store) Load() (map[string]Credential, error) {
	var entries []Credential
	if err := structure.UnmarshalKey(s.cfg, ConfigKey, &entries); err != nil {
		// The unmarshal error is dropped, not wrapped: it quotes the offending value.
		return nil, fmt.Errorf("failed to read %s", ConfigKey)
	}

	creds := make(map[string]Credential, len(entries))
	for _, e := range entries {
		if e.ID == "" {
			continue
		}
		if _, seen := creds[e.ID]; seen {
			continue
		}
		creds[e.ID] = e
	}
	return creds, nil
}

// Validate reports why a credential cannot be used, through the helpers the
// snmp check itself uses. No credential value ever reaches the returned error.
func Validate(c Credential) error {
	switch c.SNMPVersion {
	case "1", "2c":
		return nil
	case "3":
		// An empty protocol means "none".
		if c.AuthProtocol != "" {
			if _, err := gosnmplib.GetAuthProtocol(c.AuthProtocol); err != nil {
				return fmt.Errorf("credential %q has an unsupported authProtocol %q", c.ID, c.AuthProtocol)
			}
		}
		if c.PrivProtocol != "" {
			if _, err := gosnmplib.GetPrivProtocol(c.PrivProtocol); err != nil {
				return fmt.Errorf("credential %q has an unsupported privProtocol %q", c.ID, c.PrivProtocol)
			}
		}
		return nil
	default:
		return fmt.Errorf("credential %q has an unknown SNMP version %q (expected 1, 2c, or 3)", c.ID, c.SNMPVersion)
	}
}
