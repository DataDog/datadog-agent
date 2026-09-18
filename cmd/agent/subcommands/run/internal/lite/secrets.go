// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lite

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsimpl "github.com/DataDog/datadog-agent/comp/core/secrets/impl"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"go.yaml.in/yaml/v3"
)

func resolveSecrets(ctx context.Context, cfg *reportingConfig) error {
	var multi map[string]secrets.SecretBackendConfig
	if err := structure.UnmarshalKey(cfg, "multi_secret_backends", &multi); err != nil {
		return errors.New("invalid reporting secret backend configuration")
	}
	r := secretsimpl.NewOneShotResolver(ctx, noopsimpl.GetCompatComponent())
	r.Configure(secrets.ConfigParams{
		Command: cfg.GetString("secret_backend_command"), Arguments: cfg.GetStringSlice("secret_backend_arguments"),
		Type: cfg.GetString("secret_backend_type"), Config: cfg.GetStringMap("secret_backend_config"), MultiBackends: multi,
		Timeout: min(cfg.GetInt("secret_backend_timeout"), int(rescueTimeout.Seconds())), MaxSize: cfg.GetInt("secret_backend_output_max_size"),
		GroupExecPerm:   cfg.GetBool("secret_backend_command_allow_group_exec_perm"),
		RemoveLinebreak: cfg.GetBool("secret_backend_remove_trailing_line_break"),
	})
	keys := []string{"api_key", "hostname", "proxy.http", "proxy.https", "proxy.no_proxy"}
	if cfg.IsConfigured("dd_url") {
		keys = append(keys, "dd_url")
	} else {
		keys = append(keys, "site")
	}
	settings := map[string]interface{}{}
	for _, key := range keys {
		if cfg.IsConfigured(key) {
			settings[key] = cfg.Get(key)
		}
	}
	raw, err := yaml.Marshal(settings)
	if err != nil {
		return errors.New("cannot encode reporting settings")
	}
	resolvedKeys := map[string]bool{}
	r.SubscribeToChanges(func(_, _ string, path []string, _, _ any) {
		if len(path) > 0 {
			resolvedKeys[path[0]] = true
		}
	})
	resolved, err := r.Resolve(raw, "agent-startup", "", "", true)
	if err != nil {
		return errors.New("cannot resolve reporting secrets")
	}
	if bytes.Contains(resolved, []byte("ENC[")) {
		return errors.New("unresolved reporting secret")
	}
	if err = yaml.Unmarshal(resolved, &settings); err != nil {
		return errors.New("cannot decode reporting settings")
	}
	for key, value := range settings {
		if !resolvedKeys[key] {
			continue
		}
		if !validValue(key, value) {
			return errors.New("invalid resolved reporting setting")
		}
		cfg.rememberSensitive(key, value)
		cfg.Set(key, value, model.SourceSecret)
	}
	return nil
}

func (cfg *reportingConfig) rememberSensitive(key string, value interface{}) {
	switch key {
	case "api_key", "dd_url", "proxy.http", "proxy.https", "secret_backend_arguments", "secret_backend_config", "multi_secret_backends":
		cfg.sensitive = append(cfg.sensitive, sensitiveStrings(value)...)
	}
}

func sensitiveStrings(value interface{}) []string {
	var values []string
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		values = append(values, v)
		if u, err := url.Parse(v); err == nil && u.User != nil {
			values = append(values, u.User.Username())
			if password, ok := u.User.Password(); ok {
				values = append(values, password)
			}
		}
	case []string:
		values = append(values, v...)
	case []interface{}:
		for _, child := range v {
			values = append(values, sensitiveStrings(child)...)
		}
	case map[string]interface{}:
		for _, child := range v {
			values = append(values, sensitiveStrings(child)...)
		}
	}
	return values
}

func (cfg *reportingConfig) redact(message string) string {
	// Match complete credentials before shorter values they contain.
	slices.SortFunc(cfg.sensitive, func(a, b string) int { return len(b) - len(a) })
	var replacements []string
	for _, value := range cfg.sensitive {
		if value != "" {
			replacements = append(replacements, value, "********")
		}
	}
	return strings.NewReplacer(replacements...).Replace(message)
}
