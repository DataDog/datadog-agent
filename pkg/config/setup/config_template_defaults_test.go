// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// paramDefaultRe matches: ## @param KEY - TYPE - (optional|required) - default: VALUE
var paramDefaultRe = regexp.MustCompile(`^## @param (\S+) - [^-]+ - (?:optional|required) - default: (.+)$`)

// parseTopLevelParamDefaults returns config key → documented default string from
// top-level ## @param lines in config_template.yaml. Go-template expressions are
// skipped; list/map defaults are kept as raw JSON strings for JSON comparison.
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
		// Strip surrounding double-quotes from string values.
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

// TestConfigTemplateDefaultsMatchCode checks that default values documented in
// @param comments in config_template.yaml match the defaults registered by InitConfig.
func TestConfigTemplateDefaultsMatchCode(t *testing.T) {
	cfg := newTestConf(t)
	templateDefaults := parseTopLevelParamDefaults(t)

	// Keys where the template documents an effective/user-visible default that differs
	// from the raw code default. Each value must describe why.
	knownExceptions := map[string]string{
		"site":                  "effective default computed at runtime; code uses BindEnv only",
		"dd_url":                "effective default computed from site at runtime; code uses BindEnv only",
		"bind_host":             "effective default applied in consuming code; code uses BindEnv only",
		"container_proc_root":   "runtime-conditional: /host/proc in containers, /proc otherwise",
		"container_cgroup_root": "runtime-conditional: /host/sys/fs/cgroup/ in containers, /sys/fs/cgroup/ otherwise",
	}

	for key, templateDefault := range templateDefaults {
		if reason, skip := knownExceptions[key]; skip {
			t.Logf("skipping known exception %q: %s", key, reason)
			continue
		}

		// Use the default source directly; DD_* env vars can override Get() and
		// cause false failures in environments that set them for unrelated reasons.
		var codeDefault interface{}
		for _, vs := range cfg.GetAllSources(key) {
			if vs.Source == pkgconfigmodel.SourceDefault {
				codeDefault = vs.Value
				break
			}
		}

		if strings.HasPrefix(templateDefault, "[") || strings.HasPrefix(templateDefault, "{") {
			// List/map defaults: compare via JSON since fmt.Sprintf produces Go syntax
			// (e.g. "[a b c]") rather than JSON syntax.
			var templateVal interface{}
			if err := json.Unmarshal([]byte(templateDefault), &templateVal); err != nil {
				t.Logf("skipping %q: cannot parse template default as JSON: %v", key, err)
				continue
			}
			codeJSON, err := json.Marshal(codeDefault)
			if err != nil {
				t.Logf("skipping %q: cannot marshal code default to JSON: %v", key, err)
				continue
			}
			var codeVal interface{}
			if err := json.Unmarshal(codeJSON, &codeVal); err != nil {
				t.Logf("skipping %q: cannot unmarshal code default JSON: %v", key, err)
				continue
			}
			if tSlice, ok := templateVal.([]interface{}); ok {
				// Sort before comparing: config lists are typically sets where order doesn't matter.
				cSlice, _ := codeVal.([]interface{})
				toSortedStrings := func(s []interface{}) []string {
					out := make([]string, len(s))
					for i, v := range s {
						out[i] = fmt.Sprintf("%v", v)
					}
					sort.Strings(out)
					return out
				}
				if !reflect.DeepEqual(toSortedStrings(tSlice), toSortedStrings(cSlice)) {
					t.Errorf("config key %q: template documents default=%q but code default=%v",
						key, templateDefault, codeDefault)
				}
				continue
			}
			if !reflect.DeepEqual(templateVal, codeVal) {
				t.Errorf("config key %q: template documents default=%q but code default=%v",
					key, templateDefault, codeDefault)
			}
			continue
		}

		actual := fmt.Sprintf("%v", codeDefault)
		if actual != templateDefault {
			t.Errorf("config key %q: template documents default=%q but code default=%q",
				key, templateDefault, actual)
		}
	}
}
