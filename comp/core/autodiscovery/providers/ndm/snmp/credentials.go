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

// credentialsDir and credentialsFilename locate the credential file Fleet
// Automation writes, relative to confd_path. The file sits in a subdirectory
// because the file config provider collects every yaml directly under snmp.d
// as a check config.
const (
	credentialsDir      = "snmp.d/credentials"
	credentialsFilename = "snmp_credentials.yaml"
)

// credential is one entry of the credential file. The yaml names match
// pkg/snmp.Authentication. ID is the join key an RC instance references, Name
// is a human label.
type credential struct {
	ID              string `yaml:"id"`
	Name            string `yaml:"name"`
	SNMPVersion     string `yaml:"snmp_version"`
	CommunityString string `yaml:"community_string"`
	User            string `yaml:"user"`
	AuthProtocol    string `yaml:"authProtocol"`
	AuthKey         string `yaml:"authKey"`
	PrivProtocol    string `yaml:"privProtocol"`
	PrivKey         string `yaml:"privKey"`
	ContextName     string `yaml:"context_name"`
	ContextEngineID string `yaml:"context_engine_id"`
}

// credentialsDocument is the credential file.
type credentialsDocument struct {
	Credentials []credential `yaml:"credentials"`
}

// credentialStore reads the credential file Fleet Automation writes to
// conf.d/snmp.d/credentials/snmp_credentials.yaml.
type credentialStore struct {
	cfg model.Reader
}

func newCredentialStore(cfg model.Reader) *credentialStore {
	return &credentialStore{cfg: cfg}
}

// path returns where the credential file is expected.
func (s *credentialStore) path() string {
	return filepath.Join(s.cfg.GetString("confd_path"), credentialsDir, credentialsFilename)
}

// load returns the credentials indexed by id, re-reading the file on every
// call. An absent file is an empty set. An entry with no id is skipped and the
// first of two entries sharing an id wins.
func (s *credentialStore) load() (map[string]credential, error) {
	doc, err := readCredentialsFile(s.path())
	if err != nil {
		return nil, err
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

// readCredentialsFile parses the credential file. No file content ever reaches
// the returned error.
func readCredentialsFile(path string) (credentialsDocument, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return credentialsDocument{}, nil
	}
	if err != nil {
		// The fs.PathError already names the file.
		return credentialsDocument{}, fmt.Errorf("failed to read the credentials file: %w", err)
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
		if c.CommunityString == "" {
			return fmt.Errorf("credential %q is SNMP version %q and has no community_string", c.Name, c.SNMPVersion)
		}
		return nil
	case "3":
		if c.User == "" {
			return fmt.Errorf("credential %q is SNMP version 3 and has no user", c.Name)
		}
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
