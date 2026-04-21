/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"os"
	"path/filepath"
	"strings"

	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	yaml "gopkg.in/yaml.v2"
)

func pick(val string, best *ValueWithSource, source Source) {
	v := strings.TrimSpace(val)
	if v == "" {
		return
	}
	// Only take the value if we haven't set one yet or we're still at "default".
	if best.Source == "" || best.Source == SourceDefault {
		best.Value = v
		best.Source = source
	}
}

func joinNoProxySlice(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	return strings.Join(slice, ",")
}

// minimal YAML shape to read proxy.* from datadog.yaml directly when needed
type ddYAML struct {
	Proxy struct {
		HTTP    string   `yaml:"http"`
		HTTPS   string   `yaml:"https"`
		NoProxy []string `yaml:"no_proxy"`
	} `yaml:"proxy"`
}

func tryLoadProxyFromYAML(confDir string) (http, https string, np []string, ok bool) {
	if strings.TrimSpace(confDir) == "" {
		return "", "", nil, false
	}
	path := filepath.Join(confDir, "datadog.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", nil, false
	}
	var doc ddYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", "", nil, false
	}
	return doc.Proxy.HTTP, doc.Proxy.HTTPS, doc.Proxy.NoProxy, true
}

// ComputeEffective returns the chosen HTTP/HTTPS/NO_PROXY values with their sources.
// Precedence (highest→lowest):
//   DD_PROXY_*  → datadog.yaml proxy.*  → HTTPS_PROXY/HTTP_PROXY/NO_PROXY (incl. lowercase variants)
func ComputeEffective() Effective {
	var http, https, noProxy ValueWithSource
	http.Source, https.Source, noProxy.Source = SourceDefault, SourceDefault, SourceDefault

	// 1) Highest: DD_* (explicit Datadog env)
	pick(os.Getenv("DD_PROXY_HTTP"), &http, SourceDDEnv)
	pick(os.Getenv("DD_PROXY_HTTPS"), &https, SourceDDEnv)
	pick(os.Getenv("DD_NO_PROXY"), &noProxy, SourceDDEnv)

	// 2) Next: in-process datadog config (if already loaded)
	cfg := configsetup.Datadog()
	if cfg != nil {
		pick(cfg.GetString("proxy.http"), &http, SourceConfig)
		pick(cfg.GetString("proxy.https"), &https, SourceConfig)
		if v := cfg.GetStringSlice("proxy.no_proxy"); len(v) > 0 {
			pick(joinNoProxySlice(v), &noProxy, SourceConfig)
		}
	}

	// 2b) Fallback: if nothing (or partial) came from the in-process config, read datadog.yaml directly.
	// We honor DD_CONF_DIR (subcommand sets it from -c). This makes -c work even when the parent
	// command didn't initialize the global config.
	if ddConf := os.Getenv("DD_CONF_DIR"); ddConf != "" {
		if http.Value == "" || https.Value == "" || noProxy.Value == "" {
			if h, s, np, ok := tryLoadProxyFromYAML(ddConf); ok {
				if http.Value == "" {
					pick(h, &http, SourceConfig)
				}
				if https.Value == "" {
					pick(s, &https, SourceConfig)
				}
				if noProxy.Value == "" && len(np) > 0 {
					pick(joinNoProxySlice(np), &noProxy, SourceConfig)
				}
			}
		}
	}

	// 3) Lowest: standard env (upper then lower)
	pick(os.Getenv("HTTPS_PROXY"), &https, SourceStdEnv)
	pick(os.Getenv("HTTP_PROXY"), &http, SourceStdEnv)
	pick(os.Getenv("NO_PROXY"), &noProxy, SourceStdEnv)
	if https.Value == "" {
		pick(os.Getenv("https_proxy"), &https, SourceStdEnv)
	}
	if http.Value == "" {
		pick(os.Getenv("http_proxy"), &http, SourceStdEnv)
	}
	if noProxy.Value == "" {
		pick(os.Getenv("no_proxy"), &noProxy, SourceStdEnv)
	}

	eff := Effective{
		HTTP:    http,
		HTTPS:   https,
		NoProxy: noProxy,
	}

	// Non-exact NO_PROXY toggle (env preferred; allow config if exposed).
	if v := os.Getenv("DD_NO_PROXY_NONEXACT_MATCH"); strings.EqualFold(v, "true") {
		eff.NonExactNoProxy = true
	} else if cfg != nil && cfg.IsSet("DD_NO_PROXY_NONEXACT_MATCH") {
		eff.NonExactNoProxy = strings.EqualFold(cfg.GetString("DD_NO_PROXY_NONEXACT_MATCH"), "true")
	}

	return eff
}
