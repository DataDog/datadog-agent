// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxconfig is the installer CLI's bridge between `datadog.yaml` and
// the env-var-only `pkg/fleet/installer/env` package.
//
// The installer proper never reads `datadog.yaml`. The daemon (which already
// owns an fx config.Component) translates yaml into env vars when spawning
// the installer subprocess. For interactive invocations
// (`sudo datadog-installer <cmd>`), this package plays the same role: it
// spins a minimal fx app, reads every installer-relevant yaml field via the
// same `config.Component` the daemon uses, and exports the values as `DD_*`
// env vars via `os.Setenv` — skipping anything already set in the process
// environment so explicit `DD_*` / CLI flags win.
//
// Callers wire `LoadAndExportEnv` into the root Cobra command's
// `PersistentPreRun`, so every subcommand that reaches `env.Get()` sees a
// fully env-populated process.
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
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// LoadAndExportEnv reads installer-relevant fields from datadog.yaml via fx
// and exports them as DD_* environment variables. Existing env vars are
// preserved so explicit values override the yaml.
//
// confFilePath is the path to the directory containing datadog.yaml (the
// installer CLI's `--cfgpath` flag). Empty means "use the fx default".
//
// This is a best-effort bootstrap: any failure (missing file, parse error,
// permission denied) is logged and swallowed so the installer CLI can still
// proceed using the inherited process environment.
func LoadAndExportEnv(confFilePath string) {
	err := fxutil.OneShot(
		applyConfigToEnv,
		fx.Supply(core.BundleParams{
			ConfigParams: agentconfig.NewAgentParams(confFilePath, agentconfig.WithIgnoreErrors(true)),
			LogParams:    logdef.ForOneShot("INSTALLER", "off", false),
		}),
		core.Bundle(core.WithSecrets()),
	)
	if err != nil {
		// Best-effort: fall back to whatever the caller's environment
		// already has. The installer's env-only code path will still run.
		pkglog.Debugf("fxconfig bootstrap failed (continuing with process env): %v", err)
	}
}

// applyConfigToEnv is the fx.Invoke target. It receives the resolved
// config.Component and translates the installer-relevant fields to env vars.
func applyConfigToEnv(cfg agentconfig.Component) {
	// Scalars: simple (yamlKey, envVar) pairs. Sanitize the api_key the same
	// way the daemon does.
	setEnvIfUnset("DD_API_KEY", utils.SanitizeAPIKey(cfg.GetString("api_key")))
	setEnvIfUnset("DD_SITE", cfg.GetString("site"))
	setEnvIfUnset("DD_HOSTNAME", cfg.GetString("hostname"))
	setEnvIfUnset("DD_INSTALLER_MIRROR", cfg.GetString("installer.mirror"))
	setEnvIfUnset("DD_INSTALLER_REGISTRY_URL", cfg.GetString("installer.registry.url"))
	setEnvIfUnset("DD_INSTALLER_REGISTRY_AUTH", cfg.GetString("installer.registry.auth"))
	setEnvIfUnset("DD_INSTALLER_REGISTRY_USERNAME", cfg.GetString("installer.registry.username"))
	setEnvIfUnset("DD_INSTALLER_REGISTRY_PASSWORD", cfg.GetString("installer.registry.password"))
	setEnvIfUnset("DD_LOG_LEVEL", cfg.GetString("log_level"))

	if cfg.IsSet("remote_updates") {
		setEnvIfUnset("DD_REMOTE_UPDATES", strconv.FormatBool(cfg.GetBool("remote_updates")))
	}

	// Proxy: the installer env package reads all three of http_proxy /
	// HTTP_PROXY / DD_PROXY_HTTP (last wins). We export the DD_PROXY_*
	// variant so a user's pre-existing http_proxy lower-precedence entry
	// is still respected.
	setEnvIfUnset("DD_PROXY_HTTP", cfg.GetString("proxy.http"))
	setEnvIfUnset("DD_PROXY_HTTPS", cfg.GetString("proxy.https"))
	if np := cfg.GetStringSlice("proxy.no_proxy"); len(np) > 0 {
		setEnvIfUnset("DD_PROXY_NO_PROXY", strings.Join(np, ","))
	}

	// Tags: the daemon uses utils.GetConfiguredTags(cfg, false) to merge
	// `tags:` and `extra_tags:`. Mirror that: what the daemon emits from
	// its own Env.ToEnv() is a single DD_TAGS value.
	if tags := utils.GetConfiguredTags(cfg, false); len(tags) > 0 {
		setEnvIfUnset("DD_TAGS", strings.Join(tags, ","))
	}

	// installer.registry.extensions.<pkg>.<ext>.{url,auth,username,password}
	// → DD_INSTALLER_REGISTRY_EXT_{URL,AUTH,USERNAME,PASSWORD}_<PKG>__<EXT>
	// matching the prefix scheme env.Env round-trips.
	exportExtensionOverrides(cfg.GetStringMap("installer.registry.extensions"))
}

// exportExtensionOverrides translates the nested yaml map into env vars. The
// key convention matches `parseExtensionRegistryEnv` in pkg/fleet/installer/env.
func exportExtensionOverrides(raw map[string]interface{}) {
	if len(raw) == 0 {
		return
	}
	const (
		urlVar      = "DD_INSTALLER_REGISTRY_EXT_URL_"
		authVar     = "DD_INSTALLER_REGISTRY_EXT_AUTH_"
		usernameVar = "DD_INSTALLER_REGISTRY_EXT_USERNAME_"
		passwordVar = "DD_INSTALLER_REGISTRY_EXT_PASSWORD_"
	)
	for pkg, extMapAny := range raw {
		extMap, ok := extMapAny.(map[string]interface{})
		if !ok {
			continue
		}
		pkgKey := envKey(pkg)
		for ext, extCfgAny := range extMap {
			extCfg, ok := extCfgAny.(map[string]interface{})
			if !ok {
				continue
			}
			suffix := pkgKey + "__" + envKey(ext)
			if s, ok := extCfg["url"].(string); ok {
				setEnvIfUnset(urlVar+suffix, s)
			}
			if s, ok := extCfg["auth"].(string); ok {
				setEnvIfUnset(authVar+suffix, s)
			}
			if s, ok := extCfg["username"].(string); ok {
				setEnvIfUnset(usernameVar+suffix, s)
			}
			if s, ok := extCfg["password"].(string); ok {
				setEnvIfUnset(passwordVar+suffix, s)
			}
		}
	}
}

// envKey uppercases a yaml key and maps `-` to `_`, matching the parser's
// reverse mapping in pkg/fleet/installer/env.
func envKey(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
}

// setEnvIfUnset writes value under key only when key is not already present
// in the process environment. Empty values are skipped — they would add
// no information and could mask an env.Get() default.
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
