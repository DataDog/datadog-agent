// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RshellCommandNamespacePrefix identifies restricted-shell commands.
const RshellCommandNamespacePrefix = "rshell:"

// Validate checks routing and warns about ineffective operator policy entries.
func Validate(config model.Reader) error {
	ddSite := site(config)
	_, err := apiBaseURL(config, ddSite)
	if err != nil {
		return err
	}
	if shouldWarnAboutRCOverride(config) {
		log.Warn("PAR is enabled with remote_configuration.rc_dd_url set but private_action_runner.par_dd_url unset; the RC override does not change PAR routing. Set private_action_runner.par_dd_url explicitly if PAR requires custom routing.")
	}

	warnBackslashPaths(config.GetStringSlice(par.RestrictedShellAllowedPaths))
	warnNonDirectoryPaths(config.GetStringSlice(par.RestrictedShellAllowedPaths))
	warnUnnamespacedCommands(par.RestrictedShellAllowedCommands, config.GetStringSlice(par.RestrictedShellAllowedCommands))
	warnUnnamespacedCommands(par.RestrictedShellPrivilegedElevatableCommands, config.GetStringSlice(par.RestrictedShellPrivilegedElevatableCommands))
	return nil
}

func warnUnnamespacedCommands(key string, commands []string) {
	for _, c := range commands {
		if !strings.HasPrefix(c, RshellCommandNamespacePrefix) {
			log.Warnf(
				"%s entry %q is missing the %q prefix and will never match a backend command; use %q instead",
				key,
				c,
				RshellCommandNamespacePrefix,
				RshellCommandNamespacePrefix+c,
			)
		}
	}
}

func warnBackslashPaths(paths []string) {
	for _, p := range paths {
		if strings.ContainsRune(p, '\\') {
			log.Warnf("%s entry %q contains a backslash; only forward-slash paths are supported and this entry will never match a backend rule",
				par.RestrictedShellAllowedPaths, p)
		}
	}
}

func warnNonDirectoryPaths(paths []string) {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			log.Warnf("%s entry %q is not a directory; rshell's sandbox only accepts directory entries and will drop this entry at runtime. Use the containing directory instead.",
				par.RestrictedShellAllowedPaths, p)
		}
	}
}
