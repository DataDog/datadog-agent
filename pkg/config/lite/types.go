// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lite extracts a minimum bootstrap config (api_key, site, dd_url and
// optionally secret_backend_command) from a possibly-broken datadog.yaml,
// without depending on the full agent config layer. Used by the Agent Health
// Platform forwarder and by the agent's failure-path rescue hook.
//
// Resolution walks a tiered pipeline (env → full YAML → indent-stripped YAML
// → column-0 regex → fuzzy match → defaults) and resolves ENC[handle] values
// via secret_backend_command. The first tier to produce a value wins.
package lite

// Source records where a ConfigField value was resolved from. Order mirrors
// the agent's source priority (env beats file beats default).
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
// placeholders we could not resolve count as unresolved so the next tier can
// try.
func (f ConfigField) resolved() bool {
	switch f.Source {
	case SourceNone, SourceEncrypted:
		return false
	}
	return f.Value != ""
}

// Config is the result of running Extract against env + a candidate
// datadog.yaml.
type Config struct {
	APIKey ConfigField
	Site   ConfigField
	DDURL  ConfigField

	// APIKeyCandidates holds alternative api_key candidates the fuzzy tier
	// found alongside APIKey (sorted best-to-worst by edit distance). The
	// rescue path tries each in order when the primary 401s — this is how
	// we handle the case where `app_key` and a typo'd `api_kye` are both
	// distance 1 from "api_key" with no way to distinguish them statically.
	APIKeyCandidates []ConfigField

	// SecretBackendCommand is resolved through the same pipeline as the
	// credentials. It is consulted only to decrypt ENC[handle] placeholders
	// found in APIKey / Site / DDURL.
	SecretBackendCommand ConfigField

	ConfigFilePath string         // absolute path of the datadog.yaml we read, or ""
	FileReadErr    error          // set if a file was located but reading it failed
	YAMLParseErr   error          // set if the Tier-2 full yaml.Unmarshal failed
	ParsedConfig   map[string]any // Tier-2 result on success, for downstream schema checks
}
