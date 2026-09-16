// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

// credentialsFile is the credential file Fleet Automation writes, relative to
// confd_path.
const credentialsFile = "snmp.d/snmp_credentials.yaml"

// credential is one entry of the credentials file. The yaml names match
// pkg/snmp.Authentication.
type credential struct {
	ID              string `yaml:"id"`
	SNMPVersion     string `yaml:"snmp_version"`
	CommunityString string `yaml:"community_string"`
	User            string `yaml:"user"`
	AuthProtocol    string `yaml:"authProtocol"`
	AuthKey         string `yaml:"authKey"`
	PrivProtocol    string `yaml:"privProtocol"`
	PrivKey         string `yaml:"privKey"`
	ContextName     string `yaml:"context_name"`
	// context_engine_id is absent: the snmp check's InstanceConfig has no such field.
}

// credentialsDocument is the credential file Fleet Automation writes.
type credentialsDocument struct {
	Credentials []credential `yaml:"credentials"`
}

// credentialStore reads the credential file Fleet Automation writes next to
// the snmp check configuration.
type credentialStore struct {
	cfg model.Reader
}

func newCredentialStore(cfg model.Reader) *credentialStore {
	return &credentialStore{cfg: cfg}
}

// path returns where the credential file is expected.
func (s *credentialStore) path() string {
	return filepath.Join(s.cfg.GetString("confd_path"), credentialsFile)
}

// load returns the credentials indexed by id, re-reading the file on every
// call. An absent file is an empty set, not an error. An entry with no id is
// skipped and the first of two entries sharing an id wins.
func (s *credentialStore) load() (map[string]credential, error) {
	path := s.path()

	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]credential{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var doc credentialsDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		// The parse error is dropped, not wrapped: it quotes the offending value.
		return nil, fmt.Errorf("failed to parse %s", path)
	}

	creds := make(map[string]credential, len(doc.Credentials))
	for _, e := range doc.Credentials {
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

// validate reports why a credential cannot produce a usable check instance,
// through the helpers the snmp check itself uses. No credential value ever
// reaches the returned error.
func validate(c credential) error {
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
