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
	"strconv"
	"strings"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

// credentialsFile is the credential file Fleet Automation writes, relative to
// confd_path.
const credentialsFile = "snmp.d/snmp_credentials.yaml"

// credentialNameTag is the tag prefix carrying a credential's id.
const credentialNameTag = "credential-name:"

// version is an snmp_version field, which Fleet Automation writes as an
// integer and the snmp check reads as a string.
type version string

// UnmarshalYAML accepts both an integer and a string.
func (v *version) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch value := raw.(type) {
	case string:
		*v = version(value)
	case int:
		*v = version(strconv.Itoa(value))
	default:
		return errors.New("snmp_version is neither a string nor an integer")
	}
	return nil
}

// credential is one instance of the credentials file. The yaml names match
// the snmp check's instance config, so the file is a valid snmp check
// configuration.
type credential struct {
	ID              string   `yaml:"-"`
	Tags            []string `yaml:"tags"`
	SNMPVersion     version  `yaml:"snmp_version"`
	CommunityString string   `yaml:"community_string"`
	User            string   `yaml:"user"`
	AuthProtocol    string   `yaml:"authProtocol"`
	AuthKey         string   `yaml:"authKey"`
	PrivProtocol    string   `yaml:"privProtocol"`
	PrivKey         string   `yaml:"privKey"`
	ContextName     string   `yaml:"context_name"`
	// context_engine_id is absent: the snmp check's InstanceConfig has no such field.
}

// credentialsDocument is the credential file Fleet Automation writes: an
// snmp check configuration whose instances carry the credentials.
type credentialsDocument struct {
	Credentials []credential `yaml:"instances"`
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
// call. An absent file is an empty set, not an error. An instance carrying no
// credential-name tag is skipped and the first of two sharing an id wins.
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
		e.ID = credentialName(e.Tags)
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

// credentialName returns the id an instance's tags carry, or "".
func credentialName(tags []string) string {
	for _, tag := range tags {
		if name, found := strings.CutPrefix(tag, credentialNameTag); found {
			return name
		}
	}
	return ""
}

// validate reports why a credential cannot produce a usable check instance,
// through the helpers the snmp check itself uses. No credential value ever
// reaches the returned error.
func validate(c credential) error {
	switch c.SNMPVersion {
	case "1", "2", "2c":
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
		return fmt.Errorf("credential %q has an unknown SNMP version %q (expected 1, 2, 2c, or 3)", c.ID, c.SNMPVersion)
	}
}
