// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lite extracts a minimum bootstrap config from a possibly-broken datadog.yaml,
// without depending on the full agent config layer
package lite

// Source records where a ConfigField value was resolved from. Order somewhat mirrors
// the agent's source priority
type Source string

const (
	SourceNone          Source = "none"
	SourceDefault       Source = "default"
	SourceFileFuzzy     Source = "file_fuzzy"
	SourceFileRegex     Source = "file_regex"
	SourceFileYAMLTop   Source = "file_yaml_top"
	SourceFileYAMLFull  Source = "file_yaml_full"
	SourceEnv           Source = "env"
	SourceEncrypted     Source = "encrypted" // ENC[handle] not yet (or not) decrypted
	SourceSecretBackend Source = "secret_backend"
)

// DefaultSite is the Datadog site used when nothing else resolves it.
const DefaultSite = "datadoghq.com"

// ConfigField is a resolved value for a single lite-mode config key.
type ConfigField struct {
	Value      string
	Source     Source
	MatchedKey string // literal key text seen in the file (fuzzy/regex tiers only)
}

// resolved reports whether the field carries a usable value. Encrypted
// placeholders we could not resolve count as unresolved so the next tier can try
func (f ConfigField) resolved() bool {
	switch f.Source {
	case SourceNone, SourceEncrypted:
		return false
	}
	return f.Value != ""
}

// LiteConfig is the result of running Extract against env + datadog.yaml.
type LiteConfig struct {
	APIKey ConfigField
	Site   ConfigField
	DDURL  ConfigField

	// APIKeyCandidates holds alternative api_key candidates the fuzzy tier
	// found alongside APIKey (sorted best-to-worst by edit distance). The
	// rescue path tries each in order to find a working key.
	APIKeyCandidates []ConfigField

	// used to decrypt ENC[handle] placeholders
	SecretBackendCommand ConfigField

	ConfigFilePath string // absolute path of the datadog.yaml we read, or ""
	FileReadErr    error
	YAMLParseErr   error          // set if the full yaml.Unmarshal failed
	ParsedConfig   map[string]any // parse config success, for schema checks
}
