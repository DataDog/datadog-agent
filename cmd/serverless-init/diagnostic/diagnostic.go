// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package diagnostic provides optional startup-time diagnostic logging for
// serverless-init. Enable by setting DD_SERVERLESS_DIAGNOSTIC_INFO=true.
//
// When enabled, logs all environment variables (with secrets masked), key
// agent configuration values, and mode/origin detection results on startup.
//
// WARNING: Output may contain sensitive values. Do not enable in production.
package diagnostic

import (
	"os"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const diagnosticEnvVar = "DD_SERVERLESS_DIAGNOSTIC_INFO"

// secretKeywords are substrings whose presence in an env var name triggers masking.
var secretKeywords = []string{"API_KEY", "SECRET", "PASSWORD", "TOKEN"}

// LogIfEnabled logs diagnostic startup information when DD_SERVERLESS_DIAGNOSTIC_INFO=true.
// Call once after DetectMode and GetCloudServiceType, before fxutil.OneShot.
func LogIfEnabled(modeConf mode.Conf, cs cloudservice.CloudService) {
	if strings.ToLower(os.Getenv(diagnosticEnvVar)) != "true" {
		return
	}

	log.Infof("[SERVERLESS_DIAGNOSTIC] mode=%s sidecar=%v origin=%s",
		modeConf.LoggerName, modeConf.SidecarMode, cs.GetOrigin())

	envVars := os.Environ()
	sort.Strings(envVars)
	log.Infof("[SERVERLESS_DIAGNOSTIC] env_var_count=%d", len(envVars))
	for _, e := range envVars {
		log.Infof("[SERVERLESS_DIAGNOSTIC] env: %s", maskSecret(e))
	}

	cfg := pkgconfigsetup.Datadog()
	log.Infof("[SERVERLESS_DIAGNOSTIC] config: dd_site=%s logs_enabled=%v apm_enabled=%v rc_enabled=%v api_key_present=%v",
		cfg.GetString("dd_site"),
		cfg.GetBool("logs_enabled"),
		cfg.GetBool("apm_config.enabled"),
		cfg.GetBool("remote_configuration.enabled"),
		cfg.GetString("api_key") != "",
	)
}

// maskSecret replaces the value portion of a KEY=value pair with *** when the
// key contains a known secret keyword. Non-secret vars are returned unchanged.
func maskSecret(kv string) string {
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return kv
	}
	upper := strings.ToUpper(parts[0])
	for _, keyword := range secretKeywords {
		if strings.Contains(upper, keyword) {
			return parts[0] + "=***"
		}
	}
	return kv
}
