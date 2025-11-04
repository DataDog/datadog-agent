/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import "strings"

// LintAll holds simple config-only lints that don't hit the network.
func LintAll(eff Effective) []Finding {
	var out []Finding

	// NO_PROXY leading dot can be surprising in some envs.
	if np := strings.TrimSpace(eff.NoProxy.Value); np != "" {
		for _, tok := range splitNoProxyList(np) {
			if strings.HasPrefix(tok, ".") {
				out = append(out, Finding{
					Code:        "no_proxy.leading_dot",
					Severity:    SeverityYellow,
					Description: "no_proxy entry starts with a dot (e.g., .example.com). Behavior may differ across environments.",
					Action:      "Prefer explicit hosts or bare domains (example.com) to avoid ambiguity.",
					Evidence:    tok,
				})
				break
			}
		}
	}

	return out
}
