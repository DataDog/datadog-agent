// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// paramDefaultRe matches top-level @param lines that declare a default value.
// Format: ## @param KEY - TYPE - (optional|required) - default: VALUE
var paramDefaultRe = regexp.MustCompile(`^## @param (\S+) - [^-]+ - (?:optional|required) - default: (.+)$`)

// parseTopLevelParamDefaults reads config_template.yaml and returns a map of
// config key → documented default value string.
// It only parses top-level "## @param" lines (nested keys use a different comment style).
// Complex defaults (lists and maps) and Go-template expressions are skipped.
func parseTopLevelParamDefaults(t *testing.T) map[string]string {
	t.Helper()

	f, err := os.Open("../config_template.yaml")
	if err != nil {
		t.Fatalf("failed to open config_template.yaml: %v", err)
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := paramDefaultRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := m[1]
		rawDefault := strings.TrimSpace(m[2])

		// Skip Go template expressions (e.g. OS-conditional defaults).
		if strings.HasPrefix(rawDefault, "{{") {
			continue
		}
		// Skip list and map types.
		if strings.HasPrefix(rawDefault, "[") || strings.HasPrefix(rawDefault, "{") {
			continue
		}

		// Normalize: strip surrounding double-quotes for string values.
		normalized := rawDefault
		if len(rawDefault) >= 2 && rawDefault[0] == '"' && rawDefault[len(rawDefault)-1] == '"' {
			normalized = rawDefault[1 : len(rawDefault)-1]
		}

		result[key] = normalized
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading config_template.yaml: %v", err)
	}
	return result
}

// TestConfigTemplateDefaultsMatchCode verifies that the default values documented
// in @param comments in config_template.yaml match the actual defaults registered
// by InitConfig. A mismatch means either the template comment or the code is stale.
//
// Keys in knownExceptions are intentionally different: the template documents a
// user-visible or computed effective default, while the code leaves the key without
// a programmatic default so that viper can detect whether the user explicitly set it.
func TestConfigTemplateDefaultsMatchCode(t *testing.T) {
	cfg := newTestConf(t)
	templateDefaults := parseTopLevelParamDefaults(t)

	// Keys where the template intentionally documents a user-visible/effective default
	// that differs from the raw code default. Each entry must include a reason.
	knownExceptions := map[string]string{
		// "Don't set a default on 'site' to allow detecting with viper whether it's set
		// in config" (see common_settings.go). The template documents "datadoghq.com" as
		// the effective default but the code intentionally leaves it unset.
		"site": "effective default computed at runtime; code uses BindEnv only",
		// dd_url is derived from site at runtime; no programmatic default is set.
		"dd_url": "effective default computed from site at runtime; code uses BindEnv only",
		// bind_host has no programmatic default (BindEnv only); consuming code falls back
		// to "localhost". The template documents this effective fallback.
		"bind_host": "effective default applied in consuming code; code uses BindEnv only",
		// container_proc_root and container_cgroup_root defaults are set conditionally at
		// startup based on whether /host/... paths are mounted (containerized environment).
		// The template documents the containerized default (/host/...); non-containerized
		// environments use /proc and /sys/fs/cgroup/ respectively.
		"container_proc_root":   "runtime-conditional: /host/proc in containers, /proc otherwise",
		"container_cgroup_root": "runtime-conditional: /host/sys/fs/cgroup/ in containers, /sys/fs/cgroup/ otherwise",
	}

	for key, templateDefault := range templateDefaults {
		if reason, skip := knownExceptions[key]; skip {
			t.Logf("skipping known exception %q: %s", key, reason)
			continue
		}

		// Read the default layer directly to avoid env-variable overrides (DD_*)
		// causing false failures when the test runs in an environment that has
		// those variables set for unrelated reasons.
		var codeDefault interface{}
		for _, vs := range cfg.GetAllSources(key) {
			if vs.Source == pkgconfigmodel.SourceDefault {
				codeDefault = vs.Value
				break
			}
		}
		actual := fmt.Sprintf("%v", codeDefault)
		if actual != templateDefault {
			t.Errorf("config key %q: template documents default=%q but code default=%q",
				key, templateDefault, actual)
		}
	}
}
