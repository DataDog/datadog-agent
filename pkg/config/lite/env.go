// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "os"

// applyEnv resolves any unresolved field from the process environment.
// DD_DD_URL beats the legacy DD_URL — matching the agent's BindEnv order in
// pkg/config/setup/common_settings.go.
func applyEnv(cfg *LiteConfig) {
	set := func(field *ConfigField, vars ...string) {
		if field.resolved() {
			return
		}
		for _, v := range vars {
			if val := os.Getenv(v); val != "" {
				field.Value = val
				field.Source = SourceEnv
				return
			}
		}
	}
	set(&cfg.APIKey, "DD_API_KEY")
	set(&cfg.Site, "DD_SITE")
	set(&cfg.DDURL, "DD_DD_URL", "DD_URL")
	set(&cfg.SecretBackendCommand, "DD_SECRET_BACKEND_COMMAND")
}
