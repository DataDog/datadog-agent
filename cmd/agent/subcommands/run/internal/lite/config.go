// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package lite reports failures before the Agent has started.
package lite

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/create"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/spf13/cast"
	"go.yaml.in/yaml/v3"
)

// Params identifies the configuration sources selected by the run command.
type Params struct {
	ConfigPath        string
	DefaultConfigPath string
	ExtraConfigPaths  []string
	FleetPoliciesDir  string
}

type reportingConfig struct {
	model.BuildableConfig
	sensitive []string
}

// Only these settings can enter the private reporting configuration.
var reportingKeys = []string{
	"api_key", "site", "dd_url", "hostname", "fleet_policies_dir", "health_platform.enabled",
	"proxy.http", "proxy.https", "proxy.no_proxy", "skip_ssl_validation", "min_tls_version", "sslkeylogfile",
	"tls_handshake_timeout", "http_dial_fallback_delay", "no_proxy_nonexact_match",
	"use_proxy_for_cloud_metadata", "fips.enabled",
	"secret_backend_command", "secret_backend_arguments", "secret_backend_timeout",
	"secret_backend_output_max_size", "secret_backend_command_allow_group_exec_perm",
	"secret_backend_remove_trailing_line_break", "secret_backend_type",
	"secret_backend_config", "multi_secret_backends",
}

func recoverConfig(p Params) (*reportingConfig, string, error) {
	cfg := &reportingConfig{BuildableConfig: create.NewConfig("datadog")}
	setup.InitConfig(cfg)
	cfg.BuildSchema()
	for _, key := range reportingKeys {
		cfg.rememberSensitive(key, cfg.Get(key))
	}
	path, err := selectedPath(p)
	if err != nil {
		return nil, "", err
	}
	if err = loadSettings(cfg, path, model.SourceFile, p.ConfigPath == ""); err != nil {
		return nil, "", err
	}
	for _, extra := range p.ExtraConfigPaths {
		if err = loadSettings(cfg, extra, model.SourceFile, false); err != nil {
			return nil, "", err
		}
	}
	// Fleet policies outrank environment variables, just as in normal startup.
	setup.FleetConfigOverride(cfg)
	fleetDir := p.FleetPoliciesDir
	if fleetDir == "" {
		fleetDir = cfg.GetString("fleet_policies_dir")
	}
	if fleetDir != "" {
		if err = loadSettings(cfg, filepath.Join(fleetDir, "datadog.yaml"), model.SourceFleetPolicies, true); err != nil {
			return nil, "", err
		}
	}
	return cfg, path, nil
}

func selectedPath(p Params) (string, error) {
	path := p.ConfigPath
	if path == "" {
		path = p.DefaultConfigPath
	}
	if path == "" {
		return "", nil
	}
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return filepath.Abs(path)
	}
	return filepath.Abs(filepath.Join(path, "datadog.yaml"))
}

func loadSettings(cfg *reportingConfig, path string, source model.Source, optional bool) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if optional && os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return errors.New("cannot read selected reporting configuration")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("cannot read selected reporting configuration")
	}
	settings, err := reportingSettings(raw)
	if err != nil {
		return err
	}
	for k, value := range settings {
		if !validValue(k, value) {
			return fmt.Errorf("invalid reporting setting %s", k)
		}
		cfg.rememberSensitive(k, value)
		cfg.Set(k, value, source)
	}
	return nil
}

func reportingSettings(raw []byte) (map[string]interface{}, error) {
	var document map[string]interface{}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		var recoveryErr error
		document, recoveryErr = recoverBlocks(raw)
		if recoveryErr != nil {
			return nil, recoveryErr
		}
	}
	selected := map[string]interface{}{}
	for _, key := range reportingKeys {
		value, found, err := settingAt(document, strings.Split(key, "."))
		if err != nil {
			return nil, fmt.Errorf("ambiguous reporting setting %s", key)
		}
		if found {
			selected[key] = value
		}
	}
	return selected, nil
}

func settingAt(document map[string]interface{}, path []string) (interface{}, bool, error) {
	var result interface{}
	found := false
	wanted := strings.Join(path, ".")
	seen := map[string]bool{}
	for key, value := range document {
		// Normalize only reporting path names, never opaque backend map values.
		key = strings.ToLower(key)
		if key != wanted && key != path[0] {
			continue
		}
		if seen[key] {
			return nil, false, errors.New("duplicate reporting path")
		}
		seen[key] = true
		matched := true
		if key != wanted {
			nested, ok := value.(map[string]interface{})
			if !ok {
				return nil, false, errors.New("expected a mapping")
			}
			var err error
			value, matched, err = settingAt(nested, path[1:])
			if err != nil {
				return nil, false, err
			}
		}
		if matched {
			if found {
				return nil, false, errors.New("conflicting reporting paths")
			}
			result, found = value, true
		}
	}
	return result, found, nil
}

var topLevelKey = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.]*):(?:[ \t]|$)`)

// Invalid YAML is recoverable only as independent, plain top-level blocks.
// Indented content is never promoted to a top-level setting. Quoted keys,
// aliases, document boundaries and other ambiguous syntax fail closed.
func recoverBlocks(raw []byte) (map[string]interface{}, error) {
	blocks := map[string][]byte{}
	var order []string
	var key string
	for _, line := range bytes.SplitAfter(raw, []byte("\n")) {
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			match := topLevelKey.FindSubmatch(line)
			if match == nil {
				return nil, errors.New("ambiguous top-level configuration")
			}
			key = string(match[1])
			if _, exists := blocks[key]; exists {
				return nil, errors.New("duplicate configuration field")
			}
			order = append(order, key)
		}
		if key == "" {
			return nil, errors.New("ambiguous configuration indentation")
		}
		blocks[key] = append(blocks[key], line...)
	}
	document := map[string]interface{}{}
	for i, key := range order {
		block := blocks[key]
		var parsed map[string]interface{}
		if err := yaml.Unmarshal(block, &parsed); err != nil {
			// A later column-zero field may still belong to this unclosed block.
			if i != len(order)-1 || requiredRoot(key) || bytes.ContainsAny(block, "\"'&*|>") {
				return nil, errors.New("unrecoverable reporting configuration")
			}
			continue
		}
		if requiredRoot(key) {
			document[key] = parsed[key]
		}
	}
	return document, nil
}

func requiredRoot(key string) bool {
	key = strings.ToLower(strings.SplitN(key, ".", 2)[0])
	for _, wanted := range reportingKeys {
		if strings.SplitN(wanted, ".", 2)[0] == key {
			return true
		}
	}
	return false
}

// Validate transport inputs before getters can silently coerce malformed values
// to defaults. This is deliberately not validation of the Agent configuration.
func validateSettings(cfg model.Reader) error {
	for _, key := range reportingKeys {
		if value := cfg.Get(key); value != nil && !validValue(key, value) {
			return fmt.Errorf("invalid reporting setting %s", key)
		}
	}
	if cfg.GetBool("fips.enabled") {
		return errors.New("startup reporting cannot recover FIPS proxy routing")
	}
	switch strings.ToLower(cfg.GetString("min_tls_version")) {
	case "", "tlsv1.0", "tlsv1.1", "tlsv1.2", "tlsv1.3":
	default:
		return errors.New("invalid reporting TLS version")
	}
	return nil
}

func validValue(key string, value interface{}) bool {
	switch key {
	case "health_platform.enabled", "skip_ssl_validation", "no_proxy_nonexact_match", "use_proxy_for_cloud_metadata", "fips.enabled", "secret_backend_command_allow_group_exec_perm", "secret_backend_remove_trailing_line_break":
		_, err := strconv.ParseBool(fmt.Sprint(value))
		return err == nil
	case "tls_handshake_timeout", "http_dial_fallback_delay":
		_, err := cast.ToDurationE(value)
		return value != nil && err == nil
	case "secret_backend_timeout", "secret_backend_output_max_size":
		n, err := cast.ToIntE(value)
		return err == nil && n > 0
	case "secret_backend_config", "multi_secret_backends":
		_, ok := value.(map[string]interface{})
		return ok
	case "proxy.no_proxy", "secret_backend_arguments":
		return stringList(value)
	default:
		_, ok := value.(string)
		return ok
	}
}

func stringList(value interface{}) bool {
	switch values := value.(type) {
	case []string:
		return true
	case []interface{}:
		for _, item := range values {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validateDestination(cfg model.Reader) error {
	if cfg.IsConfigured("dd_url") {
		if err := validHTTPURL(cfg.GetString("dd_url")); err != nil {
			return err
		}
	} else if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*$`).MatchString(cfg.GetString("site")) {
		return errors.New("invalid reporting site")
	}
	for _, key := range []string{"proxy.http", "proxy.https"} {
		if value := cfg.GetString(key); value != "" {
			if err := validHTTPURL(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validHTTPURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
		return errors.New("invalid reporting destination or proxy")
	}
	return nil
}
