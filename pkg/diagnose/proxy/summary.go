/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"fmt"
	"strings"
)

// FormatSummary renders a short privacy-safe block.
// (CLI has its own writer, but this is available for other callers.)
func FormatSummary(res Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary: %s\n", res.Summary)
	fmt.Fprintf(&b, "HTTPS: %q [%s]\n", RedactURL(res.Effective.HTTPS.Value), res.Effective.HTTPS.Source)
	fmt.Fprintf(&b, "HTTP : %q [%s]\n", RedactURL(res.Effective.HTTP.Value), res.Effective.HTTP.Source)
	if res.Effective.NoProxy.Value != "" {
		fmt.Fprintf(&b, "NO_PROXY: %q [%s]\n", res.Effective.NoProxy.Value, res.Effective.NoProxy.Source)
	}
	// Key findings (trim to keep it short)
	count := 0
	for _, f := range res.Findings {
		if count >= 3 {
			b.WriteString("…\n")
			break
		}
		fmt.Fprintf(&b, "- [%s] %s\n", f.Severity, f.Description)
		count++
	}
	return b.String()
}
