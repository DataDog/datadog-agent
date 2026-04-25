// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxconfig is the installer CLI's bridge between `datadog.yaml` and
// the env-var-only `pkg/fleet/installer/env` package.
//
// The installer binary never reads `datadog.yaml`. The daemon (which already
// owns an fx config.Component) translates yaml into env vars when spawning
// the installer subprocess. For interactive CLI invocations
// (`sudo datadog-installer <cmd>`), this package plays the same role: it
// spins a minimal fx app in `PersistentPreRun`, reads every
// installer-relevant yaml field via the same `config.Component` the daemon
// uses, merges in any legacy DD_INSTALLER_REGISTRY_* prefix vars the user
// set, and exports a canonical set of `DD_*` env vars via `os.Setenv`
// (skipping anything already set in the process environment so explicit
// `DD_*` / CLI flags win). Legacy registry prefix vars are `Unsetenv`d
// after translation so the installer's `FromEnv` sees a single contract
// (`DD_INSTALLER_REGISTRY` JSON only).
package fxconfig

import (
	"os"
	"strconv"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// yamlSourcedKeys returns the set of dotted keys (e.g. "proxy.no_proxy")
// that appear in the datadog.yaml file. Unlike `cfg.GetSource(key) ==
// SourceFile`, this correctly identifies yaml-origin values even when
// post-init processing has replaced their source (e.g. proxy.* gets
// promoted to config-post-init after defaults are layered in, which
// would falsely hide user intent).
//
// We use AllSettingsBySource()[SourceFile] — that map only contains keys
// that were literally present in the yaml file, so it's a reliable
// signal of user intent regardless of downstream transformations.
func yamlSourcedKeys(cfg agentconfig.Component) map[string]bool {
	out := map[string]bool{}
	all := cfg.AllSettingsBySource()
	fromFile, ok := all[pkgconfigmodel.SourceFile].(map[string]interface{})
	if !ok {
		return out
	}
	flattenKeys("", fromFile, out)
	return out
}

// flattenKeys walks a nested `map[string]interface{}` and records every
// leaf path under `prefix` into `out`. A leaf is any non-map value.
func flattenKeys(prefix string, m map[string]interface{}, out map[string]bool) {
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if child, isMap := v.(map[string]interface{}); isMap {
			// Record the parent too — some callers care about the
			// presence of a subtree (e.g. `installer.registry.extensions`).
			out[path] = true
			flattenKeys(path, child, out)
			continue
		}
		out[path] = true
	}
}

// LoadAndExportEnv reads installer-relevant fields from datadog.yaml via fx
// and exports them as DD_* environment variables. Existing env vars are
// preserved so explicit values override the yaml. Legacy
// DD_INSTALLER_REGISTRY_{URL,AUTH,USERNAME,PASSWORD}[_<PKG>] vars are
// absorbed into a unified DD_INSTALLER_REGISTRY JSON blob (and then
// unset), so the installer binary sees a single registry contract.
//
// confFilePath is the path to the directory containing datadog.yaml (the
// installer CLI's --cfgpath flag). Empty means "use the fx default".
//
// No-op when DD_INSTALLER_FROM_DAEMON=true: the daemon already emitted
// every relevant DD_* env var from its own config.Component via
// Env.ToEnv() when spawning this subprocess, so re-reading yaml here
// would be redundant work.
//
// This is a best-effort bootstrap: any failure (missing file, parse error,
// permission denied) is logged and swallowed so the installer CLI can
// still proceed using the inherited process environment.
func LoadAndExportEnv(confFilePath string) {
	if os.Getenv("DD_INSTALLER_FROM_DAEMON") == "true" {
		return
	}
	// Always translate legacy DD_INSTALLER_REGISTRY_* prefix vars into
	// the canonical DD_INSTALLER_REGISTRY JSON. This runs independently of
	// fx so user-set overrides reach the installer even if the fx bootstrap
	// fails for any reason (broken yaml, missing dep, secrets provider
	// error, etc.).
	applyEnvOnlyRegistry()

	err := fxutil.OneShot(
		applyConfigToEnv,
		fx.Supply(core.BundleParams{
			ConfigParams: agentconfig.NewAgentParams(confFilePath, agentconfig.WithIgnoreErrors(true)),
			LogParams:    logdef.ForOneShot("INSTALLER", "off", false),
		}),
		core.Bundle(core.WithSecrets()),
	)
	if err != nil {
		pkglog.Warnf("fxconfig bootstrap failed (continuing with process env): %v", err)
	}
}

// applyEnvOnlyRegistry absorbs legacy DD_INSTALLER_REGISTRY_* prefix vars
// into DD_INSTALLER_REGISTRY (without yaml) and unsets the legacy vars.
// Safe to call multiple times: setEnvIfUnset preserves the first-written
// value, and the second call finds no legacy vars left to absorb.
func applyEnvOnlyRegistry() {
	registry, blob, err := env.BuildRegistryFromEnv()
	if err != nil {
		pkglog.Debugf("fxconfig: env-only registry fallback failed: %v", err)
		return
	}
	if !registry.IsEmpty() {
		setEnvIfUnset(env.EnvInstallerRegistry, blob)
	}
	unsetLegacyRegistryVars()
}

func applyConfigToEnv(cfg agentconfig.Component) {
	// Only emit env vars for values that actually came from datadog.yaml.
	// See yamlSourcedKeys for why we can't use cfg.GetSource(key) directly.
	fromYAML := yamlSourcedKeys(cfg)
	if fromYAML["api_key"] {
		setEnvIfUnset("DD_API_KEY", utils.SanitizeAPIKey(cfg.GetString("api_key")))
	}
	if fromYAML["site"] {
		setEnvIfUnset("DD_SITE", cfg.GetString("site"))
	}
	if fromYAML["hostname"] {
		setEnvIfUnset("DD_HOSTNAME", cfg.GetString("hostname"))
	}
	if fromYAML["installer.mirror"] {
		setEnvIfUnset("DD_INSTALLER_MIRROR", cfg.GetString("installer.mirror"))
	}
	if fromYAML["log_level"] {
		setEnvIfUnset("DD_LOG_LEVEL", cfg.GetString("log_level"))
	}
	if fromYAML["remote_updates"] {
		setEnvIfUnset("DD_REMOTE_UPDATES", strconv.FormatBool(cfg.GetBool("remote_updates")))
	}
	if fromYAML["proxy.http"] {
		setEnvIfUnset("DD_PROXY_HTTP", cfg.GetString("proxy.http"))
	}
	if fromYAML["proxy.https"] {
		setEnvIfUnset("DD_PROXY_HTTPS", cfg.GetString("proxy.https"))
	}
	if fromYAML["proxy.no_proxy"] {
		if np := cfg.GetStringSlice("proxy.no_proxy"); len(np) > 0 {
			setEnvIfUnset("DD_PROXY_NO_PROXY", strings.Join(np, ","))
		}
	}
	if fromYAML["tags"] || fromYAML["extra_tags"] {
		if tags := utils.GetConfiguredTags(cfg, false); len(tags) > 0 {
			setEnvIfUnset("DD_TAGS", strings.Join(tags, ","))
		}
	}

	// Registry: fold yaml `installer.registry.*` (incl. per-extension
	// entries) + already-set DD_INSTALLER_REGISTRY JSON (which includes
	// legacy prefix vars absorbed by the earlier applyEnvOnlyRegistry).
	// Unconditional Setenv so yaml extensions added here don't get lost
	// because the earlier env-only pass already set the env var.
	registry, blob, err := env.BuildRegistryFromConfigAndEnv(cfg)
	if err != nil {
		pkglog.Debugf("fxconfig: failed to build registry config (continuing): %v", err)
	} else if !registry.IsEmpty() {
		if err := os.Setenv(env.EnvInstallerRegistry, blob); err != nil {
			pkglog.Debugf("fxconfig: failed to set %s: %v", env.EnvInstallerRegistry, err)
		}
	}
	unsetLegacyRegistryVars()
}

// unsetLegacyRegistryVars removes DD_INSTALLER_REGISTRY_{URL,AUTH,USERNAME,
// PASSWORD}[_<PKG>] from the process environment after they've been
// absorbed into DD_INSTALLER_REGISTRY.
func unsetLegacyRegistryVars() {
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		for _, prefix := range env.LegacyRegistryEnvPrefixes {
			if key == prefix || strings.HasPrefix(key, prefix+"_") {
				_ = os.Unsetenv(key)
				break
			}
		}
	}
}

func setEnvIfUnset(key, value string) {
	if value == "" {
		return
	}
	if _, alreadySet := os.LookupEnv(key); alreadySet {
		return
	}
	if err := os.Setenv(key, value); err != nil {
		pkglog.Debugf("fxconfig: failed to set %s: %v", key, err)
	}
}
