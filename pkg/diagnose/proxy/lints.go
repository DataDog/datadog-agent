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
