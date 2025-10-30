package proxy

import (
	"os"
	"strings"

	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func pick(val string, best *ValueWithSource, source Source) {
	if strings.TrimSpace(val) == "" {
		return
	}
	if best.Source == "" || best.Source == SourceDefault {
		best.Value = strings.TrimSpace(val)
		best.Source = source
	}
}

func joinNoProxySlice(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	return strings.Join(slice, ",")
}

// Precedence (highest→lowest):
//   DD_PROXY_*  →  datadog.yaml (proxy.*)  →  HTTPS_PROXY/HTTP_PROXY/NO_PROXY (+ lowercase)
func ComputeEffective() Effective {
	var http, https, noProxy ValueWithSource
	http.Source, https.Source, noProxy.Source = SourceDefault, SourceDefault, SourceDefault

	// Highest: DD_* (Datadog env)
	pick(os.Getenv("DD_PROXY_HTTP"), &http, SourceDDEnv)
	pick(os.Getenv("DD_PROXY_HTTPS"), &https, SourceDDEnv)
	pick(os.Getenv("DD_NO_PROXY"), &noProxy, SourceDDEnv)

	// Next: datadog.yaml via setup.Datadog() – assumes config was loaded by the Agent
	// (or by setting DD_CONF_DIR / passing -c at CLI).
	cfg := configsetup.Datadog()
	if cfg != nil {
		pick(cfg.GetString("proxy.http"), &http, SourceConfig)
		pick(cfg.GetString("proxy.https"), &https, SourceConfig)
		if v := cfg.GetStringSlice("proxy.no_proxy"); len(v) > 0 {
			pick(joinNoProxySlice(v), &noProxy, SourceConfig)
		}
	}

	// Lowest: standard env
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

	// Non-exact no_proxy (env preferred; allow config fallback if present)
	if v := os.Getenv("DD_NO_PROXY_NONEXACT_MATCH"); strings.EqualFold(v, "true") {
		eff.NonExactNoProxy = true
	} else if cfg != nil && cfg.IsSet("DD_NO_PROXY_NONEXACT_MATCH") {
		eff.NonExactNoProxy = strings.EqualFold(cfg.GetString("DD_NO_PROXY_NONEXACT_MATCH"), "true")
	}

	return eff
}
