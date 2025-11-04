/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"context"
	"net"
	"net/url"
	"time"
)

// ProbeProxyConnectivity performs a minimal active probe when network checks are enabled.
// It attempts a TCP connection to the configured HTTPS proxy. If this fails, we report a red finding.
// This intentionally avoids sending any HTTP CONNECT or TLS handshake in the MVP.
func ProbeProxyConnectivity(eff Effective) []Finding {
	var out []Finding

	// Nothing to probe if no HTTPS proxy is set.
	if eff.HTTPS.Value == "" {
		return out
	}

	u, err := url.Parse(eff.HTTPS.Value)
	if err != nil || u.Host == "" {
		// Shape/parse problems are already covered by config lints.
		return out
	}

	// Determine host:port; default ports if missing.
	target := u.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		switch u.Scheme {
		case "http":
			target = net.JoinHostPort(u.Host, "80")
		case "https":
			target = net.JoinHostPort(u.Host, "443")
		default:
			// Non-standard/unknown schemes are flagged by lints; skip active probe.
			return out
		}
	}

	// Fast TCP dial to the proxy.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		out = append(out, Finding{
			Code:        "proxy.https.connect_failed",
			Severity:    SeverityRed,
			Description: "Failed to connect to the configured HTTPS proxy.",
			Action:      "Verify host/port, firewall, routing, and that the proxy is reachable.",
			Evidence: map[string]string{
				"target": target,
				"error":  err.Error(),
			},
		})
		return out
	}
	_ = conn.Close()

	return out
}

// ProbeEndpointsConnectivity remains a stub in the MVP.
// (Future: DNS/TCP/TLS/HTTP probes to each intake when --no-network=false.)
func ProbeEndpointsConnectivity(_ Effective, _ []Endpoint) []Finding {
	return nil
}
