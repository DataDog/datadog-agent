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
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	// Env-only fallback: always translate any remaining legacy
	// DD_INSTALLER_REGISTRY_{URL,AUTH,USERNAME,PASSWORD}[_<PKG>] vars
	// into the canonical DD_INSTALLER_REGISTRY JSON. When fx succeeded
	// this is a no-op (applyConfigToEnv already absorbed and unset them);
	// when fx failed before invoking applyConfigToEnv this is the safety
	// net so user-set overrides aren't silently dropped.
	applyEnvOnlyRegistry()
}

// applyEnvOnlyRegistry absorbs legacy DD_INSTALLER_REGISTRY_* prefix vars
// into DD_INSTALLER_REGISTRY (without yaml) and unsets the legacy vars.
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
	setEnvIfUnset("DD_API_KEY", utils.SanitizeAPIKey(cfg.GetString("api_key")))
	setEnvIfUnset("DD_SITE", cfg.GetString("site"))
	setEnvIfUnset("DD_HOSTNAME", cfg.GetString("hostname"))
	setEnvIfUnset("DD_INSTALLER_MIRROR", cfg.GetString("installer.mirror"))
	setEnvIfUnset("DD_LOG_LEVEL", cfg.GetString("log_level"))

	if cfg.IsConfigured("remote_updates") {
		setEnvIfUnset("DD_REMOTE_UPDATES", strconv.FormatBool(cfg.GetBool("remote_updates")))
	}

	setEnvIfUnset("DD_PROXY_HTTP", cfg.GetString("proxy.http"))
	setEnvIfUnset("DD_PROXY_HTTPS", cfg.GetString("proxy.https"))
	// Only emit DD_PROXY_NO_PROXY when the user explicitly set
	// proxy.no_proxy in yaml. GetStringSlice always returns the effective
	// value including cloud-metadata defaults (169.254.169.254 etc.),
	// which would leak into downstream datadog.yaml writes.
	if cfg.IsConfigured("proxy.no_proxy") {
		if np := cfg.GetStringSlice("proxy.no_proxy"); len(np) > 0 {
			setEnvIfUnset("DD_PROXY_NO_PROXY", strings.Join(np, ","))
		}
	}

	// Only emit DD_TAGS when explicitly configured. Default config can
	// pull in host tags from cloud metadata which would then overwrite
	// the user's explicit "no tags" choice in datadog.yaml.
	if cfg.IsConfigured("tags") || cfg.IsConfigured("extra_tags") {
		if tags := utils.GetConfiguredTags(cfg, false); len(tags) > 0 {
			setEnvIfUnset("DD_TAGS", strings.Join(tags, ","))
		}
	}

	// Registry: fold yaml `installer.registry.*` (incl. per-extension
	// entries) + legacy DD_INSTALLER_REGISTRY_* prefix vars + any
	// already-set DD_INSTALLER_REGISTRY JSON into a single canonical
	// JSON blob. Unset the legacy prefix vars so the installer's FromEnv
	// sees a clean contract.
	registry, blob, err := env.BuildRegistryFromConfigAndEnv(cfg)
	if err != nil {
		pkglog.Debugf("fxconfig: failed to build registry config (continuing): %v", err)
	} else if !registry.IsEmpty() {
		setEnvIfUnset(env.EnvInstallerRegistry, blob)
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
