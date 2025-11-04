/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"net/url"
	"strings"

	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// EffectiveEndpoints returns a privacy-safe list of "well-known" endpoints
// we can evaluate against NO_PROXY rules. Only the host/port are important;
// URLs are used as convenient carriers.
func EffectiveEndpoints() []Endpoint {
	var eps []Endpoint

	cfg := configsetup.Datadog()

	// Resolve site
	site := "datadoghq.com"
	if cfg != nil {
		if s := strings.TrimSpace(cfg.GetString("site")); s != "" {
			site = s
		}
	}

	// Resolve dd_url (control plane)
	ddURL := "https://app." + site
	if cfg != nil {
		if s := strings.TrimSpace(cfg.GetString("dd_url")); s != "" {
			ddURL = s
		}
	}
	eps = append(eps, Endpoint{Name: "core", URL: ddURL})

	// A small curated set of product intakes (hostnames only matter for NO_PROXY).
	host := func(h string) string { return "https://" + h + "." + site }
	eps = append(eps,
		Endpoint{Name: "dbm-metrics", URL: host("dbm-metrics-intake")},
		Endpoint{Name: "ndm", URL: host("ndm-intake")},
		Endpoint{Name: "snmp-traps", URL: host("snmp-traps-intake")},
		Endpoint{Name: "netpath", URL: host("netpath-intake")},
		Endpoint{Name: "container-life", URL: host("contlcycle-intake")},
		Endpoint{Name: "container-image", URL: host("contimage-intake")},
		Endpoint{Name: "sbom", URL: host("sbom-intake")},
	)

	// Flare support (generic hostname; versioned alias also exists in prod)
	eps = append(eps, Endpoint{Name: "flare", URL: "https://flare.agent." + site})

	return eps
}

func splitNoProxyList(s string) []string {
	// Support comma and/or whitespace separated tokens.
	out := []string{}
	f := func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }
	for _, tok := range strings.FieldsFunc(s, f) {
		tok = strings.TrimSpace(tok)
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

func domainMatches(host, suffix string) bool {
	// exact match or label-boundary suffix match
	if strings.EqualFold(host, suffix) {
		return true
	}
	return strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(suffix))
}

// EvaluateNoProxy computes a matrix describing which endpoints are bypassed by NO_PROXY.
func EvaluateNoProxy(eff Effective, eps []Endpoint) []EndpointCheck {
	matrix := make([]EndpointCheck, 0, len(eps))

	noProxy := strings.TrimSpace(eff.NoProxy.Value)
	tokens := splitNoProxyList(noProxy)

	for _, ep := range eps {
		u, err := url.Parse(ep.URL)
		if err != nil || u.Host == "" {
			continue
		}
		host := u.Hostname()
		port := u.Port()

		check := EndpointCheck{
			Endpoint: ep,
			Host:     host,
			Port:     port,
			Bypassed: false,
			Matched:  "",
		}

		for _, t := range tokens {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if t == "*" {
				check.Bypassed = true
				check.Matched = t
				break
			}
			if strings.HasPrefix(t, ".") {
				if domainMatches(host, strings.TrimPrefix(t, ".")) {
					check.Bypassed = true
					check.Matched = t
					break
				}
			} else {
				// exact hostname or (if allowed) substring fallback
				if strings.EqualFold(host, t) {
					check.Bypassed = true
					check.Matched = t
					break
				}
				if eff.NonExactNoProxy && strings.Contains(strings.ToLower(host), strings.ToLower(t)) {
					check.Bypassed = true
					check.Matched = t
					break
				}
			}
		}

		matrix = append(matrix, check)
	}

	return matrix
}
