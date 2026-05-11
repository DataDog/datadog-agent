// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lite extracts the minimum configuration the Agent Health Platform
// needs (api_key, site, dd_url, optionally secret_backend_command) from a
// possibly-broken datadog.yaml, without depending on the full agent config
// layer. It is meant to bootstrap the Health Platform forwarder when the
// agent's normal startup has failed.
//
// The package implements a small tiered resolver pipeline that walks each
// field through progressively-less-confident strategies until something
// produces a value:
//
//  1. Environment variables (DD_API_KEY, DD_SITE, DD_DD_URL, DD_URL)
//  2. Full yaml.Unmarshal of the raw bytes
//  3. yaml.Unmarshal after stripping indented lines (top-level only)
//  4. Column-0 anchored regex on the raw bytes
//  5. Damerau-Levenshtein fuzzy match on top-level keys (typo tolerance)
//  6. Defaults (site only)
//
// ENC[handle] placeholders surfaced by any of those tiers are resolved via
// the configured secret_backend_command (which itself comes through the same
// pipeline). If secret resolution fails the field is marked SourceEncrypted
// and treated as unresolved so the next tier can try.
package lite

// Source records where a ConfigField value was resolved from. It mirrors the
// agent's source priority order (env beats file beats default).
type Source string

const (
	// SourceNone means the field was never resolved.
	SourceNone Source = "none"
	// SourceDefault means the field was filled with a built-in default.
	SourceDefault Source = "default"
	// SourceFileFuzzy means a top-level key matched the field name within a
	// small Damerau-Levenshtein distance (typo tolerance).
	SourceFileFuzzy Source = "file_fuzzy"
	// SourceFileRegex means a column-0 anchored regex matched the field name
	// exactly in the raw bytes.
	SourceFileRegex Source = "file_regex"
	// SourceFileYAMLTop means the field was found by yaml.Unmarshal after the
	// file was stripped to top-level lines only.
	SourceFileYAMLTop Source = "file_yaml_top"
	// SourceFileYAMLFull means the entire file parsed as YAML and the field
	// was present at the top level.
	SourceFileYAMLFull Source = "file_yaml_full"
	// SourceEnv means the field came from an environment variable.
	SourceEnv Source = "env"
	// SourceEncrypted means the field was found as an ENC[handle] placeholder
	// that we could not (or have not yet) resolved. Callers must not use the
	// stored value as a credential.
	SourceEncrypted Source = "encrypted"
	// SourceSecretBackend means the field's ENC[handle] was resolved through
	// secret_backend_command into a plaintext value.
	SourceSecretBackend Source = "secret_backend"
)

// DefaultSite is the Datadog site used when nothing else resolves it.
const DefaultSite = "datadoghq.com"

// ConfigField is a resolved value for a single lite-mode config key.
type ConfigField struct {
	// Value is the resolved string, or "" if Source is SourceNone.
	Value string
	// Source records which tier produced Value.
	Source Source
	// MatchedKey is the literal key text seen in the file when Source is
	// SourceFileFuzzy or SourceFileRegex; empty otherwise.
	MatchedKey string
}

// resolved reports whether the field carries a usable value. Encrypted
// placeholders that we could not resolve count as unresolved so the next tier
// can try.
func (f ConfigField) resolved() bool {
	switch f.Source {
	case SourceNone, SourceEncrypted:
		return false
	}
	return f.Value != ""
}

// LiteConfig is the result of running Extract against env + a candidate
// datadog.yaml.
type LiteConfig struct {
	APIKey ConfigField
	Site   ConfigField
	DDURL  ConfigField

	// SecretBackendCommand is resolved through the same pipeline as the
	// credentials. It is consulted only to decrypt ENC[handle] placeholders
	// found in APIKey / Site / DDURL.
	SecretBackendCommand ConfigField

	// ConfigFilePath is the absolute path of the datadog.yaml we read, or "".
	ConfigFilePath string
	// FileReadErr is set if a file was located but reading it failed.
	FileReadErr error
	// YAMLParseErr is set if the Tier-2 full yaml.Unmarshal failed.
	// Downstream callers (the invalidconfig issue module, lite Rescue) use
	// this to decide whether to raise a yaml_parse vs schema_validation issue.
	YAMLParseErr error
	// ParsedConfig holds the result of the Tier-2 full parse when it
	// succeeded. Downstream callers may feed this into schema.ValidateCoreConfig
	// to avoid re-parsing.
	ParsedConfig map[string]any
}
