/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

// Run executes config lints and, if enabled, minimal network probes.
// - Config lints always run.
// - When noNetwork == false, we also attempt a TCP dial to the HTTPS proxy.
func Run(noNetwork bool) Result {
	eff := ComputeEffective()
	findings := []Finding{}

	// Config lints
	findings = append(findings, LintAll(eff)...)
	findings = append(findings, lintURLShapes(eff)...)
	findings = append(findings, lintConflicts(eff)...)

	// Config-only endpoint evaluation (privacy-safe)
	eps := EffectiveEndpoints()
	matrix := EvaluateNoProxy(eff, eps)
	if matrix == nil {
		// ensure endpoint_matrix is always [] in JSON, never null
		matrix = []EndpointCheck{}
	}

	for _, row := range matrix {
		if row.Bypassed {
			findings = append(findings, Finding{
				Code:        "no_proxy.endpoint_bypassed",
				Severity:    SeverityYellow,
				Description: row.Endpoint.Name + " endpoint will bypass the proxy due to NO_PROXY",
				Action:      "Remove or narrow the NO_PROXY token if unintended.",
				Evidence: map[string]string{
					"endpoint": row.Endpoint.Name,
					"host":     row.Host,
					"token":    row.Matched,
				},
			})
		}
	}

	// Minimal active probe path (off by default)
	if !noNetwork {
		findings = append(findings, ProbeProxyConnectivity(eff)...)
		findings = append(findings, ProbeEndpointsConnectivity(eff, eps)...)
	}

	// Summary rollup
	summary := SeverityGreen
	for _, f := range findings {
		if f.Severity == SeverityRed {
			summary = SeverityRed
			break
		}
		if f.Severity == SeverityYellow && summary == SeverityGreen {
			summary = SeverityYellow
		}
	}

	return Result{
		Summary:        summary,
		Effective:      eff,
		Findings:       findings,
		EndpointMatrix: matrix,
	}
}
