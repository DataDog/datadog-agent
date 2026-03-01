/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"net/url"
	"strings"
)

func lintURLShapes(eff Effective) []Finding {
	var out []Finding

	// HTTPS proxy
	if v := strings.TrimSpace(eff.HTTPS.Value); v != "" {
		u, err := url.Parse(v)
		if err != nil || u.Scheme == "" || u.Host == "" {
			out = append(out, Finding{
				Code:        "proxy.https.invalid_url",
				Severity:    SeverityYellow,
				Description: "Proxy https is not a valid URL (missing scheme/host or parse failed).",
				Action:      "Provide a full URL, e.g. http://proxy.company:3128",
				Evidence:    v,
			})
		} else if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
			out = append(out, Finding{
				Code:        "proxy.https.unknown_scheme",
				Severity:    SeverityYellow,
				Description: "Proxy https uses a non-standard scheme.",
				Action:      "Use http:// or https:// unless you explicitly support the scheme.",
				Evidence:    v,
			})
		}
	}

	// HTTP proxy
	if v := strings.TrimSpace(eff.HTTP.Value); v != "" {
		u, err := url.Parse(v)
		if err != nil || u.Scheme == "" || u.Host == "" {
			out = append(out, Finding{
				Code:        "proxy.http.invalid_url",
				Severity:    SeverityYellow,
				Description: "Proxy http is not a valid URL (missing scheme/host or parse failed).",
				Action:      "Provide a full URL, e.g. http://proxy.company:3128",
				Evidence:    v,
			})
		} else if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
			out = append(out, Finding{
				Code:        "proxy.http.unknown_scheme",
				Severity:    SeverityYellow,
				Description: "Proxy http uses a non-standard scheme.",
				Action:      "Use http:// or https:// unless you explicitly support the scheme.",
				Evidence:    v,
			})
		}
	}

	return out
}
