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

// credentialsDir is where Fleet Automation writes the credential files,
// relative to confd_path.
const credentialsDir = "snmp.d/credentials"

// credential is one entry of a credential file. The yaml names match
// pkg/snmp.Authentication.
type credential struct {
	Name            string `yaml:"name"`
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

// credentialsDocument is one credential file.
type credentialsDocument struct {
	Credentials []credential `yaml:"credentials"`
}

// credentialStore reads the credential files Fleet Automation writes under
// conf.d/snmp.d/credentials.
type credentialStore struct {
	cfg model.Reader
}

func newCredentialStore(cfg model.Reader) *credentialStore {
	return &credentialStore{cfg: cfg}
}

// dir returns where the credential files are expected.
func (s *credentialStore) dir() string {
	return filepath.Join(s.cfg.GetString("confd_path"), credentialsDir)
}

// load returns the credentials indexed by name, re-reading the files on every
// call. An absent directory is an empty set. A file that cannot be read or
// parsed is skipped and named in the returned error, the others still load. An
// entry with no name is skipped and the first of two entries sharing a name
// wins.
func (s *credentialStore) load() (map[string]credential, error) {
	dir := s.dir()

	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", dir, err)
	}

	creds := make(map[string]credential, len(paths))
	var failures []error

	for _, path := range paths {
		doc, err := readCredentialsFile(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, e := range doc.Credentials {
			if e.Name == "" {
				continue
			}
			if _, seen := creds[e.Name]; seen {
				continue
			}
			creds[e.Name] = e
		}
	}

	return creds, errors.Join(failures...)
}

// readCredentialsFile parses one credential file. No file content ever reaches
// the returned error.
func readCredentialsFile(path string) (credentialsDocument, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return credentialsDocument{}, nil
	}
	if err != nil {
		return credentialsDocument{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var doc credentialsDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		// The parse error is dropped, not wrapped: it quotes the offending value.
		return credentialsDocument{}, fmt.Errorf("failed to parse %s", path)
	}
	return doc, nil
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
				return fmt.Errorf("credential %q has an unsupported authProtocol %q", c.Name, c.AuthProtocol)
			}
		}
		if c.PrivProtocol != "" {
			if _, err := gosnmplib.GetPrivProtocol(c.PrivProtocol); err != nil {
				return fmt.Errorf("credential %q has an unsupported privProtocol %q", c.Name, c.PrivProtocol)
			}
		}
		return nil
	default:
		return fmt.Errorf("credential %q has an unknown SNMP version %q (expected 1, 2c, or 3)", c.Name, c.SNMPVersion)
	}
}
