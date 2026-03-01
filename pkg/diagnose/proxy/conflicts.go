/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"os"
	"strings"

	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// lintConflicts detects conflicting proxy values across dd_env / config / std_env.
func lintConflicts(eff Effective) []Finding {
	var out []Finding

	cfg := configsetup.Datadog()

	collect := func(key string) []ValueWithSource {
		values := []ValueWithSource{}

		// dd env
		if key == "https" {
			if v := strings.TrimSpace(os.Getenv("DD_PROXY_HTTPS")); v != "" {
				values = append(values, ValueWithSource{Value: v, Source: SourceDDEnv})
			}
		} else {
			if v := strings.TrimSpace(os.Getenv("DD_PROXY_HTTP")); v != "" {
				values = append(values, ValueWithSource{Value: v, Source: SourceDDEnv})
			}
		}

		// config
		if cfg != nil {
			if key == "https" {
				if v := strings.TrimSpace(cfg.GetString("proxy.https")); v != "" {
					values = append(values, ValueWithSource{Value: v, Source: SourceConfig})
				}
			} else {
				if v := strings.TrimSpace(cfg.GetString("proxy.http")); v != "" {
					values = append(values, ValueWithSource{Value: v, Source: SourceConfig})
				}
			}
		}

		// std env
		if key == "https" {
			if v := strings.TrimSpace(os.Getenv("HTTPS_PROXY")); v != "" {
				values = append(values, ValueWithSource{Value: v, Source: SourceStdEnv})
			}
		} else {
			if v := strings.TrimSpace(os.Getenv("HTTP_PROXY")); v != "" {
				values = append(values, ValueWithSource{Value: v, Source: SourceStdEnv})
			}
		}

		// Lowercase variants (last resort)
		if key == "https" && len(values) == 0 {
			if v := strings.TrimSpace(os.Getenv("https_proxy")); v != "" {
				values = append(values, ValueWithSource{Value: v, Source: SourceStdEnv})
			}
		}
		if key == "http" && len(values) == 0 {
			if v := strings.TrimSpace(os.Getenv("http_proxy")); v != "" {
				values = append(values, ValueWithSource{Value: v, Source: SourceStdEnv})
			}
		}

		// de-dup by value
		uniq := make(map[string]Source)
		out := []ValueWithSource{}
		for _, vv := range values {
			if _, seen := uniq[vv.Value]; !seen {
				uniq[vv.Value] = vv.Source
				out = append(out, vv)
			}
		}
		return out
	}

	// HTTPS
	if vals := collect("https"); len(vals) > 1 {
		out = append(out, Finding{
			Code:        "proxy.https.conflict",
			Severity:    SeverityYellow,
			Description: "HTTPS proxy is defined by multiple sources with different values.",
			Action:      "Use a single source or align the values across sources.",
			Evidence:    vals,
		})
	}

	// HTTP
	if vals := collect("http"); len(vals) > 1 {
		out = append(out, Finding{
			Code:        "proxy.http.conflict",
			Severity:    SeverityYellow,
			Description: "HTTP proxy is defined by multiple sources with different values.",
			Action:      "Use a single source or align the values across sources.",
			Evidence:    vals,
		})
	}

	return out
}
